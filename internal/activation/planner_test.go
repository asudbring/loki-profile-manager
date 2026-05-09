package activation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/store"
)

func TestBuildPlanMergesDuplicateStructuredTargets(t *testing.T) {
	storeRoot := activationTestStore(t)
	home := t.TempDir()
	writeFile(t, filepath.Join(storeRoot, "profiles", "common", "files", "settings.json"), `{"editor":{"fontSize":12},"list":[1]}`)
	writeFile(t, filepath.Join(storeRoot, "profiles", "work", "core", "files", "settings.json"), `{"editor":{"lineNumbers":"on"},"list":[2]}`)
	writeFile(t, filepath.Join(storeRoot, "profiles", "common", "manifest.yaml"), `version: 1
name: common
files:
  - id: common-settings
    source: files/settings.json
    target: "~/settings.json"
    mode: merge
    format: json
skills: []
ignore: []
merge_rules: {}
targets: {}
`)
	writeFile(t, filepath.Join(storeRoot, "profiles", "work", "core", "manifest.yaml"), `version: 1
name: work-core
files:
  - id: work-settings
    source: files/settings.json
    target: "~/settings.json"
    mode: merge
    format: json
skills: []
ignore: []
merge_rules: {}
targets: {}
`)
	plan, err := BuildPlan(context.Background(), PlanRequest{StorePath: storeRoot, Profile: "work", Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if len(plan.Operations) != 1 {
		t.Fatalf("operations = %+v", plan.Operations)
	}
	op := plan.Operations[0]
	if op.Type != OperationMerge || len(op.Sources) != 2 || op.ExpectedHash == "" {
		t.Fatalf("merge op = %+v", op)
	}
	if op.TargetPath != filepath.ToSlash(filepath.Join(home, "settings.json")) {
		t.Fatalf("TargetPath = %q", op.TargetPath)
	}
	if _, err := json.Marshal(plan); err != nil {
		t.Fatalf("plan JSON marshal error = %v", err)
	}
}

func TestBuildPlanRejectsDuplicateUnmergeableTargets(t *testing.T) {
	storeRoot := activationTestStore(t)
	home := t.TempDir()
	writeFile(t, filepath.Join(storeRoot, "profiles", "common", "files", "a.txt"), "a")
	writeFile(t, filepath.Join(storeRoot, "profiles", "work", "core", "files", "b.txt"), "b")
	writeFile(t, filepath.Join(storeRoot, "profiles", "common", "manifest.yaml"), `version: 1
name: common
files:
  - id: a
    source: files/a.txt
    target: "~/same.txt"
    mode: copy
skills: []
ignore: []
merge_rules: {}
targets: {}
`)
	writeFile(t, filepath.Join(storeRoot, "profiles", "work", "core", "manifest.yaml"), `version: 1
name: work-core
files:
  - id: b
    source: files/b.txt
    target: "~/same.txt"
    mode: copy
skills: []
ignore: []
merge_rules: {}
targets: {}
`)
	if _, err := BuildPlan(context.Background(), PlanRequest{StorePath: storeRoot, Profile: "work", Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}}); err == nil {
		t.Fatal("BuildPlan() error = nil, want duplicate target error")
	}
}

func activationTestStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	return root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
