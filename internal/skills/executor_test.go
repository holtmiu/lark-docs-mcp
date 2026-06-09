package skills

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestExecutorRunsReadOnlySkillStepsInOrder(t *testing.T) {
	manifest := Manifest{
		Name:   "read-doc",
		Inputs: map[string]any{"type": "object", "required": []any{"input"}},
		Steps: []Step{
			{Tool: "feishu_doc_resolve", Args: map[string]any{"input": "${input}"}},
			{Tool: "feishu_doc_read", Args: map[string]any{"input": "${input}", "format": "markdown"}},
		},
	}
	caller := &fakeToolCaller{results: []any{map[string]any{"token": "doc-token"}, map[string]any{"markdown": "hello"}}}
	executor := NewReadOnlyExecutor(fakeExecutorRegistry{manifest: manifest}, caller)

	got, err := executor.Run(context.Background(), RunRequest{Skill: "read-doc", Inputs: map[string]any{"input": "https://example.test/doc"}, DryRun: boolPtr(true)})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %#v, want 2 ordered calls", caller.calls)
	}
	if caller.calls[0].tool != "feishu_doc_resolve" || caller.calls[1].tool != "feishu_doc_read" {
		t.Fatalf("call order = %#v", caller.calls)
	}
	if caller.calls[0].args["input"] != "https://example.test/doc" || caller.calls[1].args["format"] != "markdown" {
		t.Fatalf("call args = %#v", caller.calls)
	}
	if got.Skill != "read-doc" || !got.DryRun || len(got.Steps) != 2 {
		t.Fatalf("result = %+v", got)
	}
}

