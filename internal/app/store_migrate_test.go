package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/db"
	"github.com/asudbring/loki-profile-manager/internal/store"
)

func TestStoreMigrateDryRunPlansWithoutCopyingOrSwitching(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	oldStore := filepath.Join(t.TempDir(), "old")
	if _, err := svc.EnsureStore(ctx, EnsureStoreRequest{StorePath: oldStore}); err != nil {
		t.Fatalf("EnsureStore() error = %v", err)
	}
	writeStoreMigrateTestFile(t, filepath.Join(oldStore, "profiles", "work", "core", "files", "settings.json"), "old")
	dest := filepath.Join(t.TempDir(), "new")

	result, err := svc.StoreMigrate(ctx, StoreMigrateRequest{ToPath: dest, DryRun: true, Provider: store.ProviderDropbox})
	if err != nil {
		t.Fatalf("StoreMigrate(--dry-run) error = %v", err)
	}
	if !result.DryRun || result.CopiedFiles != 0 || result.Switched || result.Plan.Summary.FileCount == 0 {
		t.Fatalf("dry-run result = %+v", result)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("dest exists after dry-run or stat err = %v", err)
	}
	persisted, _, err := db.GetKV(ctx, svc.database, kvStorePath)
	if err != nil {
		t.Fatalf("GetKV(store_path) error = %v", err)
	}
	if persisted != config.CleanForOS("darwin", oldStore) {
		t.Fatalf("persisted = %q, want old store %q", persisted, oldStore)
	}
}

func TestStoreMigrateDryRunIgnoresExistingOperationLock(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	oldStore := filepath.Join(t.TempDir(), "old")
	if _, err := svc.EnsureStore(ctx, EnsureStoreRequest{StorePath: oldStore}); err != nil {
		t.Fatalf("EnsureStore() error = %v", err)
	}
	lockPath := store.OperationLockPath(oldStore)
	if err := os.WriteFile(lockPath, []byte("existing lock"), 0o600); err != nil {
		t.Fatalf("WriteFile(lock) error = %v", err)
	}
	dest := filepath.Join(t.TempDir(), "new")
	result, err := svc.StoreMigrate(ctx, StoreMigrateRequest{ToPath: dest, DryRun: true})
	if err != nil {
		t.Fatalf("StoreMigrate(--dry-run with existing lock) error = %v", err)
	}
	if !result.DryRun || result.Plan.Summary.FileCount == 0 {
		t.Fatalf("dry-run result = %+v", result)
	}
	if got := string(mustReadAppTest(t, lockPath)); got != "existing lock" {
		t.Fatalf("dry-run changed lock file to %q", got)
	}
}

func TestStoreMigrateYesCopiesRebasesManagedTargetsAndPersistsNewStore(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	oldStore := filepath.Join(t.TempDir(), "old")
	if _, err := svc.EnsureStore(ctx, EnsureStoreRequest{StorePath: oldStore}); err != nil {
		t.Fatalf("EnsureStore() error = %v", err)
	}
	oldSource := filepath.Join(oldStore, "profiles", "work", "core", "files", "settings.json")
	writeStoreMigrateTestFile(t, oldSource, "old")
	target := filepath.Join(t.TempDir(), "settings.json")
	writeStoreMigrateTestFile(t, target, "old")
	targetHash, err := activation.HashPath(target)
	if err != nil {
		t.Fatalf("HashPath(target) error = %v", err)
	}
	if err := activation.PutManagedTarget(ctx, svc.database, activation.ManagedTarget{
		TargetPath:    target,
		SourcePath:    oldSource,
		Mode:          string(activation.OperationCopy),
		ContentHash:   targetHash,
		LayerKind:     "core",
		LayerName:     "work",
		LastAppliedAt: time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("PutManagedTarget() error = %v", err)
	}
	dest := filepath.Join(t.TempDir(), "new")

	result, err := svc.StoreMigrate(ctx, StoreMigrateRequest{ToPath: dest, Yes: true, Provider: store.ProviderOneDriveBusiness})
	if err != nil {
		t.Fatalf("StoreMigrate(--yes) error = %v", err)
	}
	if !result.Switched || result.CopiedFiles == 0 || result.RebasedManagedTargets != 1 {
		t.Fatalf("migrate result = %+v", result)
	}
	if got := string(mustReadAppTest(t, filepath.Join(dest, "profiles", "work", "core", "files", "settings.json"))); got != "old" {
		t.Fatalf("copied store file = %q", got)
	}
	persisted, _, err := db.GetKV(ctx, svc.database, kvStorePath)
	if err != nil {
		t.Fatalf("GetKV(store_path) error = %v", err)
	}
	if persisted != config.CleanForOS("darwin", dest) {
		t.Fatalf("persisted store = %q, want %q", persisted, dest)
	}
	record, found, err := activation.GetManagedTarget(ctx, svc.database, target)
	if err != nil || !found {
		t.Fatalf("GetManagedTarget() found=%v err=%v", found, err)
	}
	wantSource := filepath.Join(config.CleanForOS("darwin", dest), "profiles", "work", "core", "files", "settings.json")
	if record.SourcePath != wantSource {
		t.Fatalf("rebased source = %q, want %q", record.SourcePath, wantSource)
	}
}

func TestStoreMigrateCopyOnlyDoesNotSwitchOrRebase(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	oldStore := filepath.Join(t.TempDir(), "old")
	if _, err := svc.EnsureStore(ctx, EnsureStoreRequest{StorePath: oldStore}); err != nil {
		t.Fatalf("EnsureStore() error = %v", err)
	}
	dest := filepath.Join(t.TempDir(), "new")
	result, err := svc.StoreMigrate(ctx, StoreMigrateRequest{ToPath: dest, Yes: true, CopyOnly: true})
	if err != nil {
		t.Fatalf("StoreMigrate(copy-only) error = %v", err)
	}
	if result.Switched || result.RebasedManagedTargets != 0 {
		t.Fatalf("copy-only result = %+v", result)
	}
	persisted, _, err := db.GetKV(ctx, svc.database, kvStorePath)
	if err != nil {
		t.Fatalf("GetKV(store_path) error = %v", err)
	}
	if persisted != config.CleanForOS("darwin", oldStore) {
		t.Fatalf("persisted = %q, want old %q", persisted, oldStore)
	}
}

func writeStoreMigrateTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
