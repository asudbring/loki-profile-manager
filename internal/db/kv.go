package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func GetKV(ctx context.Context, database *sql.DB, key string) (string, bool, error) {
	if database == nil {
		return "", false, fmt.Errorf("get kv %q: database is nil", key)
	}
	var value string
	err := database.QueryRowContext(ctx, `SELECT value FROM kv_state WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get kv %q: %w", key, err)
	}
	return value, true, nil
}

func SetKV(ctx context.Context, database *sql.DB, key, value string) error {
	if database == nil {
		return fmt.Errorf("set kv %q: database is nil", key)
	}
	_, err := database.ExecContext(ctx, `
INSERT INTO kv_state (key, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key,
		value,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("set kv %q: %w", key, err)
	}
	return nil
}

func DeleteKV(ctx context.Context, database *sql.DB, key string) error {
	if database == nil {
		return fmt.Errorf("delete kv %q: database is nil", key)
	}
	if _, err := database.ExecContext(ctx, `DELETE FROM kv_state WHERE key = ?`, key); err != nil {
		return fmt.Errorf("delete kv %q: %w", key, err)
	}
	return nil
}
