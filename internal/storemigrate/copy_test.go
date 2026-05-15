package storemigrate

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

func TestCopyPlanWithOptionsReportsProgress(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	contentPath := filepath.Join(source, "profiles", "work", "core", "files", "settings.json")
	if err := os.WriteFile(contentPath, []byte(`{"copied":true}`), 0o644); err != nil {
		t.Fatalf("WriteFile(content) error = %v", err)
	}
	dest := filepath.Join(t.TempDir(), "dest")
	plan, err := BuildPlan(PlanOptions{FromPath: source, ToPath: dest})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	reporter := &MemoryReporter{}
	result, err := CopyPlanWithOptions(context.Background(), CopyOptions{Plan: plan, Reporter: reporter, FileTimeout: time.Second})
	if err != nil {
		t.Fatalf("CopyPlanWithOptions() error = %v", err)
	}
	if !result.Valid || result.CopiedFiles == 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(reporter.Events()) == 0 {
		t.Fatalf("no progress events recorded")
	}
}

func TestCopyPlanWithOptionsReturnsOnFileTimeout(t *testing.T) {
	plan := Plan{
		FromPath:        filepath.Join(t.TempDir(), "source"),
		ToPath:          filepath.Join(t.TempDir(), "dest"),
		CanMigrate:      true,
		Summary:         Summary{FileCount: 1},
		Entries:         []Entry{{RelativePath: "cloud.txt", SourcePath: filepath.Join(t.TempDir(), "cloud.txt"), DestPath: filepath.Join(t.TempDir(), "dest", "cloud.txt"), Kind: "file"}},
		Warnings:        []string{},
		Blockers:        []string{},
		Provider:        "manual",
		DatalessEntries: []Entry{},
	}
	if err := os.MkdirAll(plan.FromPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(source) error = %v", err)
	}
	start := time.Now()
	_, err := CopyPlanWithOptions(context.Background(), CopyOptions{
		Plan:        plan,
		FileTimeout: 10 * time.Millisecond,
		CopyFile: func(ctx context.Context, entry Entry) (int64, error) {
			<-ctx.Done()
			return 0, ctx.Err()
		},
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("CopyPlanWithOptions(timeout) error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout returned too slowly: %s", elapsed)
	}
}

func TestPlanWithDestinationRewritesEntryDestinations(t *testing.T) {
	plan := Plan{ToPath: "/final", Entries: []Entry{{RelativePath: "profiles/work/core/manifest.yaml", DestPath: "/final/profiles/work/core/manifest.yaml"}}}
	staging := PlanWithDestination(plan, "/staging")
	if staging.ToPath != "/staging" || staging.Entries[0].DestPath != filepath.Join("/staging", "profiles", "work", "core", "manifest.yaml") {
		t.Fatalf("staging plan = %+v", staging)
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
