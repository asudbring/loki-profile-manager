package storemigrate

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asudbring/loki-profile-manager/internal/store"
)

func TestDatalessFlagSet(t *testing.T) {
	if !datalessFlagSet(darwinDatalessFlag) {
		t.Fatal("dataless flag not detected")
	}
	if datalessFlagSet(0x20) {
		t.Fatal("compressed-only flag detected as dataless")
	}
}

func TestBuildPlanRequiresValidSourceAndEmptyDestination(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll(dest) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, "junk.txt"), []byte("junk"), 0o644); err != nil {
		t.Fatalf("WriteFile(junk) error = %v", err)
	}

	_, err := BuildPlan(PlanOptions{FromPath: source, ToPath: dest, Provider: store.ProviderDropbox})
	if err == nil || !strings.Contains(err.Error(), "destination must be missing or empty") {
		t.Fatalf("BuildPlan(non-empty dest) error = %v", err)
	}
}

func TestBuildPlanRejectsNestedDestination(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	dest := filepath.Join(source, "nested")

	_, err := BuildPlan(PlanOptions{FromPath: source, ToPath: dest})
	if err == nil || !strings.Contains(err.Error(), "destination cannot be inside source") {
		t.Fatalf("BuildPlan(nested dest) error = %v", err)
	}
}

func TestBuildPlanRejectsCaseVariantNestedDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "Source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	dest := filepath.Join(root, "source", "nested")
	_, err := BuildPlan(PlanOptions{FromPath: source, ToPath: dest})
	if err == nil || !strings.Contains(err.Error(), "destination cannot be inside source") {
		t.Fatalf("BuildPlan(case-variant nested dest) error = %v", err)
	}
}

func TestBuildPlanRejectsSymlinkedDestinationInsideSource(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	linkRoot := filepath.Join(t.TempDir(), "link-to-source")
	if err := os.Symlink(source, linkRoot); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	dest := filepath.Join(linkRoot, "nested-copy")
	_, err := BuildPlan(PlanOptions{FromPath: source, ToPath: dest})
	if err == nil || !strings.Contains(err.Error(), "destination cannot be inside source") {
		t.Fatalf("BuildPlan(symlinked nested dest) error = %v", err)
	}
}

func TestBuildPlanCountsFilesAndExcludesOperationLock(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	custom := filepath.Join(source, "profiles", "work", "core", "files", "settings.json")
	if err := os.WriteFile(custom, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("WriteFile(custom) error = %v", err)
	}
	if err := os.WriteFile(store.OperationLockPath(source), []byte("lock"), 0o600); err != nil {
		t.Fatalf("WriteFile(lock) error = %v", err)
	}

	plan, err := BuildPlan(PlanOptions{FromPath: source, ToPath: filepath.Join(t.TempDir(), "dest")})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if !plan.CanMigrate || plan.Summary.FileCount == 0 || plan.Summary.ByteCount == 0 {
		t.Fatalf("plan summary = %+v can=%v", plan.Summary, plan.CanMigrate)
	}
	for _, entry := range plan.Entries {
		if entry.RelativePath == ".loki-operation.lock" {
			t.Fatalf("operation lock included in plan: %+v", entry)
		}
	}
	if !planContains(plan, "profiles/work/core/files/settings.json") {
		t.Fatalf("custom file missing from plan entries: %+v", plan.Entries)
	}
}

func TestBuildPlanReportsDatalessFilesWithoutScanningForever(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	cloudFile := filepath.Join(source, "profiles", "work", "core", "files", "cloud.txt")
	if err := os.WriteFile(cloudFile, []byte("cloud"), 0o644); err != nil {
		t.Fatalf("WriteFile(cloud) error = %v", err)
	}

	plan, err := BuildPlan(PlanOptions{
		FromPath: source,
		ToPath:   filepath.Join(t.TempDir(), "dest"),
		Dataless: func(path string, info fs.FileInfo) bool {
			return path == cloudFile
		},
	})
	if err == nil || !strings.Contains(err.Error(), "cloud-only") {
		t.Fatalf("BuildPlan() error = %v, want cloud-only blocker", err)
	}
	if plan.Summary.DatalessCount != 1 {
		t.Fatalf("DatalessCount = %d, want 1", plan.Summary.DatalessCount)
	}
	if len(plan.DatalessEntries) != 1 || plan.DatalessEntries[0].RelativePath != "profiles/work/core/files/cloud.txt" {
		t.Fatalf("DatalessEntries = %+v", plan.DatalessEntries)
	}
}

func TestBuildPlanAllowsDatalessWhenHydrateRequested(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	cloudFile := filepath.Join(source, "profiles", "work", "core", "files", "cloud.txt")
	if err := os.WriteFile(cloudFile, []byte("cloud"), 0o644); err != nil {
		t.Fatalf("WriteFile(cloud) error = %v", err)
	}

	plan, err := BuildPlan(PlanOptions{
		FromPath:      source,
		ToPath:        filepath.Join(t.TempDir(), "dest"),
		AllowDataless: true,
		Dataless:      func(path string, info fs.FileInfo) bool { return path == cloudFile },
	})
	if err != nil {
		t.Fatalf("BuildPlan(AllowDataless) error = %v", err)
	}
	if !plan.CanMigrate || plan.Summary.DatalessCount != 1 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestBuildPlanUsesCloudPlaceholderDetector(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	cloudFile := filepath.Join(source, "profiles", "work", "core", "files", "cloud.txt")
	if err := os.WriteFile(cloudFile, []byte("cloud"), 0o644); err != nil {
		t.Fatalf("WriteFile(cloud) error = %v", err)
	}

	_, err := BuildPlan(PlanOptions{
		FromPath: source,
		ToPath:   filepath.Join(t.TempDir(), "dest"),
		Detector: fakeCloudDetector{path: cloudFile},
	})
	if err == nil || !strings.Contains(err.Error(), "cloud-only") {
		t.Fatalf("BuildPlan(detector) error = %v, want cloud-only blocker", err)
	}
}

type fakeCloudDetector struct{ path string }

func (d fakeCloudDetector) IsCloudOnly(path string, _ fs.FileInfo) bool {
	return path == d.path
}

func planContains(plan Plan, rel string) bool {
	for _, entry := range plan.Entries {
		if entry.RelativePath == rel {
			return true
		}
	}
	return false
}
