package skills

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var simpleInputInterpolation = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)

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
		return RunResult{}, fmt.Errorf("skill registry is not configured")
	}
	name := strings.TrimSpace(req.Skill)
	if name == "" {
		return RunResult{}, fmt.Errorf("skill name is required")
	}
	manifest, ok := e.registry.Get(name)
	if !ok {
		return RunResult{}, fmt.Errorf("skill %q was not found", name)
	}
	if err := validateReadOnlyManifest(manifest); err != nil {
		return RunResult{}, err
	}
	inputs := cloneMap(req.Inputs)
	if err := validateRequiredInputs(manifest, inputs); err != nil {
		return RunResult{}, err
	}
	steps := make([]StepResult, 0, len(manifest.Steps))
	for i, step := range manifest.Steps {
		args, err := interpolateArgs(step.Args, inputs)
		if err != nil {
			return RunResult{}, fmt.Errorf("step %d %s: %w", i, step.Tool, err)
		}
		result, err := e.caller.CallTool(ctx, step.Tool, args)
		if err != nil {
			return RunResult{}, fmt.Errorf("step %d %s failed: %w", i, step.Tool, err)
		}
		steps = append(steps, StepResult{Index: i, Tool: step.Tool, Args: args, Result: result})
	}
	return RunResult{Skill: manifest.Name, Inputs: inputs, DryRun: req.DryRun, Steps: steps}, nil
}

func validateReadOnlyManifest(manifest Manifest) error {
	if manifest.Write {
		return fmt.Errorf("skill %q is write-capable and cannot run in read-only executor mode", manifest.Name)
	}
	for _, capability := range manifest.Capabilities {
		if isWriteCapability(capability) {
			return fmt.Errorf("skill %q capability %q is write-capable and cannot run in read-only executor mode", manifest.Name, capability)
		}
	}
	for i, step := range manifest.Steps {
		if !isAllowedStepTool(step.Tool) {
			return fmt.Errorf("skill %q step %d tool %q is not allowed", manifest.Name, i, step.Tool)
		}
		if isWriteStepTool(step.Tool) {
			return fmt.Errorf("skill %q step %d tool %q is write-capable and cannot run in read-only executor mode", manifest.Name, i, step.Tool)
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
			return fmt.Errorf("required input %q is missing", name)
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