func TestExecutorInterpolatesSimpleStringInputs(t *testing.T) {
	manifest := Manifest{Name: "metadata", Inputs: map[string]any{"type": "object"}, Steps: []Step{{Tool: "feishu_doc_get_metadata", Args: map[string]any{"input": "${docURL}", "credentialId": "${credentialId}"}}}}
	caller := &fakeToolCaller{results: []any{map[string]any{"title": "Doc"}}}
	executor := NewReadOnlyExecutor(fakeExecutorRegistry{manifest: manifest}, caller)

	_, err := executor.Run(context.Background(), RunRequest{Skill: "metadata", Inputs: map[string]any{"docURL": "url-1", "credentialId": "cred-1"}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	wantArgs := map[string]any{"input": "url-1", "credentialId": "cred-1"}
	if !reflect.DeepEqual(caller.calls[0].args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", caller.calls[0].args, wantArgs)
	}
}

func TestExecutorMissingRequiredInputFailsBeforeSteps(t *testing.T) {
	manifest := Manifest{Name: "read-doc", Inputs: map[string]any{"type": "object", "required": []any{"input"}}, Steps: []Step{{Tool: "feishu_doc_read", Args: map[string]any{"input": "${input}"}}}}
	caller := &fakeToolCaller{}
	executor := NewReadOnlyExecutor(fakeExecutorRegistry{manifest: manifest}, caller)

	_, err := executor.Run(context.Background(), RunRequest{Skill: "read-doc", Inputs: map[string]any{}})
	if err == nil || !strings.Contains(err.Error(), "required input") {
		t.Fatalf("error = %v, want required input error", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want no calls", caller.calls)
	}
}

func TestExecutorUnsupportedInterpolationSyntaxFailsClosed(t *testing.T) {
	manifest := Manifest{Name: "read-doc", Inputs: map[string]any{"type": "object"}, Steps: []Step{{Tool: "feishu_doc_read", Args: map[string]any{"input": "${doc.url}"}}}}
	caller := &fakeToolCaller{}
	executor := NewReadOnlyExecutor(fakeExecutorRegistry{manifest: manifest}, caller)

	_, err := executor.Run(context.Background(), RunRequest{Skill: "read-doc", Inputs: map[string]any{"doc": "url"}})
	if err == nil || !strings.Contains(err.Error(), "unsupported interpolation") {
		t.Fatalf("error = %v, want unsupported interpolation error", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want no calls", caller.calls)
	}
}

func TestExecutorRejectsWriteCapabilitySkillInReadOnlyMode(t *testing.T) {
	manifest := Manifest{Name: "comment", Write: true, Inputs: map[string]any{"type": "object"}, Capabilities: []string{"doc.comment.create"}, Steps: []Step{{Tool: "feishu_doc_create_comment", Args: map[string]any{"input": "doc", "content": "hi"}}}}
	caller := &fakeToolCaller{}
	executor := NewReadOnlyExecutor(fakeExecutorRegistry{manifest: manifest}, caller)

	_, err := executor.Run(context.Background(), RunRequest{Skill: "comment", Inputs: map[string]any{}})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("error = %v, want read-only rejection", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want no calls", caller.calls)
	}
}

func TestExecutorReturnsStructuredStepOutputs(t *testing.T) {
	manifest := Manifest{Name: "inspect", Inputs: map[string]any{"type": "object"}, Steps: []Step{{Tool: "feishu_doc_resolve", Args: map[string]any{"input": "doc-token"}}, {Tool: "feishu_doc_check_permission", Args: map[string]any{"input": "doc-token"}}}}
	caller := &fakeToolCaller{results: []any{map[string]any{"token": "doc-token"}, map[string]any{"canRead": true}}}
	executor := NewReadOnlyExecutor(fakeExecutorRegistry{manifest: manifest}, caller)

	got, err := executor.Run(context.Background(), RunRequest{Skill: "inspect", Inputs: map[string]any{"unused": "value"}, DryRun: boolPtr(false)})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got.Skill != "inspect" || got.DryRun || !reflect.DeepEqual(got.Inputs, map[string]any{"unused": "value"}) {
		t.Fatalf("result context = %+v", got)
	}
	if len(got.Steps) != 2 || got.Steps[0].Index != 0 || got.Steps[0].Tool != "feishu_doc_resolve" || got.Steps[1].Tool != "feishu_doc_check_permission" {
		t.Fatalf("step metadata = %+v", got.Steps)
	}
	marshaled, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal result: %v", err)
	}
	if !strings.Contains(string(marshaled), "\"result\":") || !strings.Contains(string(marshaled), "canRead") {
		t.Fatalf("structured result JSON missing step outputs: %s", marshaled)
	}
}

func TestExecutorPrevalidatesAllStepArgsBeforeCallingTools(t *testing.T) {
	manifest := Manifest{Name: "preflight", Inputs: map[string]any{"type": "object"}, Steps: []Step{
		{Tool: "feishu_doc_resolve", Args: map[string]any{"input": "${input}"}},
		{Tool: "feishu_doc_read", Args: map[string]any{"input": "${missing}"}},
	}}
	caller := &fakeToolCaller{}
	executor := NewReadOnlyExecutor(fakeExecutorRegistry{manifest: manifest}, caller)

	_, err := executor.Run(context.Background(), RunRequest{Skill: "preflight", Inputs: map[string]any{"input": "doc-token"}})
	assertSkillErrorCode(t, err, "unsupported_interpolation")
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want no calls when a later step arg is invalid", caller.calls)
	}
}

func TestExecutorNilCallerFailsClosed(t *testing.T) {
	manifest := Manifest{Name: "read-doc", Inputs: map[string]any{"type": "object"}, Steps: []Step{{Tool: "feishu_doc_read", Args: map[string]any{"input": "doc-token"}}}}
	executor := NewReadOnlyExecutor(fakeExecutorRegistry{manifest: manifest}, nil)

	_, err := executor.Run(context.Background(), RunRequest{Skill: "read-doc"})
	assertSkillErrorCode(t, err, "skill_executor_unconfigured")
}

func TestExecutorStepFailureHasStructuredCode(t *testing.T) {
	manifest := Manifest{Name: "read-doc", Inputs: map[string]any{"type": "object"}, Steps: []Step{{Tool: "feishu_doc_read", Args: map[string]any{"input": "doc-token"}}}}
	caller := &fakeToolCaller{err: errors.New("boom")}
	executor := NewReadOnlyExecutor(fakeExecutorRegistry{manifest: manifest}, caller)

	_, err := executor.Run(context.Background(), RunRequest{Skill: "read-doc"})
	assertSkillErrorCode(t, err, "skill_step_failed")
}

func TestExecutorRejectsTooManySteps(t *testing.T) {
	steps := make([]Step, 51)
	for i := range steps {
		steps[i] = Step{Tool: "feishu_doc_read", Args: map[string]any{"input": "doc-token"}}
	}
	manifest := Manifest{Name: "too-many", Inputs: map[string]any{"type": "object"}, Steps: steps}
	caller := &fakeToolCaller{}
	executor := NewReadOnlyExecutor(fakeExecutorRegistry{manifest: manifest}, caller)

	_, err := executor.Run(context.Background(), RunRequest{Skill: "too-many"})
	assertSkillErrorCode(t, err, "skill_step_limit_exceeded")
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want no calls", caller.calls)
	}
}

func TestWriteSkillRejectedWhenWritePolicyDisabled(t *testing.T) {
	manifest := writeCommentManifest(map[string]any{"input": "doc-token", "content": "hi", "dryRun": false, "operationId": "op-1"})
	caller := &fakeToolCaller{}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: false})

	_, err := executor.Run(context.Background(), RunRequest{Skill: "comment", Inputs: map[string]any{}, DryRun: boolPtr(false)})
	assertSkillErrorCode(t, err, "write_skills_disabled")
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want no unsafe downstream calls", caller.calls)
	}
}

