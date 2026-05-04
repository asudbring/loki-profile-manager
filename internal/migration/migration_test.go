package migration

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/activation"
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

func TestBuildAdoptPlanAllowsSymlinkTarget(t *testing.T) {
	home := t.TempDir()
	storePath := migrationStore(t)
	realTarget := filepath.Join(home, "real.gitconfig")
	linkTarget := filepath.Join(home, ".gitconfig")
	writeMigrationFile(t, realTarget, "[user]\n")
	if err := os.Symlink(realTarget, linkTarget); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	plan, err := BuildAdoptPlan(AdoptRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: migrationTestResolver(home), Target: linkTarget})
	if err != nil {
		t.Fatalf("BuildAdoptPlan() error = %v", err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	item := plan.Items[0]
	if !sameMigrationFile(t, item.SourcePath, realTarget) || !sameMigrationFile(t, item.TargetPath, linkTarget) || item.AdoptedTargetHash == "" || !item.WillAdoptRecord {
		t.Fatalf("item = %+v", item)
	}
}

func TestBuildAdoptPlanRejectsBrokenSymlinkTarget(t *testing.T) {
	home := t.TempDir()
	storePath := migrationStore(t)
	linkTarget := filepath.Join(home, ".gitconfig")
	if err := os.Symlink(filepath.Join(home, "missing.gitconfig"), linkTarget); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := BuildAdoptPlan(AdoptRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: migrationTestResolver(home), Target: linkTarget})
	if err == nil || !strings.Contains(err.Error(), "broken symlink") {
		t.Fatalf("BuildAdoptPlan() error = %v", err)
	}
}

func TestBuildAdoptPlanRejectsSymlinkRenderMode(t *testing.T) {
	home := t.TempDir()
	storePath := migrationStore(t)
	realTarget := filepath.Join(home, "real.gitconfig")
	linkTarget := filepath.Join(home, ".gitconfig")
	writeMigrationFile(t, realTarget, "[user]\n")
	if err := os.Symlink(realTarget, linkTarget); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := BuildAdoptPlan(AdoptRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: migrationTestResolver(home), Target: linkTarget, Mode: manifest.ModeRender})
	if err == nil || !strings.Contains(err.Error(), "requires a regular file") {
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

func TestBuildLocalPlanAllowsKnownSymlinkDotfile(t *testing.T) {
	home := t.TempDir()
	storePath := migrationStore(t)
	realTarget := filepath.Join(home, "real.gitconfig")
	linkTarget := filepath.Join(home, ".gitconfig")
	writeMigrationFile(t, realTarget, "[user]\n")
	if err := os.Symlink(realTarget, linkTarget); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	plan, err := BuildLocalPlan(LocalRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: migrationTestResolver(home)})
	if err != nil {
		t.Fatalf("BuildLocalPlan() error = %v", err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("items = %+v", plan.Items)
	}
	item := plan.Items[0]
	if item.Target != "~/.gitconfig" || !sameMigrationFile(t, item.SourcePath, realTarget) || item.AdoptedTargetHash == "" {
		t.Fatalf("item = %+v", item)
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

func TestExecuteAdoptSymlinkCopiesResolvedSourceAndRecordsSymlinkHash(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	storePath := migrationStore(t)
	realTarget := filepath.Join(home, "real.gitconfig")
	linkTarget := filepath.Join(home, ".gitconfig")
	writeMigrationFile(t, realTarget, "[user]\n")
	if err := os.Symlink(realTarget, linkTarget); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	resolver := migrationTestResolver(home)
	plan, err := BuildAdoptPlan(AdoptRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: resolver, Target: linkTarget})
	if err != nil {
		t.Fatalf("BuildAdoptPlan() error = %v", err)
	}
	database, err := db.Bootstrap(ctx, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	defer database.Close()
	wantHash, err := activation.HashPath(linkTarget)
	if err != nil {
		t.Fatalf("HashPath(link) error = %v", err)
	}
	if _, err := Execute(ctx, ExecuteRequest{Database: database, Resolver: resolver, Plan: plan, Yes: true}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	storeCopy := plan.Items[0].StorePath
	if info, err := os.Lstat(storeCopy); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("store copy lstat mode=%v err=%v, want regular file", info.Mode(), err)
	}
	if got := string(mustReadMigrationFile(t, storeCopy)); got != "[user]\n" {
		t.Fatalf("store copy = %q", got)
	}
	record, found, err := activation.GetManagedTarget(ctx, database, linkTarget)
	if err != nil || !found {
		t.Fatalf("GetManagedTarget() found=%v err=%v", found, err)
	}
	if record.ContentHash != wantHash {
		t.Fatalf("ContentHash = %q, want symlink hash %q", record.ContentHash, wantHash)
	}
}

func TestExecuteRejectsSymlinkRetargetBeforeManagedStateWrite(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	storePath := migrationStore(t)
	first := filepath.Join(home, "first.gitconfig")
	second := filepath.Join(home, "second.gitconfig")
	linkTarget := filepath.Join(home, ".gitconfig")
	writeMigrationFile(t, first, "first")
	writeMigrationFile(t, second, "second")
	if err := os.Symlink(first, linkTarget); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	resolver := migrationTestResolver(home)
	plan, err := BuildAdoptPlan(AdoptRequest{BuildRequest: BuildRequest{StorePath: storePath, Profile: "work"}, Resolver: resolver, Target: linkTarget})
	if err != nil {
		t.Fatalf("BuildAdoptPlan() error = %v", err)
	}
	if err := os.Remove(linkTarget); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if err := os.Symlink(second, linkTarget); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	database, err := db.Bootstrap(ctx, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	defer database.Close()
	_, err = Execute(ctx, ExecuteRequest{Database: database, Resolver: resolver, Plan: plan, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "changed before managed-state write") {
		t.Fatalf("Execute() error = %v", err)
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

func migrationTestResolver(home string) config.PathResolver {
	return config.PathResolver{GOOS: runtime.GOOS, HomeDir: home}
}

func migrationStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	return root
}

func sameMigrationFile(t *testing.T, left, right string) bool {
	t.Helper()
	leftInfo, err := os.Stat(left)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", left, err)
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", right, err)
	}
	return os.SameFile(leftInfo, rightInfo)
}

func mustReadMigrationFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return content
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
