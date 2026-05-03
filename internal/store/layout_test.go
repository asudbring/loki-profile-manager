package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureLayoutCreatesValidStoreFromEmptyFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "loki")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	result, err := EnsureLayout(root)
	if err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	if !result.Created || !result.Valid {
		t.Fatalf("result = %+v, want created valid", result)
	}
	validation := ValidateLayout(root)
	if !validation.Valid {
		t.Fatalf("layout invalid: %+v", validation)
	}
}

func TestEnsureLayoutCreatesBaseManifestsAndRegistry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	for _, path := range []string{
		filepath.Join(root, "profiles", "common", "manifest.yaml"),
		filepath.Join(root, "profiles", "work", "core", "manifest.yaml"),
		filepath.Join(root, "profiles", "dev", "core", "manifest.yaml"),
		filepath.Join(root, "profiles", "writer", "core", "manifest.yaml"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		if !strings.Contains(string(content), "version: 1") || !strings.Contains(string(content), "files: []") {
			t.Fatalf("manifest %s has unexpected content: %s", path, content)
		}
	}
	content, err := os.ReadFile(filepath.Join(root, "registry", "machines.json"))
	if err != nil {
		t.Fatalf("read machines.json error = %v", err)
	}
	var registry struct {
		Version  int             `json:"version"`
		Machines json.RawMessage `json:"machines"`
	}
	if err := json.Unmarshal(content, &registry); err != nil {
		t.Fatalf("machines.json invalid: %v", err)
	}
	if registry.Version != 1 || string(registry.Machines) != "[]" {
		t.Fatalf("unexpected registry: %s", content)
	}
}

func TestEnsureLayoutPreservesExistingValidStore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	manifestPath := filepath.Join(root, "profiles", "common", "manifest.yaml")
	custom := []byte("version: 1\nname: custom-common\n")
	if err := os.WriteFile(manifestPath, custom, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	result, err := EnsureLayout(root)
	if err != nil {
		t.Fatalf("EnsureLayout() second error = %v", err)
	}
	if result.Created || !result.Valid {
		t.Fatalf("result = %+v, want preserved valid", result)
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != string(custom) {
		t.Fatalf("manifest overwritten: %s", content)
	}
}

func TestEnsureLayoutInvalidNonEmptyStoreReportsMissing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "loki")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("not a store"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	result, err := EnsureLayout(root)
	if err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	if result.Created || result.Valid || len(result.Missing) == 0 {
		t.Fatalf("result = %+v, want invalid missing", result)
	}
}