func TestWriteStepsDefaultToDryRunTrue(t *testing.T) {
	manifest := writeCommentManifest(map[string]any{"input": "doc-token", "content": "hi", "operationId": "op-1"})
	caller := &fakeToolCaller{results: []any{map[string]any{"canWrite": true, "canComment": true}, map[string]any{"dryRun": true}}}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: true})

	got, err := executor.Run(context.Background(), RunRequest{Skill: "comment", Inputs: map[string]any{}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !got.DryRun || len(caller.calls) != 2 {
		t.Fatalf("result=%+v calls=%#v, want dry-run execution", got, caller.calls)
	}
	if caller.calls[1].args["dryRun"] != true {
		t.Fatalf("write args = %#v, want dryRun true injected by executor", caller.calls[1].args)
	}
}

func TestRealWriteRejectedUnlessWritePolicyAllowsRealMutations(t *testing.T) {
	manifest := writeCommentManifest(map[string]any{"input": "doc-token", "content": "hi", "dryRun": false, "operationId": "op-1"})
	caller := &fakeToolCaller{}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: true, EnableRealWrites: false})

	_, err := executor.Run(context.Background(), RunRequest{Skill: "comment", Inputs: map[string]any{}, DryRun: boolPtr(false)})
	assertSkillErrorCode(t, err, "real_write_disabled")
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want no unsafe downstream calls", caller.calls)
	}
}

func TestRealWriteRequiresOperationIDWhenToolSupportsIt(t *testing.T) {
	manifest := writeCommentManifest(map[string]any{"input": "doc-token", "content": "hi", "dryRun": false})
	caller := &fakeToolCaller{}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: true, EnableRealWrites: true})

	_, err := executor.Run(context.Background(), RunRequest{Skill: "comment", Inputs: map[string]any{}, DryRun: boolPtr(false)})
	assertSkillErrorCode(t, err, "operation_id_required")
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want no unsafe downstream calls", caller.calls)
	}
}

