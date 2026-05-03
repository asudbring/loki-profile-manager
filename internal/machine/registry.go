package machine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/allensu/loki-profile-manager/internal/store"
)

type Registry struct {
	Version  int      `json:"version"`
	Machines []Record `json:"machines"`
}

type Record struct {
	MachineID             string   `json:"machine_id"`
	DisplayName           string   `json:"display_name"`
	OS                    string   `json:"os"`
	Hostname              string   `json:"hostname"`
	AllowedParentProfiles []string `json:"allowed_parent_profiles"`
	AllowedBuckets        []string `json:"allowed_buckets"`
	LastSeen              string   `json:"last_seen"`
	ActiveProfile         string   `json:"active_profile"`
	ActiveBuckets         []string `json:"active_buckets"`
	LokiVersion           string   `json:"loki_version"`
}

func NewRecord(machineID, displayName, goos, hostname string, allowedProfiles, allowedBuckets []string, version string, now time.Time) Record {
	return Record{
		MachineID:             machineID,
		DisplayName:           displayName,
		OS:                    goos,
		Hostname:              hostname,
		AllowedParentProfiles: cloneStrings(allowedProfiles),
		AllowedBuckets:        cloneStrings(allowedBuckets),
		LastSeen:              now.UTC().Format(time.RFC3339),
		ActiveBuckets:         []string{},
		LokiVersion:           version,
	}
}

func ReadRegistry(storeRoot string) (Registry, error) {
	path := registryPath(storeRoot)
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Registry{Version: store.RegistryVersion, Machines: []Record{}}, nil
	}
	if err != nil {
		return Registry{}, fmt.Errorf("read machine registry %s: %w", path, err)
	}
	var registry Registry
	if err := json.Unmarshal(content, &registry); err != nil {
		return Registry{}, fmt.Errorf("parse machine registry %s: %w", path, err)
	}
	if registry.Version == 0 {
		registry.Version = store.RegistryVersion
	}
	if registry.Version != store.RegistryVersion {
		return Registry{}, fmt.Errorf("machine registry %s has unsupported version %d", path, registry.Version)
	}
	if registry.Machines == nil {
		registry.Machines = []Record{}
	}
	return registry, nil
}

func WriteRegistry(storeRoot string, registry Registry) error {
	if registry.Version == 0 {
		registry.Version = store.RegistryVersion
	}
	if registry.Machines == nil {
		registry.Machines = []Record{}
	}
	content, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal machine registry: %w", err)
	}
	return atomicWriteFile(registryPath(storeRoot), append(content, '\n'), 0o644)
}

func UpsertMachine(storeRoot string, record Record) error {
	if record.MachineID == "" {
		return fmt.Errorf("upsert machine: machine_id is required")
	}
	registry, err := ReadRegistry(storeRoot)
	if err != nil {
		return err
	}
	updated := false
	for i := range registry.Machines {
		if registry.Machines[i].MachineID == record.MachineID {
			registry.Machines[i] = record
			updated = true
			break
		}
	}
	if !updated {
		registry.Machines = append(registry.Machines, record)
	}
	if err := WriteRegistry(storeRoot, registry); err != nil {
		return err
	}
	return WriteHeartbeat(storeRoot, record)
}

func GetMachine(storeRoot, machineID string) (Record, bool, error) {
	registry, err := ReadRegistry(storeRoot)
	if err != nil {
		return Record{}, false, err
	}
	for _, record := range registry.Machines {
		if record.MachineID == machineID {
			return record, true, nil
		}
	}
	return Record{}, false, nil
}

func DeleteMachine(storeRoot, machineID string) error {
	registry, err := ReadRegistry(storeRoot)
	if err != nil {
		return err
	}
	machines := make([]Record, 0, len(registry.Machines))
	found := false
	for _, record := range registry.Machines {
		if record.MachineID == machineID {
			found = true
			continue
		}
		machines = append(machines, record)
	}
	if !found {
		return fmt.Errorf("delete machine %s: machine not found in registry", machineID)
	}
	registry.Machines = machines
	if err := WriteRegistry(storeRoot, registry); err != nil {
		return err
	}
	if err := os.Remove(heartbeatPath(storeRoot, machineID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete machine heartbeat: %w", err)
	}
	return nil
}

func registryPath(storeRoot string) string {
	return filepath.Join(storeRoot, "registry", "machines.json")
}

func cloneStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func atomicWriteFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file for %s: %w", path, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
