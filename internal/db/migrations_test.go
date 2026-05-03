package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigrationsIdempotent(t *testing.T) {
	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	if err := Migrate(context.Background(), database); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := Migrate(context.Background(), database); err != nil {
		t.Fatalf("Migrate() second run error = %v", err)
	}

	var count int
	if err := database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM schema_migrations WHERE version = 1`).Scan(&count); err != nil {
		t.Fatalf("query schema_migrations error = %v", err)
	}
	if count != 1 {
		t.Fatalf("migration version count = %d, want 1", count)
	}
}

func assertTablesExist(t *testing.T, database *sql.DB, tables []string) {
	t.Helper()
	for _, table := range tables {
		var name string
		err := database.QueryRowContext(context.Background(), `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
	}
}
