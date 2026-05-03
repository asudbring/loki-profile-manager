package migration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/db"
	"github.com/allensu/loki-profile-manager/internal/manifest"
	"github.com/allensu/loki-profile-manager/internal/store"
)

func TestBuildAdoptPlanRequiresExistingHomeTarget(t *testing.T) {
	home := t.TempDir()
	storePath := migrationStore(t)
	if _, err := BuildAdoptPlan(AdoptRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}, Target: filepath.Join(home, "missing.txt")}); err == nil {
		t.Fatal("BuildAdoptPlan() error = nil, want missing target error")
	}
	writeMigrationFile(t, filepath.Join(home, ".gitconfig"), "[user]\n")
	plan, err := BuildAdoptPlan(AdoptRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}, Target: filepath.Join(home, ".gitconfig"), Mode: manifest.ModeSymlink})
	if err != nil {
		t.Fatalf("BuildAdoptPlan() error = %v", err)
	}
	if len(plan.Items) != 1 || plan.Items[0].Mode != manifest.ModeSymlink || plan.Items[0].Target != "~/.gitconfig" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestBuildAdoptPlanRejectsInvalidMergeMode(t *testing.T) {
	home := t.TempDir()
	storePath := migrationStore(t)
	target := filepath.Join(home, ".gitconfig")
	writeMigrationFile(t, target, "[user]\n")
	_, err := BuildAdoptPlan(AdoptRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}, Target: target, Mode: manifest.ModeMerge})
	if err == nil || !strings.Contains(err.Error(), "merge mode requires") {
		t.Fatalf("BuildAdoptPlan() error = %v", err)
	}
}

func TestBuildAdoptPlanRejectsSymlinkSource(t *testing.T) {
	home := t.TempDir()
	storePath := migrationStore(t)
	realTarget := filepath.Join(home, "real.gitconfig")
	linkTarget := filepath.Join(home, ".gitconfig")
	writeMigrationFile(t, realTarget, "[user]\n")
	if err := os.Symlink(realTarget, linkTarget); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := BuildAdoptPlan(AdoptRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}, Target: linkTarget})
	if err == nil || !strings.Contains(err.Error(), "is a symlink") {
		t.Fatalf("BuildAdoptPlan() error = %v", err)
	}
}

func TestBuildLocalPlanKnownPathsAndSkills(t *testing.T) {
	home := t.TempDir()
	storePath := migrationStore(t)
	writeMigrationFile(t, filepath.Join(home, ".gitconfig"), "[user]\n")
	writeMigrationFile(t, filepath.Join(home, "Library", "Application Support", "Code", "User", "settings.json"), `{"editor.fontSize": 12}`)
	writeMigrationFile(t, filepath.Join(home, ".pi", "agent", "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: demo skill\n---\nBody\n")
	plan, err := BuildLocalPlan(LocalRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("BuildLocalPlan() error = %v", err)
	}
	if len(plan.Items) != 3 {
		t.Fatalf("items = %+v", plan.Items)
	}
	seenSkill := false
	for _, item := range plan.Items {
		if item.IsSkill && item.SkillSource == "skills/pi/demo" {
			seenSkill = true
		}
	}
	if !seenSkill {
		t.Fatalf("skill item missing: %+v", plan.Items)
	}
}

func TestBuildLocalPlanLinuxVSCodePath(t *testing.T) {
	home := t.TempDir()
	storePath := migrationStore(t)
	writeMigrationFile(t, filepath.Join(home, ".config", "Code", "User", "settings.json"), `{"linux": true}`)
	plan, err := BuildLocalPlan(LocalRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: config.PathResolver{GOOS: "linux", HomeDir: home}})
	if err != nil {
		t.Fatalf("BuildLocalPlan() error = %v", err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("items = %+v", plan.Items)
	}
	if plan.Items[0].Target != "~/.config/Code/User/settings.json" {
		t.Fatalf("target = %q", plan.Items[0].Target)
	}
}

func TestBuildRepoPlanPreservesRelativeStructure(t *testing.T) {
	home := t.TempDir()
	storePath := migrationStore(t)
	repo := t.TempDir()
	writeMigrationFile(t, filepath.Join(repo, ".gitconfig"), "[user]\n")
	writeMigrationFile(t, filepath.Join(repo, ".config", "app", "settings.json"), `{"ok": true}`)
	writeMigrationFile(t, filepath.Join(repo, ".ssh", "id_ed25519"), "private-key")
	plan, err := BuildRepoPlan(RepoRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}, RepoPath: repo})
	if err != nil {
		t.Fatalf("BuildRepoPlan() error = %v", err)
	}
	if len(plan.Items) != 2 {
		t.Fatalf("items = %+v warnings=%v", plan.Items, plan.Warnings)
	}
	if len(plan.Warnings) == 0 {
		t.Fatalf("sensitive path warning missing")
	}
	for _, item := range plan.Items {
		if item.Target == "~/.ssh/id_ed25519" {
			t.Fatalf("private key imported: %+v", item)
		}
		if item.Target == "~/.config/app/settings.json" && item.ManifestSource != "files/.config/app/settings.json" {
			t.Fatalf("settings item = %+v", item)
		}
	}
}

