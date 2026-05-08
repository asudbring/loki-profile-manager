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
	"github.com/allensu/loki-profile-manager/internal/store"
)

func TestAdoptWritesManagedTargetAndSwitchSafety(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := adoptStore(t)
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	target := filepath.Join(home, ".gitconfig")
	writeAppFile(t, target, "[user]\n\tname = Local\n")

	dryRun, err := svc.Adopt(ctx, AdoptRequest{StorePath: storePath, Target: target, Profile: "work", DryRun: true})
	if err != nil {
		t.Fatalf("Adopt(dry-run) error = %v", err)
	}
	if len(dryRun.Plan.Items) != 1 || dryRun.Changed != 0 {
		t.Fatalf("dry-run = %+v", dryRun)
	}
	if _, err := os.Stat(filepath.Join(storePath, "profiles", "work", "core", "files", ".gitconfig")); !os.IsNotExist(err) {
		t.Fatalf("dry-run store copy exists or stat err = %v", err)
	}

	if _, err := svc.Adopt(ctx, AdoptRequest{StorePath: storePath, Target: target, Profile: "work"}); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("Adopt(no yes) error = %v", err)
	}

	result, err := svc.Adopt(ctx, AdoptRequest{StorePath: storePath, Target: target, Profile: "work", Yes: true})
	if err != nil {
		t.Fatalf("Adopt() error = %v", err)
	}
	if result.Changed != 1 {
		t.Fatalf("result = %+v", result)
	}
	storeCopy := filepath.Join(storePath, "profiles", "work", "core", "files", ".gitconfig")
	if got := readAppFile(t, storeCopy); !strings.Contains(got, "Local") {
		t.Fatalf("store copy = %q", got)
	}
	managedTargetPath := result.Plan.Items[0].TargetPath
	if _, found, err := activation.GetManagedTarget(ctx, svc.database, managedTargetPath); err != nil || !found {
		t.Fatalf("managed target path=%q found=%v err=%v", managedTargetPath, found, err)
	}
	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", DryRun: true}); err != nil {
		t.Fatalf("Switch(dry-run after adopt) error = %v", err)
	}
	writeAppFile(t, target, "changed")
	changed, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", DryRun: true})
	if err != nil {
		t.Fatalf("Switch(changed target dry-run) error = %v", err)
	}
	if !changed.CaptureRequired || len(changed.CapturePlan.Changes) != 1 {
		t.Fatalf("Switch(changed target) result = %+v", changed)
	}
}

func TestAdoptSymlinkWritesManagedTargetAndSwitchSafety(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := adoptStore(t)
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	realTarget := filepath.Join(home, "real.gitconfig")
	linkTarget := filepath.Join(home, ".gitconfig")
	writeAppFile(t, realTarget, "[user]\n")
	if err := os.Symlink(realTarget, linkTarget); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	result, err := svc.Adopt(ctx, AdoptRequest{StorePath: storePath, Target: linkTarget, Profile: "work", Yes: true})
	if err != nil {
		t.Fatalf("Adopt() error = %v", err)
	}
	if result.Changed != 1 {
		t.Fatalf("result = %+v", result)
	}
	storeCopy := filepath.Join(storePath, "profiles", "work", "core", "files", ".gitconfig")
	if info, err := os.Lstat(storeCopy); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("store copy lstat mode=%v err=%v, want regular file", info.Mode(), err)
	}
	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", DryRun: true}); err != nil {
		t.Fatalf("Switch(dry-run after symlink adopt) error = %v", err)
	}

	if err := os.Remove(linkTarget); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	otherTarget := filepath.Join(home, "other.gitconfig")
	writeAppFile(t, otherTarget, "[user]\n")
	if err := os.Symlink(otherTarget, linkTarget); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", DryRun: true}); err == nil || !strings.Contains(err.Error(), "cannot be captured") {
		t.Fatalf("Switch(changed symlink) error = %v", err)
	}
}

func TestAdoptFailsWhenStoreOperationLockHeld(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := adoptStore(t)
	target := filepath.Join(home, ".gitconfig")
	writeAppFile(t, target, "[user]\n\tname = Local\n")
	unlock, err := store.AcquireOperationLock(ctx, storePath, store.OperationLockOptions{Operation: "test-holder"})
	if err != nil {
		t.Fatalf("AcquireOperationLock() error = %v", err)
	}
	defer unlock()

	lockCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := svc.Adopt(lockCtx, AdoptRequest{StorePath: storePath, Target: target, Profile: "work", Yes: true}); err == nil || !strings.Contains(err.Error(), "operation lock") {
		t.Fatalf("Adopt() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(storePath, "profiles", "work", "core", "files", ".gitconfig")); !os.IsNotExist(err) {
		t.Fatalf("store copy exists or stat err = %v", err)
	}
}

func adoptStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	return root
}
