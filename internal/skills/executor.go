package skills

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var simpleInputInterpolation = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)

const MaxReadOnlySkillSteps = 50

type SkillError struct {
	Code    string
	Message string
	Name    string
	Err     error
}

func (e SkillError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

func (e SkillError) Unwrap() error {
	return e.Err
}

func skillError(code, message string, err error) SkillError {
	return SkillError{Code: code, Message: message, Err: err}
}

type ExecutorRegistry interface {
	Get(name string) (Manifest, bool)
}

type ToolCaller interface {
	CallTool(ctx context.Context, tool string, args map[string]any) (any, error)
}

type ReadOnlyExecutor struct {
	registry ExecutorRegistry
	caller   ToolCaller
}

type RunRequest struct {
	Skill  string         `json:"name"`
	Inputs map[string]any `json:"inputs,omitempty"`
	DryRun bool           `json:"dryRun"`
}

type RunResult struct {
	Skill  string         `json:"skill"`
	Inputs map[string]any `json:"inputs"`
	DryRun bool           `json:"dryRun"`
	Steps  []StepResult   `json:"steps"`
}

type StepResult struct {
	Index  int            `json:"index"`
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args"`
	Result any            `json:"result"`
}

func NewReadOnlyExecutor(registry ExecutorRegistry, caller ToolCaller) ReadOnlyExecutor {
	return ReadOnlyExecutor{registry: registry, caller: caller}
}

func (e ReadOnlyExecutor) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if e.registry == nil {
		return RunResult{}, skillError("skill_executor_unconfigured", "skill registry is not configured", nil)
	}
	if e.caller == nil {
		return RunResult{}, skillError("skill_executor_unconfigured", "skill tool caller is not configured", nil)
	}
	name := strings.TrimSpace(req.Skill)
	if name == "" {
		return RunResult{}, skillError("invalid_skill_input", "skill name is required", nil)
	}
	manifest, ok := e.registry.Get(name)
	if !ok {
		return RunResult{}, SkillError{Code: "skill_not_found", Message: fmt.Sprintf("skill %q was not found", name), Name: name}
	}
	if err := validateReadOnlyManifest(manifest); err != nil {
		return RunResult{}, err
	}
	inputs := cloneMap(req.Inputs)
	if err := validateRequiredInputs(manifest, inputs); err != nil {
		return RunResult{}, err
	}
	if len(manifest.Steps) > MaxReadOnlySkillSteps {
		return RunResult{}, SkillError{Code: "skill_step_limit_exceeded", Message: fmt.Sprintf("skill %q has %d steps, exceeding max %d", manifest.Name, len(manifest.Steps), MaxReadOnlySkillSteps), Name: manifest.Name}
	}
	resolvedSteps := make([]StepResult, len(manifest.Steps))
	for i, step := range manifest.Steps {
		args, err := interpolateArgs(step.Args, inputs)
		if err != nil {
			return RunResult{}, SkillError{Code: "unsupported_interpolation", Message: fmt.Sprintf("step %d %s: %v", i, step.Tool, err), Name: manifest.Name, Err: err}
		}
		resolvedSteps[i] = StepResult{Index: i, Tool: step.Tool, Args: args}
	}
	steps := make([]StepResult, 0, len(manifest.Steps))
	for _, step := range resolvedSteps {
		result, err := e.caller.CallTool(ctx, step.Tool, step.Args)
		if err != nil {
			return RunResult{}, SkillError{Code: "skill_step_failed", Message: fmt.Sprintf("step %d %s failed: %v", step.Index, step.Tool, err), Name: manifest.Name, Err: err}
		}
		step.Result = result
		steps = append(steps, step)
	}
	return RunResult{Skill: manifest.Name, Inputs: inputs, DryRun: req.DryRun, Steps: steps}, nil
}

func validateReadOnlyManifest(manifest Manifest) error {
	if manifest.Write {
		return SkillError{Code: "read_only_violation", Message: fmt.Sprintf("skill %q is write-capable and cannot run in read-only executor mode", manifest.Name), Name: manifest.Name}
	}
	for _, capability := range manifest.Capabilities {
		if isWriteCapability(capability) {
			return SkillError{Code: "read_only_violation", Message: fmt.Sprintf("skill %q capability %q is write-capable and cannot run in read-only executor mode", manifest.Name, capability), Name: manifest.Name}
		}
	}
	for i, step := range manifest.Steps {
		if !isAllowedStepTool(step.Tool) {
			return SkillError{Code: "read_only_violation", Message: fmt.Sprintf("skill %q step %d tool %q is not allowed", manifest.Name, i, step.Tool), Name: manifest.Name}
		}
		if isWriteStepTool(step.Tool) {
			return SkillError{Code: "read_only_violation", Message: fmt.Sprintf("skill %q step %d tool %q is write-capable and cannot run in read-only executor mode", manifest.Name, i, step.Tool), Name: manifest.Name}
		}
	}
	return nil
}

func validateRequiredInputs(manifest Manifest, inputs map[string]any) error {
	required, ok := manifest.Inputs["required"]
	if !ok {
		return nil
	}
	for _, name := range requiredInputNames(required) {
		if _, ok := inputs[name]; !ok {
			return skillError("invalid_skill_input", fmt.Sprintf("required input %q is missing", name), nil)
		}
	}
	return nil
}

func requiredInputNames(required any) []string {
	switch values := required.(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		names := make([]string, 0, len(values))
		for _, value := range values {
			if name, ok := value.(string); ok {
				names = append(names, name)
			}
		}
		return names
	default:
		return nil
	}
}

func interpolateArgs(args map[string]any, inputs map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(args))
	for key, value := range args {
		interpolated, err := interpolateValue(value, inputs)
		if err != nil {
			return nil, fmt.Errorf("arg %q: %w", key, err)
		}
		out[key] = interpolated
	}
	return out, nil
}

func interpolateValue(value any, inputs map[string]any) (any, error) {
	text, ok := value.(string)
	if !ok {
		return value, nil
	}
	if !strings.Contains(text, "${") {
		return text, nil
	}
	matches := simpleInputInterpolation.FindStringSubmatch(text)
	if matches == nil {
		return nil, fmt.Errorf("unsupported interpolation syntax %q", text)
	}
	inputValue, ok := inputs[matches[1]]
	if !ok {
		return nil, fmt.Errorf("input %q is not provided", matches[1])
	}
	stringValue, ok := inputValue.(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string for interpolation", matches[1])
	}
	return stringValue, nil
}

func cloneMap(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
