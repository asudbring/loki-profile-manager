package machine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/allensu/loki-profile-manager/internal/store"
)

func TestUpsertMachineWritesRegistryAndHeartbeat(t *testing.T) {
	root := testStore(t)
	record := testRecord("machine-1")
	if err := UpsertMachine(root, record); err != nil {
		t.Fatalf("UpsertMachine() error = %v", err)
	}
	registry, err := ReadRegistry(root)
	if err != nil {
		t.Fatalf("ReadRegistry() error = %v", err)
	}
	if len(registry.Machines) != 1 || registry.Machines[0].MachineID != record.MachineID {
		t.Fatalf("registry = %+v", registry)
	}
	heartbeat, err := ReadHeartbeat(root, record.MachineID)
	if err != nil {
		t.Fatalf("ReadHeartbeat() error = %v", err)
	}
	if heartbeat.MachineID != record.MachineID || heartbeat.ActiveProfile != record.ActiveProfile {
		t.Fatalf("heartbeat = %+v", heartbeat)
	}
}

func TestUpsertMachineUpdatesWithoutDuplicate(t *testing.T) {
	root := testStore(t)
	record := testRecord("machine-1")
	if err := UpsertMachine(root, record); err != nil {
		t.Fatalf("UpsertMachine() error = %v", err)
	}
	record.DisplayName = "updated"
	if err := UpsertMachine(root, record); err != nil {
		t.Fatalf("UpsertMachine() second error = %v", err)
	}
	registry, err := ReadRegistry(root)
	if err != nil {
		t.Fatalf("ReadRegistry() error = %v", err)
	}
	if len(registry.Machines) != 1 || registry.Machines[0].DisplayName != "updated" {
		t.Fatalf("registry = %+v", registry)
	}
}

func TestUpdateHeartbeatUpdatesActiveState(t *testing.T) {
	root := testStore(t)
	record := testRecord("machine-1")
	if err := UpsertMachine(root, record); err != nil {
		t.Fatalf("UpsertMachine() error = %v", err)
	}
	updated, err := UpdateHeartbeat(root, record.MachineID, "work", []string{"content-dev", "azure"}, "test-version", time.Date(2026, 5, 3, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("UpdateHeartbeat() error = %v", err)
	}
	if updated.ActiveProfile != "work" || len(updated.ActiveBuckets) != 2 || updated.LokiVersion != "test-version" {
		t.Fatalf("updated = %+v", updated)
	}
	stored, _, err := GetMachine(root, record.MachineID)
	if err != nil {
		t.Fatalf("GetMachine() error = %v", err)
	}
	if stored.LastSeen != "2026-05-03T04:00:00Z" || stored.ActiveProfile != "work" {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestDeleteMachineRemovesRegistryEntryAndHeartbeat(t *testing.T) {
	root := testStore(t)
	record := testRecord("machine-1")
	if err := UpsertMachine(root, record); err != nil {
		t.Fatalf("UpsertMachine() error = %v", err)
	}
	if err := DeleteMachine(root, record.MachineID); err != nil {
		t.Fatalf("DeleteMachine() error = %v", err)
	}
	_, ok, err := GetMachine(root, record.MachineID)
	if err != nil {
		t.Fatalf("GetMachine() error = %v", err)
	}
	if ok {
		t.Fatal("machine still present in registry")
	}
	if _, err := os.Stat(filepath.Join(root, "registry", "machines", record.MachineID+".json")); !os.IsNotExist(err) {
		t.Fatalf("heartbeat still exists or stat error: %v", err)
	}
}

func TestDeleteUnknownMachineFails(t *testing.T) {
	root := testStore(t)
	if err := DeleteMachine(root, "missing"); err == nil {
		t.Fatal("DeleteMachine() error = nil, want error")
	}
}

func TestMalformedRegistryFails(t *testing.T) {
	root := testStore(t)
	if err := os.WriteFile(filepath.Join(root, "registry", "machines.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := ReadRegistry(root); err == nil {
		t.Fatal("ReadRegistry() error = nil, want error")
	}
}

func testStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	result, err := store.EnsureLayout(root)
	if err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	if !result.Valid {
		t.Fatalf("store invalid: %+v", result)
	}
	return root
}

func testRecord(id string) Record {
	return Record{
		MachineID:             id,
		DisplayName:           "test machine",
		OS:                    "windows",
		Hostname:              "host",
		AllowedParentProfiles: []string{"work"},
		AllowedBuckets:        []string{"content-dev", "azure"},
		LastSeen:              "2026-05-03T03:00:00Z",
		ActiveProfile:         "work",
		ActiveBuckets:         []string{"content-dev"},
		LokiVersion:           "test",
	}
}
