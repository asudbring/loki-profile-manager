package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/machine"
	"github.com/asudbring/loki-profile-manager/internal/store"
)

func TestSwitchDryRunWritesNothingAndRealSwitchUpdatesHeartbeat(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := switchStore(t, "activated.txt", "hello")
	registered, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work"}, AllowedBuckets: []string{"azure"}})
	if err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	target := filepath.ToSlash(filepath.Join(home, "activated.txt"))

	dryRun, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", Buckets: []string{"azure"}, DryRun: true})
	if err != nil {
		t.Fatalf("Switch(dry-run) error = %v", err)
	}
	if len(dryRun.Plan.Operations) != 1 || dryRun.Changed != 0 {
		t.Fatalf("dry-run = %+v", dryRun)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("dry-run target exists or stat err = %v", err)
	}

	result, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", Buckets: []string{"azure"}})
	if err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	if result.SnapshotID == "" || result.Changed != 1 {
		t.Fatalf("result = %+v", result)
	}
	if got := readAppFile(t, target); got != "hello" {
		t.Fatalf("target = %q", got)
	}
	record, err := machine.ReadHeartbeat(storePath, registered.MachineID)
	if err != nil {
		t.Fatalf("ReadHeartbeat() error = %v", err)
	}
	if record.ActiveProfile != "work" || len(record.ActiveBuckets) != 1 || record.ActiveBuckets[0] != "azure" {
		t.Fatalf("heartbeat = %+v", record)
	}
}

func TestSwitchFailsWhenMachineUnregistered(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := switchStore(t, "unregistered.txt", "hello")
	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work"}); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("Switch() error = %v", err)
	}
	target := filepath.ToSlash(filepath.Join(home, "unregistered.txt"))
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists or stat err = %v", err)
	}
}

func TestSwitchEnforcesPolicyAndUnsafeOverwrite(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := switchStore(t, "blocked.txt", "new")
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"dev"}, AllowedBuckets: []string{"safe"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work"}); err == nil || !strings.Contains(err.Error(), "machine policy") {
		t.Fatalf("policy error = %v", err)
	}

	// Re-register same machine with policy that allows the switch, then verify unmanaged overwrite blocks.
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work"}, AllowedBuckets: []string{"azure"}}); err != nil {
		t.Fatalf("RegisterMachine(allowed) error = %v", err)
	}
	target := filepath.ToSlash(filepath.Join(home, "blocked.txt"))
	if err := os.WriteFile(target, []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", Buckets: []string{"azure"}}); err == nil || !strings.Contains(err.Error(), "unsafe target overwrite") {
		t.Fatalf("unsafe overwrite error = %v", err)
	}
	if got := readAppFile(t, target); got != "local" {
		t.Fatalf("unsafe target changed to %q", got)
	}
}

