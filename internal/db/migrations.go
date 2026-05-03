package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Migration struct {
	Version int
	Name    string
	SQL     string
}

var migrations = []Migration{
	{
		Version: 1,
		Name:    "initial local state schema",
		SQL: strings.TrimSpace(`
CREATE TABLE IF NOT EXISTS kv_state (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS managed_targets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_path TEXT NOT NULL UNIQUE,
    source_path TEXT,
    mode TEXT NOT NULL,
    content_hash TEXT,
    layer_kind TEXT,
    layer_name TEXT,
    last_applied_at TEXT NOT NULL,
    metadata_json TEXT
);

CREATE TABLE IF NOT EXISTS snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id TEXT NOT NULL UNIQUE,
    machine_id TEXT,
    path TEXT NOT NULL,
    previous_active_profile TEXT,
    previous_active_buckets_json TEXT,
    created_at TEXT NOT NULL,
    metadata_json TEXT
);

CREATE TABLE IF NOT EXISTS pending_captures (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_path TEXT NOT NULL,
    detected_hash TEXT,
    reason TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    metadata_json TEXT
);

CREATE TABLE IF NOT EXISTS command_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    command TEXT NOT NULL,
    args_json TEXT,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    exit_code INTEGER,
    status TEXT NOT NULL,
    error_summary TEXT
);
`),
	},
}

func Migrate(ctx context.Context, database *sql.DB) error {
	if database == nil {
		return fmt.Errorf("migrate sqlite: database is nil")
	}
	if _, err := database.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TEXT NOT NULL
);`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	applied, err := appliedMigrations(ctx, database)
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if applied[migration.Version] {
			continue
		}
		if err := applyMigration(ctx, database, migration); err != nil {
			return err
		}
	}
	return nil
}

func appliedMigrations(ctx context.Context, database *sql.DB) (map[int]bool, error) {
	rows, err := database.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := map[int]bool{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema_migrations: %w", err)
	}
	return applied, nil
}

func applyMigration(ctx context.Context, database *sql.DB, migration Migration) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", migration.Version, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
		return fmt.Errorf("apply migration %d %q: %w", migration.Version, migration.Name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		migration.Version,
		migration.Name,
		time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("record migration %d: %w", migration.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", migration.Version, err)
	}
	return nil
}
