package skills

import (
	"context"
	"fmt"
	"reflect"
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
	options  ExecutorOptions
	readOnly bool
}

type ExecutorOptions struct {
	EnableWrite      bool
	EnableRealWrites bool
}

type RunRequest struct {
	Skill  string         `json:"name"`
	Inputs map[string]any `json:"inputs,omitempty"`
	DryRun *bool          `json:"dryRun,omitempty"`
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
	return ReadOnlyExecutor{registry: registry, caller: caller, readOnly: true}
}

func NewExecutorWithOptions(registry ExecutorRegistry, caller ToolCaller, options ExecutorOptions) ReadOnlyExecutor {
	return ReadOnlyExecutor{registry: registry, caller: caller, options: options}
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
	dryRun := effectiveDryRun(req.DryRun)
	if err := e.validateManifestForRun(manifest, dryRun); err != nil {
		return RunResult{}, err
	}
	inputs := cloneMap(req.Inputs)
	if err := validateRequiredInputs(manifest, inputs); err != nil {
		return RunResult{}, err
	}
	interpolationInputs := cloneMap(inputs)
	if _, ok := interpolationInputs["operationId"]; !ok {
		interpolationInputs["operationId"] = ""
	}
	if len(manifest.Steps) > MaxReadOnlySkillSteps {
		return RunResult{}, SkillError{Code: "skill_step_limit_exceeded", Message: fmt.Sprintf("skill %q has %d steps, exceeding max %d", manifest.Name, len(manifest.Steps), MaxReadOnlySkillSteps), Name: manifest.Name}
	}
	resolvedSteps := make([]StepResult, len(manifest.Steps))
	for i, step := range manifest.Steps {
		args, err := interpolateArgs(step.Args, interpolationInputs)
		if err != nil {
			return RunResult{}, SkillError{Code: "unsupported_interpolation", Message: fmt.Sprintf("step %d %s: %v", i, step.Tool, err), Name: manifest.Name, Err: err}
		}
		if isWriteStepTool(step.Tool) {
			args["dryRun"] = dryRun
		}
		resolvedSteps[i] = StepResult{Index: i, Tool: step.Tool, Args: args}
	}
	if !dryRun {
		if err := validateResolvedRealWriteSteps(manifest, resolvedSteps); err != nil {
			return RunResult{}, err
		}
	}
	steps := make([]StepResult, 0, len(manifest.Steps))
	var permission permissionPreflight
	for _, step := range resolvedSteps {
		if isWriteStepTool(step.Tool) && !dryRun && !permissionTargetsWrite(permission.Args, step.Args, step.Tool) {
			return RunResult{}, SkillError{Code: "permission_preflight_target_mismatch", Message: fmt.Sprintf("skill %q step %d permission preflight target does not match write target", manifest.Name, step.Index), Name: manifest.Name}
		}
		if isWriteStepTool(step.Tool) && !dryRun && !permissionAllowsTool(step.Tool, permission.Result) {
			return RunResult{}, SkillError{Code: "permission_preflight_denied", Message: fmt.Sprintf("skill %q step %d permission preflight denied mutation", manifest.Name, step.Index), Name: manifest.Name}
		}
		result, err := e.caller.CallTool(ctx, step.Tool, step.Args)
		if err != nil {
			return RunResult{}, SkillError{Code: "skill_step_failed", Message: fmt.Sprintf("step %d %s failed: %v", step.Index, step.Tool, err), Name: manifest.Name, Err: err}
		}
		if step.Tool == "feishu_doc_check_permission" {
			permission = permissionPreflight{Args: step.Args, Result: result, Valid: true}
		}
		step.Result = result
		steps = append(steps, step)
	}
	return RunResult{Skill: manifest.Name, Inputs: inputs, DryRun: dryRun, Steps: steps}, nil
}

type permissionPreflight struct {
	Args   map[string]any
	Result any
	Valid  bool
}

func validateResolvedRealWriteSteps(manifest Manifest, steps []StepResult) error {
	for _, step := range steps {
		if !isWriteStepTool(step.Tool) {
			continue
		}
		if writeStepToolRequiresOperationID(step.Tool) && !hasOperationID(step.Args) {
			return SkillError{Code: "operation_id_required", Message: fmt.Sprintf("skill %q step %d tool %q requires non-empty operationId for dryRun=false", manifest.Name, step.Index, step.Tool), Name: manifest.Name}
		}
	}
	return nil
}