func TestSwitchBackupUnmanagedMovesBlockersThenSwitches(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := switchStore(t, "blocked.txt", "store")
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	target := filepath.ToSlash(filepath.Join(home, "blocked.txt"))
	if err := os.WriteFile(target, []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", BackupUnmanaged: true, Yes: true})
	if err != nil {
		t.Fatalf("Switch(backup unmanaged) error = %v", err)
	}
	if result.Changed != 1 || len(result.UnmanagedBackups) != 1 || result.UnmanagedBackupRoot == "" {
		t.Fatalf("result = %+v", result)
	}
	backup := result.UnmanagedBackups[0]
	if backup.TargetPath != target || backup.BackupPath == "" || backup.SafetyClass != activation.SafetyUnmanagedFile {
		t.Fatalf("backup = %+v", backup)
	}
	if got := readAppFile(t, backup.BackupPath); got != "local" {
		t.Fatalf("backup content = %q", got)
	}
	if got := readAppFile(t, target); got != "store" {
		t.Fatalf("target content = %q", got)
	}
	if _, err := os.Stat(filepath.Join(result.UnmanagedBackupRoot, "manifest.json")); err != nil {
		t.Fatalf("backup manifest missing: %v", err)
	}
}

func TestBackupUnmanagedCreatesUniqueRoots(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	first := filepath.Join(home, "first.txt")
	second := filepath.Join(home, "second.txt")
	if err := os.WriteFile(first, []byte("first"), 0o644); err != nil {
		t.Fatalf("WriteFile(first) error = %v", err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o644); err != nil {
		t.Fatalf("WriteFile(second) error = %v", err)
	}
	firstOp := activation.Operation{TargetPath: first, Safety: activation.SafetyStatus{Class: activation.SafetyUnmanagedFile, Safe: false}}
	secondOp := activation.Operation{TargetPath: second, Safety: activation.SafetyStatus{Class: activation.SafetyUnmanagedFile, Safe: false}}
	firstRoot, _, err := svc.backupUnmanagedTargets(activation.Plan{Profile: "work"}, []activation.Operation{firstOp})
	if err != nil {
		t.Fatalf("first backup error = %v", err)
	}
	secondRoot, _, err := svc.backupUnmanagedTargets(activation.Plan{Profile: "work"}, []activation.Operation{secondOp})
	if err != nil {
		t.Fatalf("second backup error = %v", err)
	}
	if firstRoot == secondRoot {
		t.Fatalf("backup roots collided: %s", firstRoot)
	}
}

func TestBackupUnmanagedRefusesTargetContainingLocalState(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	target := filepath.Join(home, "Library", "Application Support")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	op := activation.Operation{TargetPath: target, Safety: activation.SafetyStatus{Class: activation.SafetyUnmanagedDirectory, Safe: false}}
	_, _, err = svc.backupUnmanagedTargets(activation.Plan{Profile: "work"}, []activation.Operation{op})
	if err == nil || !strings.Contains(err.Error(), "contains Loki local state") {
		t.Fatalf("backupUnmanagedTargets() error = %v", err)
	}
	if got := readAppFile(t, filepath.Join(target, "keep.txt")); got != "keep" {
		t.Fatalf("target content changed to %q", got)
	}
}

func TestPathContainsOrEqualIsCaseInsensitiveOnDarwinWindows(t *testing.T) {
	parent := filepath.Join("/Users", "Allen", "Library", "Application Support")
	child := filepath.Join("/users", "allen", "library", "application support", "loki-profile-manager", "state.sqlite")
	if !pathContainsOrEqual(parent, child, "darwin") {
		t.Fatalf("pathContainsOrEqual(%q, %q, darwin) = false, want true", parent, child)
	}
}

func TestBackupUnmanagedRefusesTargetInsideLocalState(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	target := svc.paths.DBPath
	op := activation.Operation{TargetPath: target, Safety: activation.SafetyStatus{Class: activation.SafetyUnmanagedFile, Safe: false}}
	_, _, err = svc.backupUnmanagedTargets(activation.Plan{Profile: "work"}, []activation.Operation{op})
	if err == nil || !strings.Contains(err.Error(), "inside Loki local state") {
		t.Fatalf("backupUnmanagedTargets() error = %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("local state target changed: %v", err)
	}
}

func TestSwitchBackupUnmanagedRequiresYes(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := switchStore(t, "blocked.txt", "store")
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	target := filepath.ToSlash(filepath.Join(home, "blocked.txt"))
	if err := os.WriteFile(target, []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", BackupUnmanaged: true}); err == nil || !strings.Contains(err.Error(), "--backup-unmanaged requires --yes") {
		t.Fatalf("Switch() error = %v", err)
	}
	if got := readAppFile(t, target); got != "local" {
		t.Fatalf("target changed to %q", got)
	}
}

func TestSwitchRemovesObsoleteManagedTargets(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := cleanupSwitchStore(t)
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work", "dev"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	oldTarget := filepath.Join(home, "old-profile.txt")
	newTarget := filepath.Join(home, "new-profile.txt")

	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work"}); err != nil {
		t.Fatalf("Switch(work) error = %v", err)
	}
	if got := readAppFile(t, oldTarget); got != "old" {
		t.Fatalf("old target = %q", got)
	}

	result, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "dev"})
	if err != nil {
		t.Fatalf("Switch(dev) error = %v", err)
	}
	if result.Cleaned != 1 || len(result.CleanupPlan.Changes) != 1 {
		t.Fatalf("cleanup result = %+v", result)
	}
	if _, err := os.Stat(oldTarget); !os.IsNotExist(err) {
		t.Fatalf("old target exists or stat err = %v", err)
	}
	if got := readAppFile(t, newTarget); got != "new" {
		t.Fatalf("new target = %q", got)
	}
	if _, found, err := activation.GetManagedTarget(ctx, svc.database, oldTarget); err != nil || found {
		t.Fatalf("old managed record found=%v err=%v", found, err)
	}
}

