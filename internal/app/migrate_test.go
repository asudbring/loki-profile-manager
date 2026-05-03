package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/store"
)

func TestMigrateLocalWritesAdoptionRecords(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := adoptStore(t)
	target := filepath.Join(home, ".gitconfig")
	writeAppFile(t, target, "[user]\n\temail = local@example.test\n")

	result, err := svc.MigrateLocal(ctx, MigrateLocalRequest{StorePath: storePath, Profile: "work", Yes: true})
	if err != nil {
		t.Fatalf("MigrateLocal() error = %v", err)
	}
	if len(result.Plan.Items) != 1 || result.Changed != 1 {
		t.Fatalf("result = %+v", result)
	}
	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", DryRun: true}); err != nil {
		t.Fatalf("Switch(dry-run after migrate local) error = %v", err)
	}
}

func TestMigrateLocalFailsWhenStoreOperationLockHeld(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := adoptStore(t)
	writeAppFile(t, filepath.Join(home, ".gitconfig"), "[user]\n\temail = local@example.test\n")
	unlock, err := store.AcquireOperationLock(ctx, storePath, store.OperationLockOptions{Operation: "test-holder"})
	if err != nil {
		t.Fatalf("AcquireOperationLock() error = %v", err)
	}
	defer unlock()

	lockCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := svc.MigrateLocal(lockCtx, MigrateLocalRequest{StorePath: storePath, Profile: "work", Yes: true}); err == nil || !strings.Contains(err.Error(), "operation lock") {
		t.Fatalf("MigrateLocal() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(storePath, "profiles", "work", "core", "files", ".gitconfig")); !os.IsNotExist(err) {
		t.Fatalf("store copy exists or stat err = %v", err)
	}
}

func TestMigrateRepoAdoptsMatchingExistingTarget(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := adoptStore(t)
	repo := t.TempDir()
	writeAppFile(t, filepath.Join(repo, ".gitconfig"), "[core]\n\teditor = vim\n")
	writeAppFile(t, filepath.Join(home, ".gitconfig"), "[core]\n\teditor = vim\n")

	result, err := svc.MigrateRepo(ctx, MigrateRepoRequest{StorePath: storePath, RepoPath: repo, Profile: "work", Yes: true})
	if err != nil {
		t.Fatalf("MigrateRepo() error = %v", err)
	}
	if len(result.Plan.Items) != 1 || !result.Plan.Items[0].WillAdoptRecord {
		t.Fatalf("result = %+v", result)
	}
	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", DryRun: true}); err != nil {
		t.Fatalf("Switch(dry-run after repo migrate) error = %v", err)
	}
}

func TestMigrateRepoFailsWhenStoreOperationLockHeld(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := adoptStore(t)
	repo := t.TempDir()
	writeAppFile(t, filepath.Join(repo, ".gitconfig"), "[core]\n\teditor = vim\n")
	writeAppFile(t, filepath.Join(home, ".gitconfig"), "[core]\n\teditor = vim\n")
	unlock, err := store.AcquireOperationLock(ctx, storePath, store.OperationLockOptions{Operation: "test-holder"})
	if err != nil {
		t.Fatalf("AcquireOperationLock() error = %v", err)
	}
	defer unlock()

	lockCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := svc.MigrateRepo(lockCtx, MigrateRepoRequest{StorePath: storePath, RepoPath: repo, Profile: "work", Yes: true}); err == nil || !strings.Contains(err.Error(), "operation lock") {
		t.Fatalf("MigrateRepo() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(storePath, "profiles", "work", "core", "files", ".gitconfig")); !os.IsNotExist(err) {
		t.Fatalf("store copy exists or stat err = %v", err)
	}
}
