package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapFreshDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.sqlite")
	database, err := Bootstrap(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	defer database.Close()
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file missing: %v", err)
	}
	assertTablesExist(t, database, []string{"schema_migrations", "kv_state", "managed_targets", "snapshots", "pending_captures", "command_history"})
}

func TestBootstrapRecreateAfterDelete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.sqlite")
	database, err := Bootstrap(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	database.Close()
	if err := os.Remove(dbPath); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	database, err = Bootstrap(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Bootstrap() after delete error = %v", err)
	}
	database.Close()
}