func TestSwitchBlocksChangedObsoleteManagedTargets(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := cleanupSwitchStore(t)
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work", "dev"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	oldTarget := filepath.Join(home, "old-profile.txt")
	newTarget := filepath.Join(home, "new-profile.txt")

	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work"}); err != nil {
		t.Fatalf("Switch(work) error = %v", err)
	}
	writeAppFile(t, oldTarget, "local edit")
	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "dev"}); err == nil || !strings.Contains(err.Error(), "local changes") {
		t.Fatalf("Switch(dev) error = %v", err)
	}
	if got := readAppFile(t, oldTarget); got != "local edit" {
		t.Fatalf("old target = %q", got)
	}
	if _, err := os.Stat(newTarget); !os.IsNotExist(err) {
		t.Fatalf("new target exists or stat err = %v", err)
	}
}

func TestSwitchBlocksChangedStaleObsoleteManagedTargets(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := cleanupSwitchStore(t)
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"dev"}, ActiveProfile: "dev"}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	staleSource := filepath.Join(storePath, "profiles", "work", "core", "files", "stale.txt")
	staleTarget := filepath.Join(home, "stale.txt")
	newTarget := filepath.Join(home, "new-profile.txt")
	writeAppFile(t, staleSource, "store")
	writeAppFile(t, staleTarget, "store")
	hash, err := activation.HashPath(staleTarget)
	if err != nil {
		t.Fatalf("HashPath() error = %v", err)
	}
	staleOp := activation.Operation{ID: "stale", Type: activation.OperationCopy, TargetPath: staleTarget, SourcePath: staleSource, LayerName: "stale", LayerKind: "core"}
	if err := activation.UpsertManagedTarget(ctx, svc.database, staleOp, hash, time.Now()); err != nil {
		t.Fatalf("UpsertManagedTarget() error = %v", err)
	}
	writeAppFile(t, staleTarget, "local edit")

	result, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "dev"})
	if err == nil || !strings.Contains(err.Error(), "obsolete managed targets") {
		t.Fatalf("Switch(dev) error = %v result=%+v", err, result)
	}
	if len(result.CleanupPlan.Changes) != 1 || result.CleanupPlan.Changes[0].Status != activation.CleanupBlocked {
		t.Fatalf("cleanup plan = %+v", result.CleanupPlan)
	}
	if got := readAppFile(t, staleTarget); got != "local edit" {
		t.Fatalf("stale target = %q", got)
	}
	if _, err := os.Stat(newTarget); !os.IsNotExist(err) {
		t.Fatalf("new target exists or stat err = %v", err)
	}
}