func TestRealWriteRequiresResolvedOperationIDAfterInterpolation(t *testing.T) {
	manifest := writeCommentManifest(map[string]any{"input": "doc-token", "content": "hi", "dryRun": false, "operationId": "${operationId}"})
	caller := &fakeToolCaller{results: []any{map[string]any{"canWrite": true, "canComment": true}}}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: true, EnableRealWrites: true})

	_, err := executor.Run(context.Background(), RunRequest{Skill: "comment", Inputs: map[string]any{"operationId": ""}, DryRun: boolPtr(false)})
	assertSkillErrorCode(t, err, "operation_id_required")
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want no preflight or write call when resolved operationId is empty", caller.calls)
	}
}

func TestRealWriteRequiresPermissionPreflightTargetMatch(t *testing.T) {
	manifest := Manifest{
		Name:         "comment",
		Write:        true,
		Inputs:       map[string]any{"type": "object"},
		Capabilities: []string{"doc.comment.create"},
		Steps: []Step{
			{Tool: "feishu_doc_check_permission", Args: map[string]any{"input": "doc-A", "credentialId": "cred-1"}},
			{Tool: "feishu_doc_create_comment", Args: map[string]any{"input": "doc-B", "credentialId": "cred-1", "content": "hi", "dryRun": false, "operationId": "op-1"}},
		},
	}
	caller := &fakeToolCaller{results: []any{map[string]any{"canWrite": true, "canComment": true}}}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: true, EnableRealWrites: true})

	_, err := executor.Run(context.Background(), RunRequest{Skill: "comment", Inputs: map[string]any{}, DryRun: boolPtr(false)})
	assertSkillErrorCode(t, err, "permission_preflight_target_mismatch")
	if len(caller.calls) != 1 || caller.calls[0].tool != "feishu_doc_check_permission" {
		t.Fatalf("calls = %#v, want only mismatched permission preflight", caller.calls)
	}
}

func TestRealWriteRequiresPermissionPreflightCredentialMatch(t *testing.T) {
	manifest := Manifest{
		Name:         "comment",
		Write:        true,
		Inputs:       map[string]any{"type": "object"},
		Capabilities: []string{"doc.comment.create"},
		Steps: []Step{
			{Tool: "feishu_doc_check_permission", Args: map[string]any{"input": "doc-token", "credentialId": "cred-1"}},
			{Tool: "feishu_doc_create_comment", Args: map[string]any{"input": "doc-token", "credentialId": "cred-2", "content": "hi", "dryRun": false, "operationId": "op-1"}},
		},
	}
	caller := &fakeToolCaller{results: []any{map[string]any{"canWrite": true, "canComment": true}}}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: true, EnableRealWrites: true})

	_, err := executor.Run(context.Background(), RunRequest{Skill: "comment", Inputs: map[string]any{}, DryRun: boolPtr(false)})
	assertSkillErrorCode(t, err, "permission_preflight_target_mismatch")
	if len(caller.calls) != 1 || caller.calls[0].tool != "feishu_doc_check_permission" {
		t.Fatalf("calls = %#v, want only mismatched permission preflight", caller.calls)
	}
}

func TestRealWriteRequiresPermissionPreflightCredentialSymmetry(t *testing.T) {
	manifest := Manifest{
		Name:         "comment",
		Write:        true,
		Inputs:       map[string]any{"type": "object"},
		Capabilities: []string{"doc.comment.create"},
		Steps: []Step{
			{Tool: "feishu_doc_check_permission", Args: map[string]any{"input": "doc-token", "credentialId": "cred-1"}},
			{Tool: "feishu_doc_create_comment", Args: map[string]any{"input": "doc-token", "content": "hi", "dryRun": false, "operationId": "op-1"}},
		},
	}
	caller := &fakeToolCaller{results: []any{map[string]any{"canWrite": true, "canComment": true}}}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: true, EnableRealWrites: true})

	_, err := executor.Run(context.Background(), RunRequest{Skill: "comment", Inputs: map[string]any{}, DryRun: boolPtr(false)})
	assertSkillErrorCode(t, err, "permission_preflight_target_mismatch")
	if len(caller.calls) != 1 || caller.calls[0].tool != "feishu_doc_check_permission" {
		t.Fatalf("calls = %#v, want only credential-bound permission preflight", caller.calls)
	}
}

