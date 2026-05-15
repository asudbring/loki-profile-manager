package storemigrate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/asudbring/loki-profile-manager/internal/store"
)

func TestCopyPlanCopiesStoreAndValidatesDestination(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	contentPath := filepath.Join(source, "profiles", "work", "core", "files", "settings.json")
	if err := os.WriteFile(contentPath, []byte(`{"copied":true}`), 0o644); err != nil {
		t.Fatalf("WriteFile(content) error = %v", err)
	}
	dest := filepath.Join(t.TempDir(), "dest")
	plan, err := BuildPlan(PlanOptions{FromPath: source, ToPath: dest, Provider: store.ProviderOneDriveBusiness})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	result, err := CopyPlan(plan)
	if err != nil {
		t.Fatalf("CopyPlan() error = %v", err)
	}
	if result.CopiedFiles == 0 || !result.Valid {
		t.Fatalf("copy result = %+v", result)
	}
	if got := mustReadStoreMigrateTest(t, filepath.Join(dest, "profiles", "work", "core", "files", "settings.json")); string(got) != `{"copied":true}` {
		t.Fatalf("copied content = %q", got)
	}
	if validation := store.ValidateLayout(dest); !validation.Valid {
		t.Fatalf("destination layout invalid: %+v", validation)
	}
}

func TestCopyPlanRechecksDestinationStillEmpty(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	dest := filepath.Join(t.TempDir(), "dest")
	plan, err := BuildPlan(PlanOptions{FromPath: source, ToPath: dest})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll(dest) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, "raced.txt"), []byte("race"), 0o644); err != nil {
		t.Fatalf("WriteFile(raced) error = %v", err)
	}
	_, err = CopyPlan(plan)
	if err == nil || !strings.Contains(err.Error(), "destination must be missing or empty") {
		t.Fatalf("CopyPlan(raced destination) error = %v", err)
	}
}

func TestCopyPlanRechecksSymlinkedDestinationParent(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	parent := filepath.Join(t.TempDir(), "dest-parent")
	dest := filepath.Join(parent, "dest")
	plan, err := BuildPlan(PlanOptions{FromPath: source, ToPath: dest})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if err := os.Symlink(source, parent); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err = CopyPlan(plan)
	if err == nil || !strings.Contains(err.Error(), "destination cannot be inside source") {
		t.Fatalf("CopyPlan(symlinked nested dest) error = %v", err)
	}
}

func TestCopyPlanPreservesSymlinkWhenSupported(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	target := filepath.Join(source, "profiles", "work", "core", "files", "target.txt")
	if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	link := filepath.Join(source, "profiles", "work", "core", "files", "link.txt")
	if err := os.Symlink("target.txt", link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	dest := filepath.Join(t.TempDir(), "dest")
	plan, err := BuildPlan(PlanOptions{FromPath: source, ToPath: dest})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if _, err := CopyPlan(plan); err != nil {
		t.Fatalf("CopyPlan() error = %v", err)
	}
	got, err := os.Readlink(filepath.Join(dest, "profiles", "work", "core", "files", "link.txt"))
	if err != nil {
		t.Fatalf("Readlink(copied) error = %v", err)
	}
	if got != "target.txt" {
		t.Fatalf("copied symlink = %q", got)
	}
}

func mustReadStoreMigrateTest(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return content
}
