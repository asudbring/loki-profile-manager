package app

import (
	"context"
	"path/filepath"
	"testing"

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
	if !status.Configured || status.StorePath != storePath {
		t.Fatalf("status = %+v", status)
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

func testService(t *testing.T) *Service {
	t.Helper()
	svc, err := NewService(context.Background(), Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: t.TempDir()}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}
