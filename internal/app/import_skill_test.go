package app

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/manifest"
	"github.com/asudbring/loki-profile-manager/internal/store"
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

func TestImportSkillZipDryRunDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	archivePath := writeSkillZip(t, filepath.Join(t.TempDir(), "sample-skill.zip"), map[string]string{
		"sample-skill/SKILL.md": "---\nname: sample-skill\ndescription: Test skill\n---\n# sample-skill\n",
		"sample-skill/notes.md": "v1",
	})

	result, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: archivePath, Common: true, DryRun: true})
	if err != nil {
		t.Fatalf("ImportSkill(zip dry-run) error = %v", err)
	}
	if result.SourceKind != "zip" || result.Name != "sample-skill" || !result.DryRun || !result.WouldCopy || !result.ManifestChanged || result.Changed != 0 {
		t.Fatalf("zip dry-run result = %+v", result)
	}
	if _, err := os.Stat(result.DestinationPath); !os.IsNotExist(err) {
		t.Fatalf("zip dry-run destination exists or stat err = %v", err)
	}
}

func TestImportSkillZipArchiveRootCopiesWithZipBaseName(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	archivePath := writeSkillZip(t, filepath.Join(t.TempDir(), "root-skill.zip"), map[string]string{
		"SKILL.md": "---\nname: root-skill\ndescription: Test skill\n---\n# root-skill\n",
		"notes.md": "v1",
	})

	result, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: archivePath, Common: true, Yes: true})
	if err != nil {
		t.Fatalf("ImportSkill(zip) error = %v", err)
	}
	if result.SourceKind != "zip" || result.Name != "root-skill" || result.ManifestSource != "skills/root-skill" || result.Changed != 1 {
		t.Fatalf("zip result = %+v", result)
	}
	if got := readAppFile(t, filepath.Join(result.DestinationPath, "notes.md")); got != "v1" {
		t.Fatalf("zip imported notes = %q", got)
	}
}

func TestImportSkillZipAcceptsWindowsSeparators(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	archivePath := writeSkillZip(t, filepath.Join(t.TempDir(), "windows-skill.zip"), map[string]string{
		`windows-skill\\SKILL.md`: "---\nname: windows-skill\ndescription: Test skill\n---\n# windows-skill\n",
		`windows-skill\\notes.md`: "v1",
	})

	result, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: archivePath, Common: true, Yes: true})
	if err != nil {
		t.Fatalf("ImportSkill(zip with windows separators) error = %v", err)
	}
	if result.SourceKind != "zip" || result.Name != "windows-skill" || result.ManifestSource != "skills/windows-skill" {
		t.Fatalf("windows separator zip result = %+v", result)
	}
	if got := readAppFile(t, filepath.Join(result.DestinationPath, "notes.md")); got != "v1" {
		t.Fatalf("windows separator imported notes = %q", got)
	}
}

func TestImportSkillZipNameOverride(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	archivePath := writeSkillZip(t, filepath.Join(t.TempDir(), "sample-skill.zip"), map[string]string{
		"sample-skill/SKILL.md": "---\nname: sample-skill\ndescription: Test skill\n---\n# sample-skill\n",
		"sample-skill/notes.md": "v1",
	})

	result, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: archivePath, Common: true, Name: "renamed-skill", Yes: true})
	if err != nil {
		t.Fatalf("ImportSkill(zip renamed) error = %v", err)
	}
	if result.Name != "renamed-skill" || result.ManifestSource != "skills/renamed-skill" {
		t.Fatalf("zip rename result = %+v", result)
	}
	if got := readAppFile(t, filepath.Join(storePath, "profiles", "common", "skills", "renamed-skill", "notes.md")); got != "v1" {
		t.Fatalf("zip renamed notes = %q", got)
	}
}

func TestImportSkillZipRejectsUnsafeEntries(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	root := t.TempDir()
	archivePath := writeSkillZip(t, filepath.Join(root, "bad.zip"), map[string]string{
		"../evil.txt": "evil",
	})

	_, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: archivePath, Common: true, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "unsafe zip entry") {
		t.Fatalf("ImportSkill(zip traversal) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "evil.txt")); !os.IsNotExist(err) {
		t.Fatalf("unsafe zip wrote outside staging or stat err = %v", err)
	}
}

func TestImportSkillZipRejectsMultipleSkillRoots(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	archivePath := writeSkillZip(t, filepath.Join(t.TempDir(), "multi.zip"), map[string]string{
		"one/SKILL.md": "---\nname: one\ndescription: One\n---\n# one\n",
		"two/SKILL.md": "---\nname: two\ndescription: Two\n---\n# two\n",
	})

	_, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: archivePath, Common: true, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "exactly one top-level skill folder") {
		t.Fatalf("ImportSkill(zip multiple roots) error = %v", err)
	}
}

func TestImportSkillRejectsInvalidZip(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	archivePath := filepath.Join(t.TempDir(), "bad.zip")
	writeAppFile(t, archivePath, "not a zip")

	_, err := svc.ImportSkill(ctx, ImportSkillRequest{StorePath: storePath, SourceFolder: archivePath, Common: true, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "open zip") {
		t.Fatalf("ImportSkill(invalid zip) error = %v", err)
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

func writeSkillZip(t *testing.T, zipPath string, entries map[string]string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(zip parent) error = %v", err)
	}
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Create(zip) error = %v", err)
	}
	writer := zip.NewWriter(file)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("Create(%s) error = %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%s) error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip Close() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file Close() error = %v", err)
	}
	return zipPath
}