func TestRepoPlanKeepsDistinctStoreSourcesForSameTarget(t *testing.T) {
	home := t.TempDir()
	storePath := migrationStore(t)
	repo := t.TempDir()
	writeMigrationFile(t, filepath.Join(repo, ".config", "app", "settings.json"), `{"top": true}`)
	writeMigrationFile(t, filepath.Join(repo, "home", ".config", "app", "settings.json"), `{"home": true}`)
	plan, err := BuildRepoPlan(RepoRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}, RepoPath: repo})
	if err != nil {
		t.Fatalf("BuildRepoPlan() error = %v", err)
	}
	if len(plan.Items) != 2 {
		t.Fatalf("items = %+v", plan.Items)
	}
	storePaths := map[string]bool{}
	for _, item := range plan.Items {
		if item.Target != "~/.config/app/settings.json" || item.Mode != manifest.ModeMerge {
			t.Fatalf("item = %+v", item)
		}
		if storePaths[item.StorePath] {
			t.Fatalf("duplicate store path in plan: %+v", plan.Items)
		}
		storePaths[item.StorePath] = true
	}
}

func TestUniquifyIDsAvoidsGeneratedCollisions(t *testing.T) {
	items := uniquifyIDs([]Item{{ID: "a"}, {ID: "a"}, {ID: "a-2"}})
	got := []string{items[0].ID, items[1].ID, items[2].ID}
	want := []string{"a", "a-2", "a-2-2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}
}

func TestManifestWriterPreservesExistingFieldsOnUpsert(t *testing.T) {
	storePath := migrationStore(t)
	layer, err := resolveLayer(storePath, "work", "")
	if err != nil {
		t.Fatalf("resolveLayer() error = %v", err)
	}
	writeMigrationFile(t, layer.ManifestPath, `version: 1
name: work-core
files:
  - id: gitconfig
    source: files/.gitconfig
    target: "~/.gitconfig"
    mode: copy
    capture: true
skills:
  - source: skills/pi/demo
    targets:
      - "~/.pi/agent/skills/demo"
ignore: []
merge_rules: {}
targets: {}
`)
	items := []Item{
		{ID: "gitconfig", ManifestPath: layer.ManifestPath, ManifestSource: "files/.gitconfig", Target: "~/.gitconfig", Mode: manifest.ModeCopy},
		{ID: "demo", ManifestPath: layer.ManifestPath, ManifestSource: "skills/pi/demo", Target: "~/.pi/agent/skills/demo", Mode: manifest.ModeCopy, IsSkill: true, SkillSource: "skills/pi/demo"},
	}
	if err := writeManifestItems(layer, items); err != nil {
		t.Fatalf("writeManifestItems() error = %v", err)
	}
	parsed, err := manifest.ParseFile(layer.ManifestPath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if !parsed.Files[0].Capture {
		t.Fatalf("capture not preserved: %+v", parsed.Files[0])
	}
	if len(parsed.Skills) != 1 || len(parsed.Skills[0].Targets) != 1 {
		t.Fatalf("skill targets not preserved: %+v", parsed.Skills)
	}
}

func TestManifestWriterRejectsSameIDDifferentEntry(t *testing.T) {
	storePath := migrationStore(t)
	layer, err := resolveLayer(storePath, "work", "")
	if err != nil {
		t.Fatalf("resolveLayer() error = %v", err)
	}
	writeMigrationFile(t, layer.ManifestPath, `version: 1
name: work-core
files:
  - id: gitconfig
    source: files/other
    target: "~/.other"
    mode: copy
skills: []
ignore: []
merge_rules: {}
targets: {}
`)
	item := Item{ID: "gitconfig", ManifestPath: layer.ManifestPath, ManifestSource: "files/.gitconfig", Target: "~/.gitconfig", Mode: manifest.ModeCopy}
	if err := writeManifestItems(layer, []Item{item}); err == nil || !strings.Contains(err.Error(), "file id gitconfig already exists") {
		t.Fatalf("writeManifestItems() error = %v", err)
	}
}

func TestExecutePreflightsManifestBeforeReplacingStoreFile(t *testing.T) {
	ctx := context.Background()
	storePath := migrationStore(t)
	layer, err := resolveLayer(storePath, "work", "")
	if err != nil {
		t.Fatalf("resolveLayer() error = %v", err)
	}
	writeMigrationFile(t, layer.ManifestPath, `version: 1
name: work-core
files:
  - id: gitconfig
    source: files/other
    target: "~/.other"
    mode: copy
skills: []
ignore: []
merge_rules: {}
targets: {}
`)
	storeFile := filepath.Join(layer.Root, "files", ".gitconfig")
	writeMigrationFile(t, storeFile, "original")
	sourceFile := filepath.Join(t.TempDir(), ".gitconfig")
	writeMigrationFile(t, sourceFile, "new")
	database, err := db.Bootstrap(ctx, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	defer database.Close()
	plan := Plan{StorePath: storePath, Profile: "work", LayerRoot: layer.Root, LayerKind: layer.Kind, LayerName: layer.Name, Items: []Item{{ID: "gitconfig", SourcePath: sourceFile, StorePath: storeFile, ManifestPath: layer.ManifestPath, ManifestSource: "files/.gitconfig", Target: "~/.gitconfig", Mode: manifest.ModeCopy}}}
	_, err = Execute(ctx, ExecuteRequest{Database: database, Plan: plan, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "file id gitconfig already exists") {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, err := os.ReadFile(storeFile); err != nil || string(got) != "original" {
		t.Fatalf("store file changed to %q err=%v", got, err)
	}
}

func TestExecuteRejectsRepoAdoptionRecordWhenTargetChanged(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	storePath := migrationStore(t)
	repo := t.TempDir()
	writeMigrationFile(t, filepath.Join(repo, ".gitconfig"), "old")
	writeMigrationFile(t, filepath.Join(home, ".gitconfig"), "old")
	plan, err := BuildRepoPlan(RepoRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}, RepoPath: repo})
	if err != nil {
		t.Fatalf("BuildRepoPlan() error = %v", err)
	}
	if len(plan.Items) != 1 || !plan.Items[0].WillAdoptRecord {
		t.Fatalf("plan = %+v", plan)
	}
	writeMigrationFile(t, filepath.Join(home, ".gitconfig"), "changed")
	database, err := db.Bootstrap(ctx, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	defer database.Close()
	_, err = Execute(ctx, ExecuteRequest{Database: database, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}, Plan: plan, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "changed before managed-state write") {
		t.Fatalf("Execute() error = %v", err)
	}
}

func migrationStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	return root
}

func writeMigrationFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
