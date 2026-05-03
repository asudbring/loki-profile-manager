package machine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func WriteHeartbeat(storeRoot string, record Record) error {
	if record.MachineID == "" {
		return fmt.Errorf("write heartbeat: machine_id is required")
	}
	content, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal heartbeat: %w", err)
	}
	return atomicWriteFile(heartbeatPath(storeRoot, record.MachineID), append(content, '\n'), 0o644)
}

func ReadHeartbeat(storeRoot, machineID string) (Record, error) {
	path := heartbeatPath(storeRoot, machineID)
	content, err := os.ReadFile(path)
	if err != nil {
		return Record{}, fmt.Errorf("read heartbeat %s: %w", path, err)
	}
	var record Record
	if err := json.Unmarshal(content, &record); err != nil {
		return Record{}, fmt.Errorf("parse heartbeat %s: %w", path, err)
	}
	return record, nil
}

func UpdateHeartbeat(storeRoot, machineID, activeProfile string, activeBuckets []string, version string, now time.Time) (Record, error) {
	record, ok, err := GetMachine(storeRoot, machineID)
	if err != nil {
		return Record{}, err
	}
	if !ok {
		return Record{}, fmt.Errorf("update heartbeat for machine %s: machine not found in registry", machineID)
	}
	record.LastSeen = now.UTC().Format(time.RFC3339)
	record.ActiveProfile = activeProfile
	record.ActiveBuckets = cloneStrings(activeBuckets)
	if version != "" {
		record.LokiVersion = version
	}
	if err := UpsertMachine(storeRoot, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func heartbeatPath(storeRoot, machineID string) string {
	return filepath.Join(storeRoot, "registry", "machines", machineID+".json")
}
