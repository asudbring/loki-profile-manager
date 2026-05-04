package doctor

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/db"
	"github.com/allensu/loki-profile-manager/internal/machine"
	"github.com/allensu/loki-profile-manager/internal/store"
)

func TestRunDetectsStaleLockStaleMachineAndConflictCopy(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	database := openDoctorTestDB(t, ctx, paths.DBPath)
	defer database.Close()

	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}

	machineID := "11111111-1111-4111-8111-111111111111"
	if err := os.MkdirAll(filepath.Dir(paths.MachineIDPath), 0o700); err != nil {
		t.Fatalf("mkdir machine id: %v", err)
	}
	if err := os.WriteFile(paths.MachineIDPath, []byte(machineID+"\n"), 0o600); err != nil {
		t.Fatalf("write machine id: %v", err)
	}

	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	old := now.Add(-31 * 24 * time.Hour)
	record := machine.NewRecord(machineID, "old mac", "darwin", "old-host", []string{"work"}, nil, "dev", old)
	if err := machine.UpsertMachine(storePath, record); err != nil {
		t.Fatalf("UpsertMachine() error = %v", err)
	}

	lockInfo := store.OperationLockInfo{Version: 1, PID: 123, Operation: "switch", AcquiredAt: old.Format(time.RFC3339Nano), ExpiresAt: old.Add(30 * time.Minute).Format(time.RFC3339Nano), Token: "test-token"}
	lockContent, err := json.Marshal(lockInfo)
	if err != nil {
		t.Fatalf("marshal lock: %v", err)
	}
	if err := os.WriteFile(store.OperationLockPath(storePath), lockContent, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	conflictPath := filepath.Join(storePath, "profiles", "work", "core", "files", "settings conflicted copy.txt")
	if err := os.WriteFile(conflictPath, []byte("conflict"), 0o600); err != nil {
		t.Fatalf("write conflict file: %v", err)
	}

	report := Run(ctx, Request{Version: "test", StorePath: storePath, LocalPaths: paths, Resolver: resolver, Database: database, Now: func() time.Time { return now }})
	for _, code := range []string{"lock.operation_stale", "machine.heartbeat_stale", "sync.conflict_copy_found"} {
		if !hasCheck(report, code) {
			t.Fatalf("report missing %s: %+v", code, report.Checks)
		}
	}
}

func openDoctorTestDB(t *testing.T, ctx context.Context, path string) *sql.DB {
	t.Helper()
	database, err := db.Bootstrap(ctx, path)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	return database
}

func hasCheck(report Report, code string) bool {
	for _, check := range report.Checks {
		if check.Code == code {
			return true
		}
	}
	return false
}
