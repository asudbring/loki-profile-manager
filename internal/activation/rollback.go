package activation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
)

type RollbackRequest struct {
	Database *sql.DB
	Snapshot Snapshot
}

func Rollback(ctx context.Context, req RollbackRequest) error {
	for i := len(req.Snapshot.Targets) - 1; i >= 0; i-- {
		entry := req.Snapshot.Targets[i]
		if err := restoreSnapshotEntry(entry); err != nil {
			return err
		}
	}
	if req.Database != nil {
		if err := SetActiveState(ctx, req.Database, req.Snapshot.PreviousActiveProfile, req.Snapshot.PreviousActiveBuckets); err != nil {
			return err
		}
	}
	return nil
}

func restoreSnapshotEntry(entry SnapshotEntry) error {
	switch entry.Kind {
	case "missing":
		if err := removeExisting(entry.TargetPath); err != nil {
			return fmt.Errorf("rollback remove created target %s: %w", entry.TargetPath, err)
		}
		return nil
	case "symlink":
		if err := removeExisting(entry.TargetPath); err != nil {
			return fmt.Errorf("rollback replace symlink %s: %w", entry.TargetPath, err)
		}
		if err := os.Symlink(entry.LinkTarget, entry.TargetPath); err != nil {
			return fmt.Errorf("rollback restore symlink %s -> %s: %w", entry.TargetPath, entry.LinkTarget, err)
		}
		return nil
	case "file", "directory":
		if entry.SnapshotPath == "" {
			return fmt.Errorf("rollback %s: snapshot path is empty", entry.TargetPath)
		}
		if err := CopyPath(entry.SnapshotPath, entry.TargetPath); err != nil {
			return fmt.Errorf("rollback restore %s: %w", entry.TargetPath, err)
		}
		return nil
	default:
		return fmt.Errorf("rollback %s: unknown snapshot kind %q", entry.TargetPath, entry.Kind)
	}
}

func SetActiveState(ctx context.Context, database *sql.DB, profile string, buckets []string) error {
	if database == nil {
		return fmt.Errorf("set active state: database is nil")
	}
	encoded, err := json.Marshal(cloneStrings(buckets))
	if err != nil {
		return fmt.Errorf("marshal active buckets: %w", err)
	}
	if _, err := database.ExecContext(ctx, `
INSERT INTO kv_state (key, value, updated_at) VALUES ('active_profile', ?, datetime('now'))
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, profile); err != nil {
		return fmt.Errorf("set active profile: %w", err)
	}
	if _, err := database.ExecContext(ctx, `
INSERT INTO kv_state (key, value, updated_at) VALUES ('active_buckets', ?, datetime('now'))
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, string(encoded)); err != nil {
		return fmt.Errorf("set active buckets: %w", err)
	}
	return nil
}
