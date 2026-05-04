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
	"github.com/allensu/loki-profile-manager/internal/machine"
)

func TestStatusNotConfigured(t *testing.T) {
	tmp := t.TempDir()
	svc, err := NewService(context.Background(), Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: tmp}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	status, err := svc.Status(context.Background(), StatusRequest{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Configured {
		t.Fatal("Configured = true, want false")
	}
	wantState := filepath.ToSlash(filepath.Join(tmp, "Library", "Application Support", "loki-profile-manager"))
	if status.LocalStatePath != wantState {
		t.Fatalf("LocalStatePath = %q, want %q", status.LocalStatePath, wantState)
	}
	if status.DatabasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
}

func TestStatusIncludesStoreOverride(t *testing.T) {
	tmp := t.TempDir()
	svc, err := NewService(context.Background(), Options{
		Resolver:      config.PathResolver{GOOS: "darwin", HomeDir: tmp},
		StoreOverride: "/tmp//loki/",
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	status, err := svc.Status(context.Background(), StatusRequest{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.StoreOverride != "/tmp/loki" {
		t.Fatalf("StoreOverride = %q", status.StoreOverride)
	}
	if status.StorePath != status.StoreOverride {
		t.Fatalf("StorePath = %q, want %q", status.StorePath, status.StoreOverride)
	}
}

func TestEnsureStoreCreatesValidStoreAndStatusConfigured(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	storePath := filepath.Join(t.TempDir(), "loki")
	wantStorePath := config.CleanForOS("darwin", storePath)

	result, err := svc.EnsureStore(context.Background(), EnsureStoreRequest{StorePath: storePath})
	if err != nil {
		t.Fatalf("EnsureStore() error = %v", err)
	}
	if !result.Created || !result.Valid {
		t.Fatalf("EnsureStore() result = %+v", result)
	}
	status, err := svc.Status(context.Background(), StatusRequest{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Configured || status.StorePath != wantStorePath {
		t.Fatalf("status = %+v, want store %s", status, wantStorePath)
	}
}

func TestEnsureMachineIDReusesSameID(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	first, err := svc.EnsureMachineID(context.Background())
	if err != nil {
		t.Fatalf("EnsureMachineID() error = %v", err)
	}
	second, err := svc.EnsureMachineID(context.Background())
	if err != nil {
		t.Fatalf("EnsureMachineID() second error = %v", err)
	}
	if first != second {
		t.Fatalf("second id = %q, want %q", second, first)
	}
}

func TestStatusReportsUnregisteredMachine(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := svc.EnsureStore(context.Background(), EnsureStoreRequest{StorePath: storePath}); err != nil {
		t.Fatalf("EnsureStore() error = %v", err)
	}
	machineID, err := svc.EnsureMachineID(context.Background())
	if err != nil {
		t.Fatalf("EnsureMachineID() error = %v", err)
	}

	status, err := svc.Status(context.Background(), StatusRequest{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.MachineID != machineID || status.MachineRegistered {
		t.Fatalf("status = %+v, want unregistered machine %s", status, machineID)
	}
	if !strings.Contains(status.MachineWarning, "not registered") {
		t.Fatalf("MachineWarning = %q", status.MachineWarning)
	}
}

func TestStatusIncludesRegisteredMachine(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := svc.EnsureStore(context.Background(), EnsureStoreRequest{StorePath: storePath}); err != nil {
		t.Fatalf("EnsureStore() error = %v", err)
	}
	registered, err := svc.RegisterMachine(context.Background(), RegisterMachineRequest{StorePath: storePath, DisplayName: "test machine", AllowedParentProfiles: []string{"work"}, AllowedBuckets: []string{"azure"}})
	if err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}

	status, err := svc.Status(context.Background(), StatusRequest{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.MachineRegistered || status.MachineID != registered.MachineID {
		t.Fatalf("status = %+v, want registered machine %s", status, registered.MachineID)
	}
	if status.MachineDisplayName != "test machine" || len(status.MachineAllowedParentProfiles) != 1 || status.MachineAllowedParentProfiles[0] != "work" {
		t.Fatalf("machine status fields = %+v", status)
	}
}

func TestStatusIncludesLocalActiveStateAndManagedTargets(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := svc.EnsureStore(ctx, EnsureStoreRequest{StorePath: storePath}); err != nil {
		t.Fatalf("EnsureStore() error = %v", err)
	}
	if err := activation.SetActiveState(ctx, svc.database, "work", []string{"azure"}); err != nil {
		t.Fatalf("SetActiveState() error = %v", err)
	}
	targetRoot := t.TempDir()
	first := filepath.Join(targetRoot, "b.txt")
	second := filepath.Join(targetRoot, "a.txt")
	for _, target := range []string{first, second} {
		op := activation.Operation{Type: activation.OperationCopy, TargetPath: target, SourcePath: target + ".source", LayerKind: "core", LayerName: "work"}
		if err := activation.PutManagedTarget(ctx, svc.database, activation.ManagedTarget{TargetPath: op.TargetPath, SourcePath: op.SourcePath, Mode: string(op.Type), ContentHash: "hash", LayerKind: op.LayerKind, LayerName: op.LayerName, LastAppliedAt: time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)}); err != nil {
			t.Fatalf("PutManagedTarget() error = %v", err)
		}
	}

	status, err := svc.Status(ctx, StatusRequest{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.ActiveProfile != "work" || status.ActiveSource != "local_state" || len(status.ActiveBuckets) != 1 || status.ActiveBuckets[0] != "azure" {
		t.Fatalf("active status = %+v", status)
	}
	if status.ManagedTargetCount != 2 || len(status.ManagedTargets) != 2 {
		t.Fatalf("managed target status = %+v", status.ManagedTargets)
	}
	if status.ManagedTargets[0].TargetPath != second || status.ManagedTargets[1].TargetPath != first {
		t.Fatalf("managed targets not sorted: %+v", status.ManagedTargets)
	}
}

func TestStatusFallsBackToMachineActiveState(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := svc.EnsureStore(ctx, EnsureStoreRequest{StorePath: storePath}); err != nil {
		t.Fatalf("EnsureStore() error = %v", err)
	}
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"work"}, ActiveProfile: "work", ActiveBuckets: []string{"azure"}}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}

	status, err := svc.Status(ctx, StatusRequest{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.ActiveProfile != "work" || status.ActiveSource != "machine_registry" || len(status.ActiveBuckets) != 1 || status.ActiveBuckets[0] != "azure" {
		t.Fatalf("status = %+v", status)
	}
}

func TestStatusPrefersLocalActiveOverMachineRegistry(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := svc.EnsureStore(ctx, EnsureStoreRequest{StorePath: storePath}); err != nil {
		t.Fatalf("EnsureStore() error = %v", err)
	}
	if _, err := svc.RegisterMachine(ctx, RegisterMachineRequest{StorePath: storePath, AllowedParentProfiles: []string{"dev"}, ActiveProfile: "dev"}); err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	if err := activation.SetActiveState(ctx, svc.database, "work", nil); err != nil {
		t.Fatalf("SetActiveState() error = %v", err)
	}

	status, err := svc.Status(ctx, StatusRequest{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.ActiveProfile != "work" || status.ActiveSource != "local_state" {
		t.Fatalf("status = %+v", status)
	}
}

func TestListAndShowSnapshots(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot, err := activation.CreateSnapshot(ctx, activation.CreateSnapshotRequest{
		Database:              svc.database,
		SnapshotRoot:          svc.paths.SnapshotDir,
		Plan:                  activation.Plan{Profile: "work", Operations: []activation.Operation{{Type: activation.OperationCopy, TargetPath: target}}},
		PreviousActiveProfile: "dev",
	})
	if err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}
	list, err := svc.ListSnapshots(ctx, SnapshotListRequest{})
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if list.SnapshotDir != svc.paths.SnapshotDir || len(list.Snapshots) != 1 || list.Snapshots[0].SnapshotID != snapshot.SnapshotID {
		t.Fatalf("list = %+v", list)
	}
	show, err := svc.ShowSnapshot(ctx, SnapshotShowRequest{SnapshotID: snapshot.SnapshotID})
	if err != nil {
		t.Fatalf("ShowSnapshot() error = %v", err)
	}
	if show.Snapshot.SnapshotID != snapshot.SnapshotID || len(show.Snapshot.Targets) != 1 || show.Snapshot.Targets[0].TargetPath != target {
		t.Fatalf("show = %+v", show)
	}
}

func TestRestoreSnapshotRequiresOneMode(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	_, err := svc.RestoreSnapshot(context.Background(), SnapshotRestoreRequest{SnapshotID: "missing"})
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("RestoreSnapshot() error = %v", err)
	}
}

func TestRestoreSnapshotDryRunReturnsPlanWithoutWriting(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	root := t.TempDir()
	target := filepath.Join(root, "created.txt")
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	plan := activation.Plan{Profile: "work", Operations: []activation.Operation{{Type: activation.OperationCopy, SourcePath: source, TargetPath: target}}}
	result, err := activation.Execute(ctx, activation.ExecuteRequest{Database: svc.database, LocalPaths: svc.paths, Plan: plan})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	dryRun, err := svc.RestoreSnapshotDryRun(ctx, SnapshotRestoreDryRunRequest{SnapshotID: result.Snapshot.SnapshotID, DryRun: true})
	if err != nil {
		t.Fatalf("RestoreSnapshotDryRun() error = %v", err)
	}
	if !dryRun.DryRun || dryRun.WouldWrite || !dryRun.GuardRecorded || dryRun.Summary.TargetCount != 1 || dryRun.Summary.RemoveCreatedTargetCount != 1 {
		t.Fatalf("dryRun = %+v", dryRun)
	}
	if got := string(mustReadAppTest(t, target)); got != "new" {
		t.Fatalf("target changed to %q", got)
	}
}

func TestRestoreSnapshotYesRequiresGuard(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	root := t.TempDir()
	target := filepath.Join(root, "created.txt")
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	result, err := activation.Execute(ctx, activation.ExecuteRequest{Database: svc.database, LocalPaths: svc.paths, Plan: activation.Plan{Profile: "work", Operations: []activation.Operation{{Type: activation.OperationCopy, SourcePath: source, TargetPath: target}}}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	_, err = svc.RestoreSnapshot(ctx, SnapshotRestoreRequest{SnapshotID: result.Snapshot.SnapshotID, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "dry-run guard") {
		t.Fatalf("RestoreSnapshot(--yes) error = %v", err)
	}
}

func TestRestoreSnapshotYesAfterDryRunRestores(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	root := t.TempDir()
	target := filepath.Join(root, "created.txt")
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	result, err := activation.Execute(ctx, activation.ExecuteRequest{Database: svc.database, LocalPaths: svc.paths, Plan: activation.Plan{Profile: "work", Operations: []activation.Operation{{Type: activation.OperationCopy, SourcePath: source, TargetPath: target}}}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := svc.RestoreSnapshot(ctx, SnapshotRestoreRequest{SnapshotID: result.Snapshot.SnapshotID, DryRun: true}); err != nil {
		t.Fatalf("RestoreSnapshot(--dry-run) error = %v", err)
	}
	restored, err := svc.RestoreSnapshot(ctx, SnapshotRestoreRequest{SnapshotID: result.Snapshot.SnapshotID, Yes: true})
	if err != nil {
		t.Fatalf("RestoreSnapshot(--yes) error = %v", err)
	}
	if !restored.Restored || restored.PreRestoreSnapshotID == "" || restored.Changed != 1 {
		t.Fatalf("restored = %+v", restored)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after restore or stat err = %v", err)
	}
}

func TestRestoreSnapshotTargetGuardIsScoped(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	root := t.TempDir()
	firstSource := filepath.Join(root, "first-source.txt")
	secondSource := filepath.Join(root, "second-source.txt")
	firstTarget := filepath.Join(root, "first.txt")
	secondTarget := filepath.Join(root, "second.txt")
	if err := os.WriteFile(firstSource, []byte("first"), 0o644); err != nil {
		t.Fatalf("WriteFile(firstSource) error = %v", err)
	}
	if err := os.WriteFile(secondSource, []byte("second"), 0o644); err != nil {
		t.Fatalf("WriteFile(secondSource) error = %v", err)
	}
	plan := activation.Plan{Profile: "work", Operations: []activation.Operation{{Type: activation.OperationCopy, SourcePath: firstSource, TargetPath: firstTarget}, {Type: activation.OperationCopy, SourcePath: secondSource, TargetPath: secondTarget}}}
	result, err := activation.Execute(ctx, activation.ExecuteRequest{Database: svc.database, LocalPaths: svc.paths, Plan: plan})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := svc.RestoreSnapshot(ctx, SnapshotRestoreRequest{SnapshotID: result.Snapshot.SnapshotID, DryRun: true, Target: firstTarget}); err != nil {
		t.Fatalf("RestoreSnapshot(first --dry-run) error = %v", err)
	}
	_, err = svc.RestoreSnapshot(ctx, SnapshotRestoreRequest{SnapshotID: result.Snapshot.SnapshotID, Yes: true, Target: secondTarget})
	if err == nil || !strings.Contains(err.Error(), "dry-run guard") {
		t.Fatalf("RestoreSnapshot(second --yes) error = %v", err)
	}
	restored, err := svc.RestoreSnapshot(ctx, SnapshotRestoreRequest{SnapshotID: result.Snapshot.SnapshotID, Yes: true, Target: firstTarget})
	if err != nil {
		t.Fatalf("RestoreSnapshot(first --yes) error = %v", err)
	}
	if !restored.Restored || restored.Summary.TargetCount != 1 {
		t.Fatalf("restored = %+v", restored)
	}
	if _, err := os.Stat(firstTarget); !os.IsNotExist(err) {
		t.Fatalf("first target exists after restore or stat err = %v", err)
	}
	if got := string(mustReadAppTest(t, secondTarget)); got != "second" {
		t.Fatalf("second target changed to %q", got)
	}
}

func TestRegisterPolicyHeartbeatAndDeleteMachine(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := svc.EnsureStore(context.Background(), EnsureStoreRequest{StorePath: storePath}); err != nil {
		t.Fatalf("EnsureStore() error = %v", err)
	}
	record, err := svc.RegisterMachine(context.Background(), RegisterMachineRequest{
		StorePath:             storePath,
		DisplayName:           "test machine",
		AllowedParentProfiles: []string{"work"},
		AllowedBuckets:        []string{"content-dev", "azure"},
		ActiveProfile:         "work",
		ActiveBuckets:         []string{"content-dev"},
	})
	if err != nil {
		t.Fatalf("RegisterMachine() error = %v", err)
	}
	if record.MachineID == "" {
		t.Fatal("MachineID empty")
	}
	if _, err := machine.ReadHeartbeat(storePath, record.MachineID); err != nil {
		t.Fatalf("ReadHeartbeat() error = %v", err)
	}
	if err := svc.ValidateMachinePolicy(context.Background(), ValidatePolicyRequest{StorePath: storePath, MachineID: record.MachineID, ParentProfile: "work", Buckets: []string{"azure"}}); err != nil {
		t.Fatalf("ValidateMachinePolicy() error = %v", err)
	}
	updated, err := svc.WriteHeartbeat(context.Background(), HeartbeatRequest{StorePath: storePath, ActiveProfile: "work", ActiveBuckets: []string{"azure"}})
	if err != nil {
		t.Fatalf("WriteHeartbeat() error = %v", err)
	}
	if len(updated.ActiveBuckets) != 1 || updated.ActiveBuckets[0] != "azure" {
		t.Fatalf("updated heartbeat = %+v", updated)
	}
	if err := svc.DeleteMachine(context.Background(), storePath, record.MachineID); err != nil {
		t.Fatalf("DeleteMachine() error = %v", err)
	}
	if err := svc.DeleteMachine(context.Background(), storePath, record.MachineID); err == nil {
		t.Fatal("DeleteMachine() unknown error = nil, want error")
	}
}

func mustReadAppTest(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return content
}

func testService(t *testing.T) *Service {
	t.Helper()
	svc, err := NewService(context.Background(), Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: t.TempDir()}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}
