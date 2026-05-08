package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/store"
)

func TestSwitchReportsLocalCopyChangesBeforeSwitch(t *testing.T) {
	ctx := context.Background()
	svc, home, storePath, target, _ := prepareSwitchCaptureService(t)
	defer svc.Close()

	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", Yes: true}); err != nil {
		t.Fatalf("initial Switch() error = %v", err)
	}
	writeAppFile(t, target, "local")

	result, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", DryRun: true})
	if err != nil {
		t.Fatalf("Switch(dry-run) error = %v", err)
	}
	if !result.CaptureRequired || len(result.CapturePlan.Changes) != 1 || result.CapturePlan.Changes[0].TargetPath != target {
		t.Fatalf("capture result = %+v home=%s", result, home)
	}
}

func TestSwitchCaptureLocalWritesBackBeforeActivation(t *testing.T) {
	ctx := context.Background()
	svc, _, storePath, target, source := prepareSwitchCaptureService(t)
	defer svc.Close()

	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", Yes: true}); err != nil {
		t.Fatalf("initial Switch() error = %v", err)
	}
	writeAppFile(t, target, "local")

	result, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", Yes: true, CaptureLocal: true})
	if err != nil {
		t.Fatalf("Switch(capture) error = %v", err)
	}
	if result.Captured != 1 || result.Changed != 1 {
		t.Fatalf("result = %+v", result)
	}
	if got := string(mustReadAppTest(t, source)); got != "local" {
		t.Fatalf("source = %q", got)
	}
	if got := string(mustReadAppTest(t, target)); got != "local" {
		t.Fatalf("target = %q", got)
	}
}

func TestSwitchCaptureLocalDryRunWritesNothing(t *testing.T) {
	ctx := context.Background()
	svc, _, storePath, target, source := prepareSwitchCaptureService(t)
	defer svc.Close()

	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", Yes: true}); err != nil {
		t.Fatalf("initial Switch() error = %v", err)
	}
	writeAppFile(t, target, "local")

	result, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", DryRun: true, CaptureLocal: true})
	if err != nil {
		t.Fatalf("Switch(capture dry-run) error = %v", err)
	}
	if result.CaptureRequired || len(result.CapturePlan.Changes) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if got := string(mustReadAppTest(t, source)); got != "store" {
		t.Fatalf("source changed during dry-run to %q", got)
	}
}

func TestSwitchBlocksWithoutCaptureLocalOnRealSwitch(t *testing.T) {
	ctx := context.Background()
	svc, _, storePath, target, source := prepareSwitchCaptureService(t)
	defer svc.Close()

	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", Yes: true}); err != nil {
		t.Fatalf("initial Switch() error = %v", err)
	}
	writeAppFile(t, target, "local")

	result, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", Yes: true})
	if err == nil || !strings.Contains(err.Error(), "--capture-local") {
		t.Fatalf("Switch(no capture) error = %v result=%+v", err, result)
	}
	if got := string(mustReadAppTest(t, source)); got != "store" {
		t.Fatalf("source changed to %q", got)
	}
}

func prepareSwitchCaptureService(t *testing.T) (*Service, string, string, string, string) {
	t.Helper()
	home := t.TempDir()
	svc, err := NewService(context.Background(), Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	source := filepath.Join(storePath, "profiles", "work", "core", "files", "settings.txt")
	writeAppFile(t, source, "store")
	writeAppFile(t, filepath.Join(storePath, "profiles", "work", "core", "manifest.yaml"), `version: 1
name: work-core
files:
  - id: settings
    source: files/settings.txt
    target: "~/settings.txt"
    mode: copy
skills: []
ignore: []
merge_rules: {}
targets: {}
`)
	if _, err := svc.RegisterMachine(context.Background(), RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	target := filepath.ToSlash(filepath.Join(home, "settings.txt"))
	return svc, home, storePath, target, source
}
