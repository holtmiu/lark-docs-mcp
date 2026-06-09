package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestRegistryLoadsMultipleManifestsFromTempDirectories(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeSkillManifest(t, filepath.Join(rootA, "alpha", "skill.yaml"), "alpha")
	writeSkillManifest(t, filepath.Join(rootB, "beta", "skill.yml"), "beta")

	registry, err := LoadRegistry([]string{rootB, rootA})
	if err != nil {
		t.Fatalf("LoadRegistry returned error: %v", err)
	}

	got := manifestNames(registry.List())
	want := []string{"alpha", "beta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("registry.List names = %#v, want sorted %#v", got, want)
	}
	if manifest, ok := registry.Get("alpha"); !ok || manifest.Name != "alpha" {
		t.Fatalf("registry.Get(alpha) = %#v, %v; want alpha manifest", manifest, ok)
	}
}

func TestRegistryRejectsDuplicateSkillNames(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "one", "skill.yaml")
	secondPath := filepath.Join(root, "two", "skill.yaml")
	writeSkillManifest(t, firstPath, "duplicate")
	writeSkillManifest(t, secondPath, "duplicate")

	_, err := LoadRegistry([]string{root})
	if err == nil {
		t.Fatal("LoadRegistry succeeded, want duplicate skill name error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error = %q, want mention duplicate", err.Error())
	}
	if !strings.Contains(err.Error(), firstPath) || !strings.Contains(err.Error(), secondPath) {
		t.Fatalf("error = %q, want conflicting manifest paths %q and %q", err.Error(), firstPath, secondPath)
	}
}

func TestRegistryRejectsWriteSkillByDefaultFailClosed(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "writer", "skill.yaml")
	writeWriteSkillManifest(t, manifestPath, "writer")

	_, err := LoadRegistry([]string{root})
	if err == nil {
		t.Fatal("LoadRegistry succeeded, want write policy error")
	}
	if !strings.Contains(err.Error(), "write") || !strings.Contains(err.Error(), manifestPath) || !strings.Contains(err.Error(), "EnableWrite") {
		t.Fatalf("error = %q, want actionable write policy error with manifest path and EnableWrite hint", err.Error())
	}
}

func TestRegistryAllowsWriteSkillWithExplicitOption(t *testing.T) {
	root := t.TempDir()
	writeWriteSkillManifest(t, filepath.Join(root, "writer", "skill.yaml"), "writer")

	registry, err := LoadRegistryWithOptions([]string{root}, RegistryOptions{EnableWrite: true})
	if err != nil {
		t.Fatalf("LoadRegistryWithOptions returned error: %v", err)
	}
	if manifest, ok := registry.Get("writer"); !ok || !manifest.Write {
		t.Fatalf("registry.Get(writer) = %#v, %v; want write manifest", manifest, ok)
	}
}

func TestRegistryIgnoresNonManifestFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "# not a skill\n")
	writeFile(t, filepath.Join(root, "notes.txt"), "name: invalid\n")
	writeSkillManifest(t, filepath.Join(root, "valid", "skill.yaml"), "valid")

	registry, err := LoadRegistry([]string{root})
	if err != nil {
		t.Fatalf("LoadRegistry returned error: %v", err)
	}
	got := manifestNames(registry.List())
	want := []string{"valid"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("registry.List names = %#v, want %#v", got, want)
	}
}

func TestRegistryLoadsNamedYAMLManifestFiles(t *testing.T) {
	root := t.TempDir()
	writeSkillManifest(t, filepath.Join(root, "export-doc-markdown.yaml"), "export-doc-markdown")
	writeSkillManifest(t, filepath.Join(root, "nested", "create-draft-doc.yml"), "create-draft-doc")

	registry, err := LoadRegistry([]string{root})
	if err != nil {
		t.Fatalf("LoadRegistry returned error: %v", err)
	}
	got := manifestNames(registry.List())
	want := []string{"create-draft-doc", "export-doc-markdown"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("registry.List names = %#v, want %#v", got, want)
	}
}

func TestRegistryRejectsInvalidYAMLManifestFiles(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "not-a-skill.yaml")
	writeFile(t, manifestPath, "name: invalid\n")

	_, err := LoadRegistry([]string{root})
	if err == nil {
		t.Fatal("LoadRegistry succeeded, want invalid manifest error for YAML manifest file")
	}
	if !strings.Contains(err.Error(), manifestPath) || !strings.Contains(err.Error(), "inputs schema") {
		t.Fatalf("error = %q, want actionable invalid manifest error mentioning %q", err.Error(), manifestPath)
	}
}

func TestRegistryBlocksDotDotPathTraversalInputs(t *testing.T) {
	root := t.TempDir()
	writeSkillManifest(t, filepath.Join(root, "skill.yaml"), "blocked")

	_, err := LoadRegistry([]string{root + string(os.PathSeparator) + ".." + string(os.PathSeparator) + filepath.Base(root)})
	if err == nil {
		t.Fatal("LoadRegistry succeeded, want path traversal error")
	}
	if !strings.Contains(err.Error(), "..") {
		t.Fatalf("error = %q, want mention ..", err.Error())
	}
}

func TestRegistryRejectsRemoteURLInputs(t *testing.T) {
	for _, input := range []string{"https://example.com/skills", "file:///tmp/skills"} {
		t.Run(input, func(t *testing.T) {
			_, err := LoadRegistry([]string{input})
			if err == nil {
				t.Fatal("LoadRegistry succeeded, want remote URL rejection")
			}
			if !strings.Contains(err.Error(), "local path") || !strings.Contains(err.Error(), input) {
				t.Fatalf("error = %q, want actionable local path rejection for %q", err.Error(), input)
			}
		})
	}
}

func TestRegistryIntentionallySkipsSymlinkEscapesForSafety(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows environments")
	}
	root := t.TempDir()
	outside := t.TempDir()
	writeSkillManifest(t, filepath.Join(outside, "skill.yaml"), "escaped")
	if err := os.Symlink(filepath.Join(outside, "skill.yaml"), filepath.Join(root, "skill.yaml")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	registry, err := LoadRegistry([]string{root})
	if err != nil {
		t.Fatalf("LoadRegistry returned error: %v", err)
	}
	if got := manifestNames(registry.List()); len(got) != 0 {
		t.Fatalf("registry.List names = %#v, want symlink escape skipped", got)
	}
}

func manifestNames(manifests []Manifest) []string {
	names := make([]string, 0, len(manifests))
	for _, manifest := range manifests {
		names = append(names, manifest.Name)
	}
	return names
}

func writeSkillManifest(t *testing.T, path, name string) {
	t.Helper()
	writeFile(t, path, strings.ReplaceAll(validManifest(t), "summarize_doc_for_review", name))
}

func writeWriteSkillManifest(t *testing.T, path, name string) {
	t.Helper()
	manifest := strings.ReplaceAll(validManifest(t), "summarize_doc_for_review", name)
	manifest = strings.Replace(manifest, "- doc.read", "- doc.comment.create", 1)
	manifest = strings.Replace(manifest, "write: false", "write: true", 1)
	manifest = strings.Replace(manifest, "tool: feishu_doc_read", "tool: feishu_doc_create_comment", 1)
	writeFile(t, path, manifest)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