func (e ReadOnlyExecutor) validateManifestForRun(manifest Manifest, dryRun bool) error {
	if e.readOnly {
		return validateReadOnlyManifest(manifest)
	}
	if !e.options.EnableWrite {
		if manifest.Write || manifestHasWriteStep(manifest) || hasWriteCapability(manifest) {
			return SkillError{Code: "write_skills_disabled", Message: fmt.Sprintf("skill %q is write-capable but write skills are disabled", manifest.Name), Name: manifest.Name}
		}
		return validateReadOnlyManifest(manifest)
	}
	if !manifest.Write {
		return validateReadOnlyManifest(manifest)
	}
	if !hasWriteCapability(manifest) {
		return SkillError{Code: "write_capability_required", Message: fmt.Sprintf("skill %q uses write mode but declares no write capability", manifest.Name), Name: manifest.Name}
	}
	hasPreflight := false
	for i, step := range manifest.Steps {
		if !isAllowedStepTool(step.Tool) {
			return SkillError{Code: "read_only_violation", Message: fmt.Sprintf("skill %q step %d tool %q is not allowed", manifest.Name, i, step.Tool), Name: manifest.Name}
		}
		if step.Tool == "feishu_doc_check_permission" {
			hasPreflight = true
			continue
		}
		if !isWriteStepTool(step.Tool) {
			continue
		}
		if !dryRun && !e.options.EnableRealWrites {
			return SkillError{Code: "real_write_disabled", Message: fmt.Sprintf("skill %q requested dryRun=false but real writes are disabled", manifest.Name), Name: manifest.Name}
		}
		if !dryRun && writeStepToolRequiresOperationID(step.Tool) && !hasOperationID(step.Args) {
			return SkillError{Code: "operation_id_required", Message: fmt.Sprintf("skill %q step %d tool %q requires operationId for dryRun=false", manifest.Name, i, step.Tool), Name: manifest.Name}
		}
		if !dryRun && !hasPreflight {
			return SkillError{Code: "permission_preflight_required", Message: fmt.Sprintf("skill %q step %d tool %q requires prior permission preflight for dryRun=false", manifest.Name, i, step.Tool), Name: manifest.Name}
		}
	}
	return nil
}

func effectiveDryRun(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}

func hasWriteCapability(manifest Manifest) bool {
	for _, capability := range manifest.Capabilities {
		if isWriteCapability(capability) {
			return true
		}
	}
	return false
}

func manifestHasWriteStep(manifest Manifest) bool {
	for _, step := range manifest.Steps {
		if isWriteStepTool(step.Tool) {
			return true
		}
	}
	return false
}

func hasOperationID(args map[string]any) bool {
	operationID, ok := args["operationId"].(string)
	return ok && strings.TrimSpace(operationID) != ""
}

func writeStepToolRequiresOperationID(tool string) bool {
	return isWriteStepTool(tool)
}

func permissionAllowsTool(tool string, result any) bool {
	canWrite, canComment, ok := permissionBooleans(result)
	if !ok {
		return false
	}
	switch tool {
	case "feishu_doc_create_comment", "feishu_doc_reply_comment", "feishu_doc_resolve_comment":
		return canComment
	default:
		return canWrite
	}
}

func permissionTargetsWrite(preflightArgs, writeArgs map[string]any, tool string) bool {
	preflightInput, ok := stringArg(preflightArgs, "input")
	if !ok || preflightInput == "" {
		return false
	}
	writeTarget, ok := writePermissionTarget(tool, writeArgs)
	if !ok || preflightInput != writeTarget {
		return false
	}
	_, preflightCredentialSpecified := preflightArgs["credentialId"]
	_, writeCredentialSpecified := writeArgs["credentialId"]
	if preflightCredentialSpecified != writeCredentialSpecified {
		return false
	}
	if !preflightCredentialSpecified {
		return true
	}
	preflightCredential, hasPreflightCredential := stringArg(preflightArgs, "credentialId")
	writeCredential, hasWriteCredential := stringArg(writeArgs, "credentialId")
	return hasPreflightCredential && hasWriteCredential && preflightCredential != "" && preflightCredential == writeCredential
}

func writePermissionTarget(tool string, args map[string]any) (string, bool) {
	if input, ok := stringArg(args, "input"); ok && input != "" {
		return input, true
	}
	if tool == "feishu_doc_create" {
		folderToken, ok := stringArg(args, "folderToken")
		if ok && folderToken != "" {
			return folderToken, true
		}
	}
	return "", false
}

func stringArg(args map[string]any, key string) (string, bool) {
	value, ok := args[key].(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func permissionBooleans(result any) (canWrite bool, canComment bool, ok bool) {
	values, isMap := result.(map[string]any)
	if isMap {
		canWrite, _ = values["canWrite"].(bool)
		canCommentValue, hasCanComment := values["canComment"].(bool)
		if hasCanComment {
			return canWrite, canCommentValue, true
		}
		return canWrite, canWrite, true
	}
	value := reflect.Indirect(reflect.ValueOf(result))
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return false, false, false
	}
	canWriteField := value.FieldByName("CanWrite")
	if !canWriteField.IsValid() || canWriteField.Kind() != reflect.Bool {
		return false, false, false
	}
	canCommentField := value.FieldByName("CanComment")
	if canCommentField.IsValid() && canCommentField.Kind() == reflect.Bool {
		return canWriteField.Bool(), canCommentField.Bool(), true
	}
	return canWriteField.Bool(), canWriteField.Bool(), true
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
