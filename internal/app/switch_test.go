package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/machine"
	"github.com/allensu/loki-profile-manager/internal/store"
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
