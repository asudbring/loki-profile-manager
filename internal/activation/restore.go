package activation

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/allensu/loki-profile-manager/internal/config"
)

type RestoreRequest struct {
	Database              *sql.DB
	LocalPaths            config.LocalPaths
	Snapshot              Snapshot
	MachineID             string
	PreviousActiveProfile string
	PreviousActiveBuckets []string
	Now                   func() time.Time
	Target                string
	ExpectedFingerprint   string
	FailAfter             int
}

type RestoreResult struct {
	SnapshotID         string            `json:"snapshot_id"`
	PreRestoreSnapshot Snapshot          `json:"pre_restore_snapshot"`
	Plan               RestoreDryRunPlan `json:"plan"`
	Changed            int               `json:"changed"`
}

func Restore(ctx context.Context, req RestoreRequest) (RestoreResult, error) {
	if req.LocalPaths.SnapshotDir == "" {
		return RestoreResult{}, fmt.Errorf("restore snapshot: snapshot directory is required")
	}
	plan, err := BuildRestoreDryRunPlanWithOptions(ctx, req.Snapshot, RestorePlanOptions{Target: req.Target})
	if err != nil {
		return RestoreResult{}, err
	}
	result := RestoreResult{SnapshotID: plan.Snapshot.SnapshotID, Plan: plan}
	if err := ValidateRestorePlan(plan); err != nil {
		return result, err
	}
	if req.ExpectedFingerprint != "" && plan.Fingerprint != req.ExpectedFingerprint {
		return result, fmt.Errorf("restore snapshot: current target state no longer matches dry-run guard")
	}
	now := time.Now
	if req.Now != nil {
		now = req.Now
	}
	backupPlan := Plan{Profile: "snapshot-restore", Operations: restoreBackupOperations(plan)}
	preRestore, err := CreateSnapshot(ctx, CreateSnapshotRequest{
		Database:              req.Database,
		SnapshotRoot:          req.LocalPaths.SnapshotDir,
		MachineID:             req.MachineID,
		Plan:                  backupPlan,
		Reason:                "pre_restore",
		SourceSnapshotID:      plan.Snapshot.SnapshotID,
		PreviousActiveProfile: req.PreviousActiveProfile,
		PreviousActiveBuckets: req.PreviousActiveBuckets,
		Now:                   now,
		SkipRetention:         true,
	})
	if err != nil {
		return result, err
	}
	result.PreRestoreSnapshot = preRestore

	rollbackSnapshot := preRestore
	rollbackSnapshot.Targets = []SnapshotEntry{}
	for i, target := range plan.Targets {
		if target.Action == RestoreActionSkipMissingTargetAbsent {
			continue
		}
		preRestoreIndex := snapshotTargetIndex(preRestore, target.Entry.TargetPath)
		if preRestoreIndex < 0 {
			return result, rollbackAfterRestoreFailure(ctx, req.Database, rollbackSnapshot, plan.Snapshot, fmt.Errorf("pre-restore snapshot missing target %s", target.Entry.TargetPath))
		}
		rollbackSnapshot.Targets = append(rollbackSnapshot.Targets, preRestore.Targets[preRestoreIndex])
		rollbackIndex := len(rollbackSnapshot.Targets) - 1
		if err := restoreSnapshotEntry(target.Entry); err != nil {
			return result, rollbackAfterRestoreFailure(ctx, req.Database, rollbackSnapshot, plan.Snapshot, err)
		}
		if err := verifyRestoredEntry(target.Entry); err != nil {
			return result, rollbackAfterRestoreFailure(ctx, req.Database, rollbackSnapshot, plan.Snapshot, err)
		}
		if err := markSnapshotTargetAfterOperation(&preRestore, Operation{TargetPath: target.Entry.TargetPath}); err != nil {
			result.PreRestoreSnapshot = preRestore
			return result, rollbackAfterRestoreFailure(ctx, req.Database, rollbackSnapshot, plan.Snapshot, err)
		}
		rollbackSnapshot.Targets[rollbackIndex] = preRestore.Targets[preRestoreIndex]
		if err := PersistSnapshot(ctx, req.Database, preRestore); err != nil {
			result.PreRestoreSnapshot = preRestore
			return result, rollbackAfterRestoreFailure(ctx, req.Database, rollbackSnapshot, plan.Snapshot, err)
		}
		result.PreRestoreSnapshot = preRestore
		result.Changed++
		if req.FailAfter > 0 && i+1 >= req.FailAfter {
			return result, rollbackAfterRestoreFailure(ctx, req.Database, rollbackSnapshot, plan.Snapshot, fmt.Errorf("simulated restore failure after %d target(s)", req.FailAfter))
		}
	}
	if req.Database != nil {
		var err error
		if plan.TargetFilter == "" {
			err = RestoreDatabaseState(ctx, req.Database, plan.Snapshot.ManagedTargets, plan.Snapshot.PreviousActiveProfile, plan.Snapshot.PreviousActiveBuckets)
		} else {
			err = RestoreManagedTargetsAtomic(ctx, req.Database, filterManagedTargetSnapshots(plan.Snapshot.ManagedTargets, plan.Targets))
		}
		if err != nil {
			return result, rollbackAfterRestoreFailure(ctx, req.Database, rollbackSnapshot, plan.Snapshot, err)
		}
	}
	return result, nil
}

