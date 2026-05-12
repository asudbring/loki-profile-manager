package activation

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/config"
)

type ExecuteRequest struct {
	Database              *sql.DB
	LocalPaths            config.LocalPaths
	Plan                  Plan
	PreviousActiveProfile string
	PreviousActiveBuckets []string
	MachineID             string
	SecretProvider        SecretProvider
	Now                   func() time.Time
	DryRun                bool
	FailAfter             int
}

type ExecuteResult struct {
	Plan     Plan     `json:"plan"`
	Snapshot Snapshot `json:"snapshot,omitempty"`
	DryRun   bool     `json:"dry_run"`
	Changed  int      `json:"changed"`
}

func Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	plan := req.Plan
	result := ExecuteResult{Plan: plan, DryRun: req.DryRun}
	if err := ValidateSafety(ctx, req.Database, &plan); err != nil {
		result.Plan = plan
		return result, err
	}
	result.Plan = plan
	if req.DryRun {
		return result, nil
	}
	if req.LocalPaths.SnapshotDir == "" {
		return result, fmt.Errorf("execute activation: snapshot directory is required")
	}
	now := time.Now
	if req.Now != nil {
		now = req.Now
	}
	snapshot, err := CreateSnapshot(ctx, CreateSnapshotRequest{
		Database:              req.Database,
		SnapshotRoot:          req.LocalPaths.SnapshotDir,
		MachineID:             req.MachineID,
		Plan:                  plan,
		PreviousActiveProfile: req.PreviousActiveProfile,
		PreviousActiveBuckets: req.PreviousActiveBuckets,
		Now:                   now,
		Keep:                  2,
	})
	if err != nil {
		return result, err
	}
	result.Snapshot = snapshot

	for i, op := range plan.Operations {
		if err := executeOperation(ctx, op, req.SecretProvider); err != nil {
			return result, rollbackAfterFailure(ctx, req.Database, snapshot, err)
		}
		if err := markSnapshotTargetAfterOperation(&snapshot, op); err != nil {
			result.Snapshot = snapshot
			return result, rollbackAfterFailure(ctx, req.Database, snapshot, err)
		}
		if err := PersistSnapshot(ctx, req.Database, snapshot); err != nil {
			result.Snapshot = snapshot
			return result, rollbackAfterFailure(ctx, req.Database, snapshot, err)
		}
		result.Snapshot = snapshot
		result.Changed++
		if req.FailAfter > 0 && i+1 >= req.FailAfter {
			return result, rollbackAfterFailure(ctx, req.Database, snapshot, fmt.Errorf("simulated activation failure after %d operation(s)", req.FailAfter))
		}
	}
	for _, op := range plan.Operations {
		hash, err := HashPath(op.TargetPath)
		if err != nil {
			return result, rollbackAfterFailure(ctx, req.Database, snapshot, err)
		}
		if err := UpsertManagedTarget(ctx, req.Database, op, hash, now()); err != nil {
			return result, rollbackAfterFailure(ctx, req.Database, snapshot, err)
		}
	}
	if err := SetActiveState(ctx, req.Database, plan.Profile, plan.Buckets); err != nil {
		return result, rollbackAfterFailure(ctx, req.Database, snapshot, err)
	}
	if err := WriteActiveProfileMarker(req.LocalPaths, plan.Profile, plan.Buckets); err != nil {
		return result, rollbackAfterFailure(ctx, req.Database, snapshot, err)
	}
	result.Plan = plan
	return result, nil
}

func executeOperation(ctx context.Context, op Operation, provider SecretProvider) error {
	switch op.Type {
	case OperationSymlink:
		return ApplySymlink(op.SourcePath, op.TargetPath)
	case OperationCopy:
		return CopyPath(op.SourcePath, op.TargetPath)
	case OperationMerge:
		paths := make([]string, 0, len(op.Sources))
		for _, source := range op.Sources {
			paths = append(paths, source.Path)
		}
		return WriteMerge(op.Format, paths, op.TargetPath)
	case OperationRender:
		return RenderToFile(ctx, provider, op.SourcePath, op.TargetPath, op.Secrets)
	case OperationMirror:
		return nil
	default:
		return fmt.Errorf("unsupported activation operation %q for %s", op.Type, op.TargetPath)
	}
}

func markSnapshotTargetAfterOperation(snapshot *Snapshot, op Operation) error {
	if snapshot == nil {
		return nil
	}
	for i := range snapshot.Targets {
		if snapshot.Targets[i].TargetPath != op.TargetPath || snapshot.Targets[i].Kind != "missing" {
			continue
		}
		info, err := os.Lstat(op.TargetPath)
		if err != nil {
			return err
		}
		hash, err := HashPath(op.TargetPath)
		if err != nil {
			return err
		}
		snapshot.Targets[i].ExpectedHash = hash
		snapshot.Targets[i].ExpectedMode = info.Mode().String()
		return nil
	}
	return nil
}

func rollbackAfterFailure(ctx context.Context, database *sql.DB, snapshot Snapshot, cause error) error {
	if rollbackErr := Rollback(ctx, RollbackRequest{Database: database, Snapshot: snapshot}); rollbackErr != nil {
		return fmt.Errorf("activation failed: %w; rollback failed: %v; snapshot preserved at %s for emergency recovery", cause, rollbackErr, snapshot.Path)
	}
	return fmt.Errorf("activation failed and rollback completed: %w", cause)
}
