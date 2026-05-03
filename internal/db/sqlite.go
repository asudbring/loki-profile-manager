package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const DriverName = "sqlite"

func Open(ctx context.Context, dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("open sqlite: database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	database, err := sql.Open(DriverName, dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := applyPragmas(ctx, database); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}

func Bootstrap(ctx context.Context, dbPath string) (*sql.DB, error) {
	database, err := Open(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	if err := Migrate(ctx, database); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}

func applyPragmas(ctx context.Context, database *sql.DB) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, pragma := range pragmas {
		if _, err := database.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("apply sqlite pragma %q: %w", pragma, err)
		}
	}
	return nil
}