func TestRealCreateRequiresBoundPreflightForFolderTarget(t *testing.T) {
	manifest := Manifest{
		Name:         "create-doc",
		Write:        true,
		Inputs:       map[string]any{"type": "object"},
		Capabilities: []string{"doc.create"},
		Steps: []Step{
			{Tool: "feishu_doc_check_permission", Args: map[string]any{"input": "folder-A"}},
			{Tool: "feishu_doc_create", Args: map[string]any{"title": "New", "folderToken": "folder-B", "dryRun": false, "operationId": "op-1"}},
		},
	}
	caller := &fakeToolCaller{results: []any{map[string]any{"canWrite": true}}}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: true, EnableRealWrites: true})

	_, err := executor.Run(context.Background(), RunRequest{Skill: "create-doc", Inputs: map[string]any{}, DryRun: boolPtr(false)})
	assertSkillErrorCode(t, err, "permission_preflight_target_mismatch")
	if len(caller.calls) != 1 || caller.calls[0].tool != "feishu_doc_check_permission" {
		t.Fatalf("calls = %#v, want only mismatched permission preflight", caller.calls)
	}
}

func TestWriteCreateAllowsBoundPreflightForFolderTarget(t *testing.T) {
	manifest := Manifest{
		Name:         "create-doc",
		Write:        true,
		Inputs:       map[string]any{"type": "object"},
		Capabilities: []string{"doc.create"},
		Steps: []Step{
			{Tool: "feishu_doc_check_permission", Args: map[string]any{"input": "folder-A", "credentialId": "cred-1"}},
			{Tool: "feishu_doc_create", Args: map[string]any{"title": "New", "folderToken": "folder-A", "credentialId": "cred-1", "dryRun": false, "operationId": "op-1"}},
		},
	}
	caller := &fakeToolCaller{results: []any{map[string]any{"canWrite": true}, map[string]any{"documentId": "doc-1"}}}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: true, EnableRealWrites: true})

	_, err := executor.Run(context.Background(), RunRequest{Skill: "create-doc", Inputs: map[string]any{}, DryRun: boolPtr(false)})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(caller.calls) != 2 || caller.calls[1].tool != "feishu_doc_create" {
		t.Fatalf("calls = %#v, want permission preflight then create", caller.calls)
	}
}

func TestWriteCreateWithoutFolderTargetFailsClosed(t *testing.T) {
	manifest := Manifest{
		Name:         "create-doc",
		Write:        true,
		Inputs:       map[string]any{"type": "object"},
		Capabilities: []string{"doc.create"},
		Steps: []Step{
			{Tool: "feishu_doc_check_permission", Args: map[string]any{"input": "root"}},
			{Tool: "feishu_doc_create", Args: map[string]any{"title": "New", "dryRun": false, "operationId": "op-1"}},
		},
	}
	caller := &fakeToolCaller{results: []any{map[string]any{"canWrite": true}}}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: true, EnableRealWrites: true})

	_, err := executor.Run(context.Background(), RunRequest{Skill: "create-doc", Inputs: map[string]any{}, DryRun: boolPtr(false)})
	assertSkillErrorCode(t, err, "permission_preflight_target_mismatch")
	if len(caller.calls) != 1 || caller.calls[0].tool != "feishu_doc_check_permission" {
		t.Fatalf("calls = %#v, want only permission preflight for unbound root/default create", caller.calls)
	}
}

