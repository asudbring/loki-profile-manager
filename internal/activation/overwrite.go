package activation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ManagedTarget struct {
	TargetPath    string
	SourcePath    string
	Mode          string
	ContentHash   string
	LayerKind     string
	LayerName     string
	LastAppliedAt string
	MetadataJSON  string
}

func ValidateSafety(ctx context.Context, database *sql.DB, plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("validate safety: plan is nil")
	}
	var blocked []string
	for i := range plan.Operations {
		safety, err := ClassifyTarget(ctx, database, plan.Operations[i])
		if err != nil {
			return err
		}
		plan.Operations[i].Safety = safety
		if !safety.Safe {
			blocked = append(blocked, fmt.Sprintf("%s: %s", plan.Operations[i].TargetPath, safety.Message))
		}
	}
	if len(blocked) > 0 {
		return fmt.Errorf("unsafe target overwrite blocked: %s", strings.Join(blocked, "; "))
	}
	return nil
}

func ClassifyTarget(ctx context.Context, database *sql.DB, op Operation) (SafetyStatus, error) {
	record, found, err := GetManagedTarget(ctx, database, op.TargetPath)
	if err != nil {
		return SafetyStatus{}, err
	}
	info, err := os.Lstat(op.TargetPath)
	if errors.Is(err, os.ErrNotExist) {
		return SafetyStatus{Class: SafetyMissing, Safe: true, Message: "target is missing", Managed: found}, nil
	}
	if err != nil {
		return SafetyStatus{}, fmt.Errorf("classify target %s: %w", op.TargetPath, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(op.TargetPath)
		if err != nil {
			return SafetyStatus{}, fmt.Errorf("read symlink %s: %w", op.TargetPath, err)
		}
		if _, err := os.Stat(op.TargetPath); errors.Is(err, os.ErrNotExist) {
			return SafetyStatus{Class: SafetyBrokenSymlink, Safe: false, Message: fmt.Sprintf("broken symlink points to %s; repair or remove it before switching", linkTarget), Managed: found}, nil
		}
		if found && record.Mode == string(OperationSymlink) && samePath(linkTarget, record.SourcePath) {
			return SafetyStatus{Class: SafetyManagedSymlink, Safe: true, Message: "existing symlink is managed by Loki", Managed: true}, nil
		}
		if found && record.ContentHash != "" {
			hash, err := HashPath(op.TargetPath)
			if err != nil {
				return SafetyStatus{}, err
			}
			if hash == record.ContentHash {
				return SafetyStatus{Class: SafetyManagedFileHash, Safe: true, Message: "existing symlink hash matches Loki state", ExistingHash: hash, Managed: true}, nil
			}
			return SafetyStatus{Class: SafetyManagedHashMismatch, Safe: false, Message: "existing symlink hash differs from Loki state; capture or repair before switching", ExistingHash: hash, Managed: true}, nil
		}
		return SafetyStatus{Class: SafetyUnmanagedFile, Safe: false, Message: "existing symlink is not recorded as a Loki-managed symlink", Managed: found}, nil
	}

	if info.IsDir() {
		if found && record.ContentHash != "" {
			hash, err := HashPath(op.TargetPath)
			if err != nil {
				return SafetyStatus{}, err
			}
			if hash == record.ContentHash {
				return SafetyStatus{Class: SafetyManagedFileHash, Safe: true, Message: "existing directory hash matches Loki state", ExistingHash: hash, Managed: true}, nil
			}
			return SafetyStatus{Class: SafetyManagedHashMismatch, Safe: false, Message: "existing directory hash differs from Loki state; capture or repair before switching", ExistingHash: hash, Managed: true}, nil
		}
		return SafetyStatus{Class: SafetyUnmanagedDirectory, Safe: false, Message: "existing directory is not managed by Loki", Managed: found}, nil
	}

	if found && record.ContentHash != "" {
		hash, err := HashPath(op.TargetPath)
		if err != nil {
			return SafetyStatus{}, err
		}
		if hash == record.ContentHash {
			return SafetyStatus{Class: SafetyManagedFileHash, Safe: true, Message: "existing file hash matches Loki state", ExistingHash: hash, Managed: true}, nil
		}
		return SafetyStatus{Class: SafetyManagedHashMismatch, Safe: false, Message: "existing file hash differs from Loki state; capture or repair before switching", ExistingHash: hash, Managed: true}, nil
	}
	return SafetyStatus{Class: SafetyUnmanagedFile, Safe: false, Message: "existing file is not managed by Loki", Managed: found}, nil
}

func GetManagedTarget(ctx context.Context, database *sql.DB, targetPath string) (ManagedTarget, bool, error) {
	if database == nil {
		return ManagedTarget{}, false, fmt.Errorf("get managed target: database is nil")
	}
	var record ManagedTarget
	err := database.QueryRowContext(ctx, `SELECT target_path, COALESCE(source_path, ''), mode, COALESCE(content_hash, ''), COALESCE(layer_kind, ''), COALESCE(layer_name, ''), last_applied_at, COALESCE(metadata_json, '') FROM managed_targets WHERE target_path = ?`, targetPath).Scan(&record.TargetPath, &record.SourcePath, &record.Mode, &record.ContentHash, &record.LayerKind, &record.LayerName, &record.LastAppliedAt, &record.MetadataJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ManagedTarget{}, false, nil
	}
	if err != nil {
		return ManagedTarget{}, false, fmt.Errorf("get managed target %s: %w", targetPath, err)
	}
	return record, true, nil
}

func UpsertManagedTarget(ctx context.Context, database *sql.DB, op Operation, contentHash string, now time.Time) error {
	if database == nil {
		return fmt.Errorf("upsert managed target: database is nil")
	}
	metadata := map[string]any{
		"operation_id": op.ID,
		"sources":      op.Sources,
		"capture":      op.Capture,
		"safety":       op.Safety,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal managed target metadata: %w", err)
	}
	record := ManagedTarget{
		TargetPath:    op.TargetPath,
		SourcePath:    op.SourcePath,
		Mode:          string(op.Type),
		ContentHash:   contentHash,
		LayerKind:     op.LayerKind,
		LayerName:     op.LayerName,
		LastAppliedAt: now.UTC().Format(time.RFC3339),
		MetadataJSON:  string(metadataJSON),
	}
	if err := PutManagedTarget(ctx, database, record); err != nil {
		return fmt.Errorf("upsert managed target %s: %w", op.TargetPath, err)
	}
	return nil
}

func PutManagedTarget(ctx context.Context, database *sql.DB, record ManagedTarget) error {
	if database == nil {
		return fmt.Errorf("put managed target: database is nil")
	}
	_, err := database.ExecContext(ctx, `
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

func DeleteManagedTarget(ctx context.Context, database *sql.DB, targetPath string) error {
	if database == nil {
		return fmt.Errorf("delete managed target: database is nil")
	}
	if _, err := database.ExecContext(ctx, `DELETE FROM managed_targets WHERE target_path = ?`, targetPath); err != nil {
		return fmt.Errorf("delete managed target %s: %w", targetPath, err)
	}
	return nil
}

func samePath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr == nil && rightErr == nil {
		return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
