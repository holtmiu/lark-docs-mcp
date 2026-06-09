package skills

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuiltInSkillsDirectoryLoadsAndDeclaresSafeWorkflows(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	repoSkillsDir := filepath.Join(filepath.Dir(filepath.Dir(wd)), "skills")
	registry, err := LoadRegistryWithOptions([]string{repoSkillsDir}, RegistryOptions{EnableWrite: true})
	if err != nil {
		t.Fatalf("LoadRegistryWithOptions(repo skills) returned error: %v", err)
	}

	gotNames := manifestNames(registry.List())
	wantNames := []string{"add-review-comment", "create-draft-doc", "export-doc-markdown"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("built-in skill names = %#v, want %#v", gotNames, wantNames)
	}

	exportDoc := mustGetSkill(t, registry, "export-doc-markdown")
	if exportDoc.Write {
		t.Fatal("export-doc-markdown Write = true, want read-only")
	}
	assertCapabilities(t, exportDoc, []string{"doc.metadata", "doc.read"})
	assertHasStep(t, exportDoc, "feishu_doc_get_metadata")
	assertHasStep(t, exportDoc, "feishu_doc_read")

	createDraft := mustGetSkill(t, registry, "create-draft-doc")
	assertWriteSkillSafety(t, createDraft, []string{"doc.create", "doc.permission.check"}, "feishu_doc_create", "folderToken")

	addComment := mustGetSkill(t, registry, "add-review-comment")
	assertWriteSkillSafety(t, addComment, []string{"doc.comment.create", "doc.permission.check"}, "feishu_doc_create_comment", "input")

	assertBuiltInWriteSkillDryRunCompatible(t, registry, "create-draft-doc", map[string]any{"folderToken": "folder-token", "title": "Draft title", "markdown": "# Draft"})
	assertBuiltInWriteSkillDryRunCompatible(t, registry, "add-review-comment", map[string]any{"input": "doc-token", "content": "Please review this section."})
}

func assertBuiltInWriteSkillDryRunCompatible(t *testing.T, registry Registry, name string, inputs map[string]any) {
	t.Helper()
	caller := &fakeToolCaller{results: []any{map[string]any{"canWrite": true, "canComment": true}, map[string]any{"dryRun": true}}}
	executor := NewExecutorWithOptions(registry, caller, ExecutorOptions{EnableWrite: true})
	result, err := executor.Run(context.Background(), RunRequest{Skill: name, Inputs: inputs})
	if err != nil {
		t.Fatalf("dry-run %s returned error: %v", name, err)
	}
	if !result.DryRun {
		t.Fatalf("dry-run %s result DryRun = false, want true", name)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("dry-run %s made %d calls, want permission preflight and mutation", name, len(caller.calls))
	}
	if caller.calls[1].args["dryRun"] != true {
		t.Fatalf("dry-run %s write call dryRun = %#v, want true", name, caller.calls[1].args["dryRun"])
	}
}

func mustGetSkill(t *testing.T, registry Registry, name string) Manifest {
	t.Helper()
	manifest, ok := registry.Get(name)
	if !ok {
		t.Fatalf("registry.Get(%q) missing", name)
	}
	return manifest
}

func assertCapabilities(t *testing.T, manifest Manifest, want []string) {
	t.Helper()
	if !reflect.DeepEqual(manifest.Capabilities, want) {
		t.Fatalf("%s capabilities = %#v, want %#v", manifest.Name, manifest.Capabilities, want)
	}
}

func assertHasStep(t *testing.T, manifest Manifest, tool string) Step {
	t.Helper()
	for _, step := range manifest.Steps {
		if step.Tool == tool {
			return step
		}
	}
	t.Fatalf("%s missing step tool %s in %#v", manifest.Name, tool, manifest.Steps)
	return Step{}
}

func assertWriteSkillSafety(t *testing.T, manifest Manifest, capabilities []string, writeTool, permissionTargetArg string) {
	t.Helper()
	if !manifest.Write {
		t.Fatalf("%s Write = false, want true", manifest.Name)
	}
	assertCapabilities(t, manifest, capabilities)
	if len(manifest.Steps) < 2 {
		t.Fatalf("%s has %d steps, want permission preflight before mutation", manifest.Name, len(manifest.Steps))
	}
	preflight := manifest.Steps[0]
	if preflight.Tool != "feishu_doc_check_permission" {
		t.Fatalf("%s first step = %s, want feishu_doc_check_permission", manifest.Name, preflight.Tool)
	}
	writeStep := assertHasStep(t, manifest, writeTool)
	if writeStep.Args == nil {
		t.Fatalf("%s write step args are nil", manifest.Name)
	}
	if got, exists := writeStep.Args["dryRun"]; exists && got != true {
		t.Fatalf("%s %s dryRun arg = %#v, want omitted for executor default or literal true", manifest.Name, writeTool, got)
	}
	if got, want := writeStep.Args["operationId"], "${operationId}"; got != want {
		t.Fatalf("%s %s operationId arg = %#v, want %#v", manifest.Name, writeTool, got, want)
	}
	preflightTarget, ok := preflight.Args["input"]
	if !ok {
		t.Fatalf("%s permission preflight missing input arg", manifest.Name)
	}
	if writeStep.Args[permissionTargetArg] != preflightTarget {
		t.Fatalf("%s permission target = %#v, write %s = %#v; want same interpolation", manifest.Name, preflightTarget, permissionTargetArg, writeStep.Args[permissionTargetArg])
	}
}
