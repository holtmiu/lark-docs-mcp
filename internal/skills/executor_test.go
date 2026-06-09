package skills

import (
	"context"
	"encoding/json"
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

	got, err := executor.Run(context.Background(), RunRequest{Skill: "read-doc", Inputs: map[string]any{"input": "https://example.test/doc"}, DryRun: true})
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

	got, err := executor.Run(context.Background(), RunRequest{Skill: "inspect", Inputs: map[string]any{"unused": "value"}, DryRun: false})
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
}

type fakeToolCall struct {
	tool string
	args map[string]any
}

func (c *fakeToolCaller) CallTool(ctx context.Context, tool string, args map[string]any) (any, error) {
	c.calls = append(c.calls, fakeToolCall{tool: tool, args: args})
	if len(c.results) >= len(c.calls) {
		return c.results[len(c.calls)-1], nil
	}
	return map[string]any{"ok": true}, nil
}
