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
	writeSkillManifest(t, filepath.Join(root, "one", "skill.yaml"), "duplicate")
	writeSkillManifest(t, filepath.Join(root, "two", "skill.yaml"), "duplicate")

	_, err := LoadRegistry([]string{root})
	if err == nil {
		t.Fatal("LoadRegistry succeeded, want duplicate skill name error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error = %q, want mention duplicate", err.Error())
	}
}

func TestRegistryIgnoresNonManifestFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "# not a skill\n")
	writeFile(t, filepath.Join(root, "not-skill.yaml"), "name: invalid\n")
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

func TestRegistrySkipsSymlinkEscapes(t *testing.T) {
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
