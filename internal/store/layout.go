package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func NewLayout(root string) Layout {
	root = filepath.Clean(root)
	return Layout{
		Root:         root,
		RegistryDir:  filepath.Join(root, "registry"),
		MachinesFile: filepath.Join(root, "registry", "machines.json"),
		MachinesDir:  filepath.Join(root, "registry", "machines"),
		ProfilesDir:  filepath.Join(root, "profiles"),
		ConflictsDir: filepath.Join(root, "conflicts"),
		SnapshotsDir: filepath.Join(root, "snapshots"),
		LogsDir:      filepath.Join(root, "logs"),
	}
}

func EnsureLayout(root string) (EnsureResult, error) {
	if root == "" {
		return EnsureResult{}, fmt.Errorf("ensure store layout: store root is required")
	}
	layout := NewLayout(root)

	exists, isEmpty, err := rootState(layout.Root)
	if err != nil {
		return EnsureResult{}, err
	}
	if !exists || isEmpty {
		if err := createLayout(layout); err != nil {
			return EnsureResult{}, err
		}
		validation := ValidateLayout(layout.Root)
		return EnsureResult{Layout: layout, Created: true, Valid: validation.Valid, Missing: validation.Missing}, nil
	}

	validation := ValidateLayout(layout.Root)
	if validation.Valid {
		return EnsureResult{Layout: layout, Created: false, Valid: true}, nil
	}
	return EnsureResult{Layout: layout, Created: false, Valid: false, Missing: validation.Missing}, nil
}

func InspectLayout(root string) (InspectionResult, error) {
	if root == "" {
		return InspectionResult{}, fmt.Errorf("inspect store layout: store root is required")
	}
	layout := NewLayout(root)
	info, err := os.Stat(layout.Root)
	if os.IsNotExist(err) {
		return InspectionResult{Exists: false, Empty: true, IsDir: false, Valid: false}, nil
	}
	if err != nil {
		return InspectionResult{}, fmt.Errorf("stat store root %s: %w", layout.Root, err)
	}
	if !info.IsDir() {
		return InspectionResult{Exists: true, Empty: false, IsDir: false, Valid: false}, fmt.Errorf("store root %s is not a directory", layout.Root)
	}
	entries, err := os.ReadDir(layout.Root)
	if err != nil {
		return InspectionResult{}, fmt.Errorf("read store root %s: %w", layout.Root, err)
	}
	validation := ValidateLayout(layout.Root)
	return InspectionResult{Exists: true, Empty: len(entries) == 0, IsDir: true, Valid: validation.Valid, Missing: validation.Missing}, nil
}

func ValidateLayout(root string) ValidationResult {
	layout := NewLayout(root)
	missing := []string{}
	for _, path := range requiredDirs(layout) {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			missing = append(missing, path)
		}
	}
	for _, path := range requiredFiles(layout) {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			missing = append(missing, path)
		}
	}
	return ValidationResult{Valid: len(missing) == 0, Missing: missing}
}

func createLayout(layout Layout) error {
	for _, path := range requiredDirs(layout) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create store directory %s: %w", path, err)
		}
	}
	if err := writeFileIfMissing(layout.MachinesFile, defaultMachinesJSON()); err != nil {
		return err
	}
	for path, content := range baseManifests(layout) {
		if err := writeFileIfMissing(path, []byte(content)); err != nil {
			return err
		}
	}
	return nil
}

func rootState(root string) (exists bool, isEmpty bool, err error) {
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return false, true, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("stat store root %s: %w", root, err)
	}
	if !info.IsDir() {
		return true, false, fmt.Errorf("store root %s is not a directory", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return true, false, fmt.Errorf("read store root %s: %w", root, err)
	}
	return true, len(entries) == 0, nil
}

func requiredDirs(layout Layout) []string {
	dirs := []string{
		layout.Root,
		layout.RegistryDir,
		layout.MachinesDir,
		layout.ProfilesDir,
		layout.ConflictsDir,
		layout.SnapshotsDir,
		layout.LogsDir,
		filepath.Join(layout.ProfilesDir, "common"),
		filepath.Join(layout.ProfilesDir, "common", "files"),
		filepath.Join(layout.ProfilesDir, "common", "skills"),
		filepath.Join(layout.ProfilesDir, "common", "templates"),
	}
	for _, profile := range []string{"work", "dev", "writer"} {
		dirs = append(dirs,
			filepath.Join(layout.ProfilesDir, profile),
			filepath.Join(layout.ProfilesDir, profile, "core"),
			filepath.Join(layout.ProfilesDir, profile, "core", "files"),
			filepath.Join(layout.ProfilesDir, profile, "core", "skills"),
			filepath.Join(layout.ProfilesDir, profile, "core", "templates"),
			filepath.Join(layout.ProfilesDir, profile, "buckets"),
		)
	}
	return dirs
}

func requiredFiles(layout Layout) []string {
	return []string{
		layout.MachinesFile,
		filepath.Join(layout.ProfilesDir, "common", "manifest.yaml"),
		filepath.Join(layout.ProfilesDir, "work", "core", "manifest.yaml"),
		filepath.Join(layout.ProfilesDir, "dev", "core", "manifest.yaml"),
		filepath.Join(layout.ProfilesDir, "writer", "core", "manifest.yaml"),
	}
}

func baseManifests(layout Layout) map[string]string {
	return map[string]string{
		filepath.Join(layout.ProfilesDir, "common", "manifest.yaml"):         baseManifest("common"),
		filepath.Join(layout.ProfilesDir, "work", "core", "manifest.yaml"):   baseManifest("work-core"),
		filepath.Join(layout.ProfilesDir, "dev", "core", "manifest.yaml"):    baseManifest("dev-core"),
		filepath.Join(layout.ProfilesDir, "writer", "core", "manifest.yaml"): baseManifest("writer-core"),
	}
}

func baseManifest(name string) string {
	return fmt.Sprintf("version: 1\nname: %s\nfiles: []\nskills: []\nignore: []\nmerge_rules: {}\ntargets: {}\n", name)
}

func defaultMachinesJSON() []byte {
	content, _ := json.MarshalIndent(struct {
		Version  int           `json:"version"`
		Machines []interface{} `json:"machines"`
	}{Version: RegistryVersion, Machines: []interface{}{}}, "", "  ")
	return append(content, '\n')
}

func writeFileIfMissing(path string, content []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
