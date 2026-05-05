package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allensu/loki-profile-manager/internal/activation"
	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/manifest"
	"github.com/allensu/loki-profile-manager/internal/store"
)

func TestImportSkillDryRunDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	source := writeSkillFolder(t, filepath.Join(t.TempDir(), "sample-skill"), "sample-skill", "v1")

	result, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: source, Common: true, DryRun: true})
	if err != nil {
		t.Fatalf("ImportSkill(dry-run) error = %v", err)
	}
	if !result.DryRun || !result.WouldCopy || !result.ManifestChanged || result.Changed != 0 {
		t.Fatalf("dry-run result = %+v", result)
	}
	if _, err := os.Stat(result.DestinationPath); !os.IsNotExist(err) {
		t.Fatalf("dry-run destination exists or stat err = %v", err)
	}
	parsed, err := manifest.ParseFile(filepath.Join(storePath, "profiles", "common", "manifest.yaml"))
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if len(parsed.Skills) != 0 {
		t.Fatalf("dry-run changed manifest: %+v", parsed.Skills)
	}
}

func TestImportSkillYesCopiesAndUpdatesManifest(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	source := writeSkillFolder(t, filepath.Join(t.TempDir(), "sample-skill"), "sample-skill", "v1")

	result, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: source, Common: true, Yes: true})
	if err != nil {
		t.Fatalf("ImportSkill() error = %v", err)
	}
	if result.Changed != 1 || result.ManifestSource != "skills/sample-skill" || result.Layer.Kind != "common" {
		t.Fatalf("result = %+v", result)
	}
	if got := readAppFile(t, filepath.Join(result.DestinationPath, "notes.md")); got != "v1" {
		t.Fatalf("imported notes = %q", got)
	}
	parsed, err := manifest.ParseFile(filepath.Join(storePath, "profiles", "common", "manifest.yaml"))
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if len(parsed.Skills) != 1 || parsed.Skills[0].Source != "skills/sample-skill" {
		t.Fatalf("manifest skills = %+v", parsed.Skills)
	}
	managedTargets, err := activation.ListManagedTargets(ctx, svc.database)
	if err != nil {
		t.Fatalf("ListManagedTargets() error = %v", err)
	}
	if len(managedTargets) != 0 {
		t.Fatalf("import-skill wrote managed targets: %+v", managedTargets)
	}
}

func TestImportSkillProfileBucketCreatesLayer(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	source := writeSkillFolder(t, filepath.Join(t.TempDir(), "cloud-skill"), "cloud-skill", "v1")

	result, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: source, Profile: "work", Bucket: "azure", Name: "cloud", Yes: true})
	if err != nil {
		t.Fatalf("ImportSkill(bucket) error = %v", err)
	}
	wantDest := filepath.Join(storePath, "profiles", "work", "buckets", "azure", "skills", "cloud")
	if result.DestinationPath != wantDest || result.ManifestSource != "skills/cloud" || result.Layer.Kind != "bucket" {
		t.Fatalf("result = %+v, want dest %s", result, wantDest)
	}
	if got := readAppFile(t, filepath.Join(wantDest, "SKILL.md")); !strings.Contains(got, "cloud-skill") {
		t.Fatalf("imported skill = %q", got)
	}
	parsed, err := manifest.ParseFile(filepath.Join(storePath, "profiles", "work", "buckets", "azure", "manifest.yaml"))
	if err != nil {
		t.Fatalf("ParseFile(bucket) error = %v", err)
	}
	if parsed.Name != "azure" || len(parsed.Skills) != 1 || parsed.Skills[0].Source != "skills/cloud" {
		t.Fatalf("bucket manifest = %+v", parsed)
	}
}

func TestImportSkillRejectsSymlinkInSource(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	source := writeSkillFolder(t, filepath.Join(t.TempDir(), "link-skill"), "link-skill", "v1")
	writeAppFile(t, filepath.Join(source, "target.txt"), "target")
	if err := os.Symlink("target.txt", filepath.Join(source, "link.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: source, Common: true, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("ImportSkill(symlink) error = %v", err)
	}
}

func TestImportSkillRequiresOverwriteForExistingDestination(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	source := writeSkillFolder(t, filepath.Join(t.TempDir(), "sample-skill"), "sample-skill", "v1")
	if _, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: source, Common: true, Yes: true}); err != nil {
		t.Fatalf("initial ImportSkill() error = %v", err)
	}
	writeAppFile(t, filepath.Join(source, "notes.md"), "v2")

	_, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: source, Common: true, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "--overwrite") {
		t.Fatalf("ImportSkill(existing) error = %v", err)
	}
	if got := readAppFile(t, filepath.Join(storePath, "profiles", "common", "skills", "sample-skill", "notes.md")); got != "v1" {
		t.Fatalf("destination changed without overwrite: %q", got)
	}
	result, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: source, Common: true, Yes: true, Overwrite: true})
	if err != nil {
		t.Fatalf("ImportSkill(overwrite) error = %v", err)
	}
	if !result.DestinationExists || !result.WouldOverwrite || result.Changed != 1 {
		t.Fatalf("overwrite result = %+v", result)
	}
	if got := readAppFile(t, filepath.Join(storePath, "profiles", "common", "skills", "sample-skill", "notes.md")); got != "v2" {
		t.Fatalf("destination not overwritten: %q", got)
	}
}

func TestImportSkillIdenticalDestinationIsIdempotentWithoutOverwrite(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	source := writeSkillFolder(t, filepath.Join(t.TempDir(), "sample-skill"), "sample-skill", "v1")
	if _, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: source, Common: true, Yes: true}); err != nil {
		t.Fatalf("initial ImportSkill() error = %v", err)
	}

	result, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: source, Common: true, Yes: true})
	if err != nil {
		t.Fatalf("ImportSkill(idempotent) error = %v", err)
	}
	if !result.DestinationExists || result.WouldCopy || result.WouldOverwrite || result.ManifestChanged || result.Changed != 0 {
		t.Fatalf("idempotent result = %+v", result)
	}
}

func TestImportSkillFailsWhenStoreOperationLockHeld(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	source := writeSkillFolder(t, filepath.Join(t.TempDir(), "locked-skill"), "locked-skill", "v1")
	unlock, err := store.AcquireOperationLock(ctx, storePath, store.OperationLockOptions{Operation: "test-holder"})
	if err != nil {
		t.Fatalf("AcquireOperationLock() error = %v", err)
	}
	defer unlock()

	lockCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	_, err = svc.ImportSkill(lockCtx, ImportSkillRequest{StorePath: storePath, SourceFolder: source, Common: true, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "operation lock") {
		t.Fatalf("ImportSkill() error = %v, want lock error", err)
	}
}

func newImportSkillTestService(t *testing.T, ctx context.Context, home string) *Service {
	t.Helper()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

func importSkillStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	return root
}

func writeSkillFolder(t *testing.T, dir, name, notes string) string {
	t.Helper()
	writeAppFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: "+name+"\ndescription: Test skill\n---\n# "+name+"\n")
	writeAppFile(t, filepath.Join(dir, "notes.md"), notes)
	return dir
}
