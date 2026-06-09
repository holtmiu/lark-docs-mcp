package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Registry is a read-only collection of validated local skill manifests.
type Registry struct {
	manifests map[string]Manifest
	sources   map[string]string
}

// RegistryOptions controls which local skill manifests may be exposed.
type RegistryOptions struct {
	// EnableWrite must be explicitly true before write-capable skills are loaded.
	EnableWrite bool
}

// LoadRegistry loads validated skill manifests from local directories.
func LoadRegistry(dirs []string) (Registry, error) {
	return LoadRegistryWithOptions(dirs, RegistryOptions{})
}

// LoadRegistryWithOptions loads validated skill manifests from local directories using an explicit policy.
func LoadRegistryWithOptions(dirs []string, options RegistryOptions) (Registry, error) {
	registry := Registry{manifests: make(map[string]Manifest), sources: make(map[string]string)}
	roots := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		root, err := validateRegistryRoot(dir)
		if err != nil {
			return Registry{}, err
		}
		roots = append(roots, root)
	}
	sort.Strings(roots)

	for _, root := range roots {
		if err := loadRegistryRoot(root, registry.manifests, registry.sources, options); err != nil {
			return Registry{}, err
		}
	}
	return registry, nil
}

// List returns registry manifests sorted by skill name for deterministic responses.
func (r Registry) List() []Manifest {
	names := make([]string, 0, len(r.manifests))
	for name := range r.manifests {
		names = append(names, name)
	}
	sort.Strings(names)

	manifests := make([]Manifest, 0, len(names))
	for _, name := range names {
		manifests = append(manifests, r.manifests[name])
	}
	return manifests
}

// Get returns a skill manifest by name.
func (r Registry) Get(name string) (Manifest, bool) {
	manifest, ok := r.manifests[name]
	return manifest, ok
}

func validateRegistryRoot(dir string) (string, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return "", fmt.Errorf("skill registry directory is required")
	}
	if strings.Contains(trimmed, "://") {
		return "", fmt.Errorf("skill registry directory %q must be a local path; remote URLs are not allowed", trimmed)
	}
	for _, part := range strings.FieldsFunc(trimmed, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return "", fmt.Errorf("skill registry directory %q must not contain .. path traversal", trimmed)
		}
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve skill registry directory %q: %w", trimmed, err)
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve skill registry directory %q: %w", trimmed, err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("stat skill registry directory %q: %w", trimmed, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("skill registry path %q is not a directory", trimmed)
	}
	return filepath.Clean(root), nil
}

func loadRegistryRoot(root string, manifests map[string]Manifest, sources map[string]string, options RegistryOptions) error {
	paths := make([]string, 0)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && entry.IsDir() && entry.Type()&os.ModeSymlink != 0 {
			// Intentionally skip symlinked directories so a registry cannot walk outside its root.
			return filepath.SkipDir
		}
		if entry.IsDir() || !isManifestFilename(entry.Name()) {
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		return fmt.Errorf("walk skill registry directory %q: %w", root, err)
	}
	sort.Strings(paths)

	for _, path := range paths {
		manifestPath, ok, err := safeManifestPath(root, path)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return fmt.Errorf("read skill manifest %q: %w", path, err)
		}
		manifest, err := ParseManifest(data)
		if err != nil {
			return fmt.Errorf("parse skill manifest %q: %w", path, err)
		}
		if manifest.Write && !options.EnableWrite {
			return fmt.Errorf("skill manifest %q declares write: true but registry write policy is disabled; set RegistryOptions.EnableWrite=true only for trusted write-capable skills", path)
		}
		if _, exists := manifests[manifest.Name]; exists {
			return fmt.Errorf("duplicate skill name %q in %q conflicts with %q", manifest.Name, path, sources[manifest.Name])
		}
		manifests[manifest.Name] = manifest
		sources[manifest.Name] = path
	}
	return nil
}

func isManifestFilename(name string) bool {
	return name == "skill.yaml" || name == "skill.yml" || strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

func safeManifestPath(root, path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", false, fmt.Errorf("stat skill manifest %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, true, nil
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false, fmt.Errorf("resolve skill manifest symlink %q: %w", path, err)
	}
	inside, err := pathWithin(root, realPath)
	if err != nil {
		return "", false, err
	}
	if !inside {
		// Intentional safety skip: ignore manifest symlinks that resolve outside the registry root.
		return "", false, nil
	}
	return realPath, true, nil
}

func pathWithin(root, path string) (bool, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false, fmt.Errorf("resolve registry root %q: %w", root, err)
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("resolve manifest path %q: %w", path, err)
	}
	rootAbs = filepath.Clean(rootAbs)
	pathAbs = filepath.Clean(pathAbs)
	if pathAbs == rootAbs {
		return true, nil
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false, fmt.Errorf("compare manifest path %q to registry root %q: %w", path, root, err)
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)), nil
}