func TestWriteSkillWithoutWriteCapabilityRejectedBeforeMutation(t *testing.T) {
	manifest := writeCommentManifest(map[string]any{"input": "doc-token", "content": "hi", "dryRun": false, "operationId": "op-1"})
	manifest.Capabilities = []string{"doc.read"}
	caller := &fakeToolCaller{}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: true, EnableRealWrites: true})

	_, err := executor.Run(context.Background(), RunRequest{Skill: "comment", Inputs: map[string]any{}, DryRun: boolPtr(false)})
	assertSkillErrorCode(t, err, "write_capability_required")
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want no unsafe downstream calls", caller.calls)
	}
}

func TestWriteSkillWithoutPermissionPreflightRejectedBeforeMutation(t *testing.T) {
	manifest := Manifest{Name: "comment", Write: true, Inputs: map[string]any{"type": "object"}, Capabilities: []string{"doc.comment.create"}, Steps: []Step{{Tool: "feishu_doc_create_comment", Args: map[string]any{"input": "doc-token", "content": "hi", "dryRun": false, "operationId": "op-1"}}}}
	caller := &fakeToolCaller{}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: true, EnableRealWrites: true})

	_, err := executor.Run(context.Background(), RunRequest{Skill: "comment", Inputs: map[string]any{}, DryRun: boolPtr(false)})
	assertSkillErrorCode(t, err, "permission_preflight_required")
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want no unsafe downstream calls", caller.calls)
	}
}

func TestFailingPermissionPreflightStopsLaterWriteSteps(t *testing.T) {
	manifest := writeCommentManifest(map[string]any{"input": "doc-token", "content": "hi", "dryRun": false, "operationId": "op-1"})
	caller := &fakeToolCaller{results: []any{map[string]any{"canWrite": true, "canComment": false}}}
	executor := NewExecutorWithOptions(fakeExecutorRegistry{manifest: manifest}, caller, ExecutorOptions{EnableWrite: true, EnableRealWrites: true})

	_, err := executor.Run(context.Background(), RunRequest{Skill: "comment", Inputs: map[string]any{}, DryRun: boolPtr(false)})
	assertSkillErrorCode(t, err, "permission_preflight_denied")
	if len(caller.calls) != 1 || caller.calls[0].tool != "feishu_doc_check_permission" {
		t.Fatalf("calls = %#v, want only permission preflight", caller.calls)
	}
}

func boolPtr(value bool) *bool { return &value }

func writeCommentManifest(args map[string]any) Manifest {
	return Manifest{
		Name:         "comment",
		Write:        true,
		Inputs:       map[string]any{"type": "object"},
		Capabilities: []string{"doc.comment.create"},
		Steps: []Step{
			{Tool: "feishu_doc_check_permission", Args: map[string]any{"input": "doc-token"}},
			{Tool: "feishu_doc_create_comment", Args: args},
		},
	}
}

func assertSkillErrorCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want code %q", wantCode)
	}
	var skillErr SkillError
	if !errors.As(err, &skillErr) {
		t.Fatalf("error = %T %[1]v, want SkillError", err)
	}
	if skillErr.Code != wantCode {
		t.Fatalf("error code = %q, want %q (err=%v)", skillErr.Code, wantCode, err)
	}
}

type fakeExecutorRegistry struct {
	manifest Manifest
	ok       bool
}

func (r fakeExecutorRegistry) Get(name string) (Manifest, bool) {
	if r.ok || r.manifest.Name == name {
		return r.manifest, true
	}
	return Manifest{}, false
}

type fakeToolCaller struct {
	calls   []fakeToolCall
	results []any
	err     error
}

type fakeToolCall struct {
	tool string
	args map[string]any
}

func (c *fakeToolCaller) CallTool(ctx context.Context, tool string, args map[string]any) (any, error) {
	c.calls = append(c.calls, fakeToolCall{tool: tool, args: args})
	if c.err != nil {
		return nil, c.err
	}
	if len(c.results) >= len(c.calls) {
		return c.results[len(c.calls)-1], nil
	}
	return map[string]any{"ok": true}, nil
}
