package activation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
		if err := RestoreDatabaseState(ctx, req.Database, req.Snapshot.ManagedTargets, req.Snapshot.PreviousActiveProfile, req.Snapshot.PreviousActiveBuckets); err != nil {
			return err
		}
	}
	return nil
}

func restoreSnapshotEntry(entry SnapshotEntry) error {
	switch entry.Kind {
	case "missing":
		info, err := os.Lstat(entry.TargetPath)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return fmt.Errorf("rollback inspect created target %s: %w", entry.TargetPath, err)
		}
		if entry.ExpectedHash == "" {
			return fmt.Errorf("rollback conflict for %s: target was missing before activation and no expected hash was recorded; leaving it in place", entry.TargetPath)
		}
		if entry.ExpectedMode != "" && info.Mode().String() != entry.ExpectedMode {
			return fmt.Errorf("rollback conflict for %s: current mode differs from Loki-created mode; leaving it in place", entry.TargetPath)
		}
		hash, err := HashPath(entry.TargetPath)
		if err != nil {
			return fmt.Errorf("rollback hash created target %s: %w", entry.TargetPath, err)
		}
		if hash != entry.ExpectedHash {
			return fmt.Errorf("rollback conflict for %s: current hash differs from Loki-created hash; leaving it in place", entry.TargetPath)
		}
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

func RestoreManagedTargets(ctx context.Context, database *sql.DB, targets []ManagedTargetSnapshot) error {
	for _, target := range targets {
		if target.Found {
			if err := PutManagedTarget(ctx, database, target.Record); err != nil {
				return err
			}
			continue
		}
		if err := DeleteManagedTarget(ctx, database, target.TargetPath); err != nil {
			return err
		}
	}
	return nil
}

func RestoreManagedTargetsAtomic(ctx context.Context, database *sql.DB, targets []ManagedTargetSnapshot) error {
	if database == nil {
		return fmt.Errorf("restore managed targets: database is nil")
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin restore managed targets: %w", err)
	}
	defer tx.Rollback()
	if err := restoreManagedTargetsTx(ctx, tx, targets); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit restore managed targets: %w", err)
	}
	return nil
}

func RestoreDatabaseState(ctx context.Context, database *sql.DB, targets []ManagedTargetSnapshot, profile string, buckets []string) error {
	if database == nil {
		return fmt.Errorf("restore database state: database is nil")
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin restore database state: %w", err)
	}
	defer tx.Rollback()
	if err := restoreManagedTargetsTx(ctx, tx, targets); err != nil {
		return err
	}
	encoded, err := json.Marshal(cloneStrings(buckets))
	if err != nil {
		return fmt.Errorf("marshal active buckets: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO kv_state (key, value, updated_at) VALUES ('active_profile', ?, datetime('now'))
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, profile); err != nil {
		return fmt.Errorf("set active profile: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO kv_state (key, value, updated_at) VALUES ('active_buckets', ?, datetime('now'))
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, string(encoded)); err != nil {
		return fmt.Errorf("set active buckets: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit restore database state: %w", err)
	}
	return nil
}

func restoreManagedTargetsTx(ctx context.Context, tx *sql.Tx, targets []ManagedTargetSnapshot) error {
	for _, target := range targets {
		if target.Found {
			if err := putManagedTargetTx(ctx, tx, target.Record); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM managed_targets WHERE target_path = ?`, target.TargetPath); err != nil {
			return fmt.Errorf("delete managed target %s: %w", target.TargetPath, err)
		}
	}
	return nil
}

func putManagedTargetTx(ctx context.Context, tx *sql.Tx, record ManagedTarget) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO managed_targets (target_path, source_path, mode, content_hash, layer_kind, layer_name, last_applied_at, metadata_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(target_path) DO UPDATE SET
    source_path = excluded.source_path,
    mode = excluded.mode,
    content_hash = excluded.content_hash,
    layer_kind = excluded.layer_kind,
    layer_name = excluded.layer_name,
    last_applied_at = excluded.last_applied_at,
    metadata_json = excluded.metadata_json`,
		record.TargetPath,
		record.SourcePath,
		record.Mode,
		record.ContentHash,
		record.LayerKind,
		record.LayerName,
		record.LastAppliedAt,
		record.MetadataJSON,
	)
	if err != nil {
		return fmt.Errorf("put managed target %s: %w", record.TargetPath, err)
	}
	return nil
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
