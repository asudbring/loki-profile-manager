package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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
	database.SetMaxOpenConns(1)
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

func OpenExistingReadOnly(ctx context.Context, dbPath string) (*sql.DB, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if dbPath == "" {
		return nil, false, fmt.Errorf("open sqlite read-only: database path is required")
	}
	info, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("stat sqlite database %s: %w", dbPath, err)
	}
	if info.IsDir() {
		return nil, false, fmt.Errorf("open sqlite read-only: %s is a directory", dbPath)
	}

	database, err := sql.Open(DriverName, sqliteReadOnlyDSN(dbPath))
	if err != nil {
		return nil, true, fmt.Errorf("open sqlite read-only: %w", err)
	}
	database.SetMaxOpenConns(1)
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return nil, true, fmt.Errorf("ping sqlite read-only: %w", err)
	}
	return database, true, nil
}

func sqliteReadOnlyDSN(dbPath string) string {
	path := strings.ReplaceAll(filepath.ToSlash(dbPath), `\`, "/")
	path = strings.ReplaceAll(url.PathEscape(path), "%2F", "/")
	return "file:" + path + "?mode=ro"
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
