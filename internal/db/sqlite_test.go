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

func TestOpenAppliesConnectionPragmas(t *testing.T) {
	database, err := Bootstrap(context.Background(), filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	defer database.Close()
	if got := database.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}
	var foreignKeys int
	if err := database.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("PRAGMA foreign_keys error = %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
	var busyTimeout int
	if err := database.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout error = %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busyTimeout)
	}
}

func TestOpenExistingReadOnlyMissingDoesNotCreate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.sqlite")
	database, exists, err := OpenExistingReadOnly(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenExistingReadOnly() error = %v", err)
	}
	if database != nil || exists {
		t.Fatalf("database=%v exists=%v, want nil false", database, exists)
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("database was created or stat failed: %v", err)
	}
}

func TestOpenExistingReadOnlyRejectsWrites(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.sqlite")
	writable, err := Bootstrap(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	writable.Close()

	database, exists, err := OpenExistingReadOnly(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenExistingReadOnly() error = %v", err)
	}
	defer database.Close()
	if !exists {
		t.Fatal("exists = false, want true")
	}
	if _, err := database.Exec(`INSERT INTO kv_state (key, value, updated_at) VALUES ('x', 'y', 'now')`); err == nil {
		t.Fatal("read-only database write error = nil")
	}
}

func TestOpenExistingReadOnlyHandlesSpacesInPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "dir with spaces", "state.sqlite")
	writable, err := Bootstrap(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	writable.Close()

	database, exists, err := OpenExistingReadOnly(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenExistingReadOnly() error = %v", err)
	}
	defer database.Close()
	if !exists {
		t.Fatal("exists = false, want true")
	}
	assertTablesExist(t, database, []string{"schema_migrations", "kv_state"})
}

func TestSQLiteReadOnlyDSNNormalizesWindowsPath(t *testing.T) {
	got := sqliteReadOnlyDSN(`C:\Users\alice\AppData\Local\loki-profile-manager\state.sqlite`)
	want := `file:C:/Users/alice/AppData/Local/loki-profile-manager/state.sqlite?mode=ro`
	if got != want {
		t.Fatalf("sqliteReadOnlyDSN() = %q, want %q", got, want)
	}
}