func TestSwitchRegeneratesChangedRenderTargets(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}, SecretProvider: fakeAppSecretProvider{values: map[string]string{}}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := renderSwitchStore(t)
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work", "dev"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	target := filepath.Join(home, ".codex", "config.toml")

	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work"}); err != nil {
		t.Fatalf("Switch(work) error = %v", err)
	}
	if got := readAppFile(t, target); got != "profile = \"work\"\n" {
		t.Fatalf("work render target = %q", got)
	}
	writeAppFile(t, target, "profile = \"runtime\"\n")

	result, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "dev"})
	if err != nil {
		t.Fatalf("Switch(dev) error = %v result=%+v", err, result)
	}
	if len(result.CapturePlan.Changes) != 0 {
		t.Fatalf("capture plan = %+v", result.CapturePlan)
	}
	if got := readAppFile(t, target); got != "profile = \"dev\"\n" {
		t.Fatalf("dev render target = %q", got)
	}
}

func TestSwitchFailsWhenStoreOperationLockHeld(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := switchStore(t, "locked.txt", "hello")
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work"}, AllowedBuckets: []string{"azure"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	unlock, err := store.AcquireOperationLock(ctx, storePath, store.OperationLockOptions{Operation: "test-holder"})
	if err != nil {
		t.Fatalf("AcquireOperationLock() error = %v", err)
	}
	defer unlock()

	lockCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := svc.Switch(lockCtx, SwitchRequest{StorePath: storePath, ParentProfile: "work", Buckets: []string{"azure"}}); err == nil || !strings.Contains(err.Error(), "operation lock") {
		t.Fatalf("Switch() error = %v", err)
	}
	target := filepath.ToSlash(filepath.Join(home, "locked.txt"))
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists or stat err = %v", err)
	}
}

func cleanupSwitchStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	writeAppFile(t, filepath.Join(root, "profiles", "work", "core", "files", "old-profile.txt"), "old")
	writeAppFile(t, filepath.Join(root, "profiles", "work", "core", "manifest.yaml"), `version: 1
name: work-core
files:
  - id: old-file
    source: files/old-profile.txt
    target: "~/old-profile.txt"
    mode: copy
skills: []
ignore: []
merge_rules: {}
targets: {}
`)
	writeAppFile(t, filepath.Join(root, "profiles", "dev", "core", "files", "new-profile.txt"), "new")
	writeAppFile(t, filepath.Join(root, "profiles", "dev", "core", "manifest.yaml"), `version: 1
name: dev-core
files:
  - id: new-file
    source: files/new-profile.txt
    target: "~/new-profile.txt"
    mode: copy
skills: []
ignore: []
merge_rules: {}
targets: {}
`)
	return root
}

func renderSwitchStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	for _, item := range []struct {
		profile string
		content string
	}{
		{profile: "work", content: "profile = \"work\"\n"},
		{profile: "dev", content: "profile = \"dev\"\n"},
	} {
		writeAppFile(t, filepath.Join(root, "profiles", item.profile, "core", "templates", "config.toml.template"), item.content)
		writeAppFile(t, filepath.Join(root, "profiles", item.profile, "core", "manifest.yaml"), `version: 1
name: `+item.profile+`-core
files:
  - id: codex-config
    source: templates/config.toml.template
    target: "~/.codex/config.toml"
    mode: render
skills: []
ignore: []
merge_rules: {}
targets: {}
`)
	}
	return root
}

func switchStore(t *testing.T, targetName, content string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	writeAppFile(t, filepath.Join(root, "profiles", "work", "core", "files", targetName), content)
	writeAppFile(t, filepath.Join(root, "profiles", "work", "core", "manifest.yaml"), `version: 1
name: work-core
files:
  - id: test-file
    source: files/`+targetName+`
    target: "~/`+targetName+`"
    mode: copy
skills: []
ignore: []
merge_rules: {}
targets: {}
`)
	writeAppFile(t, filepath.Join(root, "profiles", "work", "buckets", "azure", "manifest.yaml"), `version: 1
name: azure
files: []
skills: []
ignore: []
merge_rules: {}
targets: {}
`)
	return root
}

func writeAppFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func readAppFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(content)
}