func filterManagedTargetSnapshots(targets []ManagedTargetSnapshot, restoreTargets []RestoreDryRunTarget) []ManagedTargetSnapshot {
	selected := map[string]bool{}
	for _, target := range restoreTargets {
		selected[target.Entry.TargetPath] = true
	}
	out := []ManagedTargetSnapshot{}
	for _, target := range targets {
		if selected[target.TargetPath] {
			out = append(out, target)
		}
	}
	return out
}

func snapshotTargetIndex(snapshot Snapshot, targetPath string) int {
	for i, entry := range snapshot.Targets {
		if entry.TargetPath == targetPath {
			return i
		}
	}
	return -1
}

func restoreBackupOperations(plan RestoreDryRunPlan) []Operation {
	operations := make([]Operation, 0, len(plan.Targets))
	for i, target := range plan.Targets {
		operations = append(operations, Operation{
			ID:         fmt.Sprintf("pre-restore-%03d", i),
			Type:       OperationCopy,
			TargetPath: target.Entry.TargetPath,
		})
	}
	return operations
}

func verifyRestoredEntry(entry SnapshotEntry) error {
	switch entry.Kind {
	case "missing":
		if _, err := os.Lstat(entry.TargetPath); err == nil {
			return fmt.Errorf("verify restored missing target %s: target still exists", entry.TargetPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("verify restored missing target %s: %w", entry.TargetPath, err)
		}
		return nil
	case "file", "directory":
		if entry.Hash == "" {
			return fmt.Errorf("verify restored %s: snapshot hash missing", entry.TargetPath)
		}
		hash, err := HashPath(entry.TargetPath)
		if err != nil {
			return fmt.Errorf("verify restored %s: %w", entry.TargetPath, err)
		}
		if hash != entry.Hash {
			return fmt.Errorf("verify restored %s: hash mismatch", entry.TargetPath)
		}
		return nil
	case "symlink":
		link, err := os.Readlink(entry.TargetPath)
		if err != nil {
			return fmt.Errorf("verify restored symlink %s: %w", entry.TargetPath, err)
		}
		if link != entry.LinkTarget {
			return fmt.Errorf("verify restored symlink %s: link target mismatch", entry.TargetPath)
		}
		return nil
	default:
		return fmt.Errorf("verify restored %s: unknown snapshot kind %q", entry.TargetPath, entry.Kind)
	}
}

func rollbackAfterRestoreFailure(ctx context.Context, database *sql.DB, preRestore Snapshot, source Snapshot, cause error) error {
	if rollbackErr := Rollback(ctx, RollbackRequest{Database: database, Snapshot: preRestore}); rollbackErr != nil {
		return fmt.Errorf("snapshot restore %s failed: %w; rollback failed: %v; pre-restore snapshot preserved at %s for emergency recovery", source.SnapshotID, cause, rollbackErr, preRestore.Path)
	}
	return fmt.Errorf("snapshot restore %s failed and rollback completed: %w; pre-restore snapshot preserved at %s", source.SnapshotID, cause, preRestore.Path)
}
