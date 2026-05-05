package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/machine"
	"github.com/allensu/loki-profile-manager/internal/store"
)

func TestSyncDryRunDetectsConflictsWithoutWriting(t *testing.T) {
	ctx := context.Background()
	svc := newSyncTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := syncTestStore(t)
	conflict := filepath.Join(storePath, "profiles", "work", "core", "files", "settings conflicted copy.json")
	writeAppFile(t, conflict, "losing")
	writeAppFile(t, filepath.Join(storePath, "profiles", "work", "core", "files", "normal.txt"), "keep")

	result, err := svc.Sync(ctx, SyncRequest{StorePath: storePath, DryRun: true})
	if err != nil {
		t.Fatalf("Sync(dry-run) error = %v", err)
	}
	if !result.DryRun || result.WouldDeleteCount != 1 || result.DeletedCount != 0 || len(result.Conflicts) != 1 || result.ConflictFingerprint == "" || result.HeartbeatUpdated || result.MachineID != "" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(conflict); err != nil {
		t.Fatalf("dry-run deleted conflict or stat failed: %v", err)
	}
	if _, err := os.Stat(store.OperationLockPath(storePath)); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote operation lock or stat err = %v", err)
	}
	if _, err := os.Stat(svc.paths.MachineIDPath); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote machine id or stat err = %v", err)
	}
}

func TestSyncYesDeletesConflictsAndUpdatesHeartbeat(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc := newSyncTestService(t, ctx, home)
	defer svc.Close()
	storePath := syncTestStore(t)
	record, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work"}, ActiveProfile: "work"})
	if err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	conflict := filepath.Join(storePath, "profiles", "work", "core", "files", "settings conflicted copy.json")
	keep := filepath.Join(storePath, "profiles", "work", "core", "files", "normal.txt")
	writeAppFile(t, conflict, "losing")
	writeAppFile(t, keep, "keep")

	result, err := svc.Sync(ctx, SyncRequest{StorePath: storePath, Yes: true})
	if err != nil {
		t.Fatalf("Sync(yes) error = %v", err)
	}
	if result.DeletedCount != 1 || result.WouldDeleteCount != 1 || !result.HeartbeatUpdated || result.MachineID != record.MachineID {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(conflict); !os.IsNotExist(err) {
		t.Fatalf("conflict still exists or stat err = %v", err)
	}
	if got := readAppFile(t, keep); got != "keep" {
		t.Fatalf("normal file changed to %q", got)
	}
	heartbeat, err := machine.ReadHeartbeat(storePath, record.MachineID)
	if err != nil {
		t.Fatalf("ReadHeartbeat() error = %v", err)
	}
	if heartbeat.ActiveProfile != "work" {
		t.Fatalf("heartbeat = %+v", heartbeat)
	}
}

func TestSyncYesRequiresRegisteredMachine(t *testing.T) {
	ctx := context.Background()
	svc := newSyncTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := syncTestStore(t)
	conflict := filepath.Join(storePath, "profiles", "work", "core", "files", "settings conflicted copy.json")
	writeAppFile(t, conflict, "losing")

	_, err := svc.Sync(ctx, SyncRequest{StorePath: storePath, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("Sync(yes) error = %v, want not registered", err)
	}
	if _, statErr := os.Stat(conflict); statErr != nil {
		t.Fatalf("unregistered sync deleted conflict or stat failed: %v", statErr)
	}
}

func TestSyncYesFailsWhenStoreOperationLockHeld(t *testing.T) {
	ctx := context.Background()
	svc := newSyncTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := syncTestStore(t)
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	unlock, err := store.AcquireOperationLock(ctx, storePath, store.OperationLockOptions{Operation: "test-holder"})
	if err != nil {
		t.Fatalf("AcquireOperationLock() error = %v", err)
	}
	defer unlock()

	lockCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	_, err = svc.Sync(lockCtx, SyncRequest{StorePath: storePath, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "operation lock") {
		t.Fatalf("Sync() error = %v, want lock error", err)
	}
}

func TestSyncYesAbortsOnExpectedFingerprintDrift(t *testing.T) {
	ctx := context.Background()
	svc := newSyncTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := syncTestStore(t)
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	first := filepath.Join(storePath, "profiles", "work", "core", "files", "settings conflicted copy.json")
	second := filepath.Join(storePath, "profiles", "work", "core", "files", "settings 2 conflicted copy.json")
	writeAppFile(t, first, "losing")
	dryRun, err := svc.Sync(ctx, SyncRequest{StorePath: storePath, DryRun: true})
	if err != nil {
		t.Fatalf("Sync(dry-run) error = %v", err)
	}
	writeAppFile(t, second, "new")

	result, err := svc.Sync(ctx, SyncRequest{StorePath: storePath, Yes: true, ExpectedConflictFingerprint: dryRun.ConflictFingerprint})
	if err == nil || !strings.Contains(err.Error(), "conflict list changed") {
		t.Fatalf("Sync(yes) error = %v, want drift", err)
	}
	if result.WouldDeleteCount != 2 {
		t.Fatalf("result = %+v, want updated conflict count", result)
	}
	for _, path := range []string{first, second} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("drift sync deleted %s or stat failed: %v", path, statErr)
		}
	}
}

func TestSyncYesWithMatchingExpectedFingerprintDeletes(t *testing.T) {
	ctx := context.Background()
	svc := newSyncTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := syncTestStore(t)
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	conflict := filepath.Join(storePath, "profiles", "work", "core", "files", "settings conflicted copy.json")
	writeAppFile(t, conflict, "losing")
	dryRun, err := svc.Sync(ctx, SyncRequest{StorePath: storePath, DryRun: true})
	if err != nil {
		t.Fatalf("Sync(dry-run) error = %v", err)
	}

	result, err := svc.Sync(ctx, SyncRequest{StorePath: storePath, Yes: true, ExpectedConflictFingerprint: dryRun.ConflictFingerprint})
	if err != nil {
		t.Fatalf("Sync(yes) error = %v", err)
	}
	if result.DeletedCount != 1 || result.ConflictFingerprint != dryRun.ConflictFingerprint {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(conflict); !os.IsNotExist(err) {
		t.Fatalf("conflict still exists or stat err = %v", err)
	}
}

func newSyncTestService(t *testing.T, ctx context.Context, home string) *Service {
	t.Helper()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

func syncTestStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	return root
}
