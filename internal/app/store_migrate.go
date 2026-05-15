package app

import (
	"context"
	"fmt"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/db"
	"github.com/asudbring/loki-profile-manager/internal/store"
	"github.com/asudbring/loki-profile-manager/internal/storemigrate"
)

// StoreMigrateRequest controls a store-root copy and optional local rewire.
type StoreMigrateRequest struct {
	FromPath     string
	ToPath       string
	Provider     store.ProviderType
	DryRun       bool
	Yes          bool
	CopyOnly     bool
	CaptureLocal bool
}

// StoreMigrateResult summarizes a store migration dry-run or execution.
type StoreMigrateResult struct {
	OldStorePath            string                 `json:"old_store_path"`
	NewStorePath            string                 `json:"new_store_path"`
	Provider                store.ProviderType     `json:"provider,omitempty"`
	DryRun                  bool                   `json:"dry_run"`
	CopyOnly                bool                   `json:"copy_only"`
	Plan                    storemigrate.Plan      `json:"plan"`
	CopiedFiles             int                    `json:"copied_files"`
	CopiedDirs              int                    `json:"copied_dirs"`
	CopiedSymlinks          int                    `json:"copied_symlinks"`
	CopiedBytes             int64                  `json:"copied_bytes"`
	Captured                int                    `json:"captured"`
	CapturePlan             activation.CapturePlan `json:"capture_plan,omitempty"`
	CaptureRequired         bool                   `json:"capture_required,omitempty"`
	RebasedManagedTargets   int                    `json:"rebased_managed_targets"`
	Switched                bool                   `json:"switched"`
	DestinationStoreValid   bool                   `json:"destination_store_valid"`
	DestinationStoreMissing []string               `json:"destination_store_missing,omitempty"`
	Warnings                []string               `json:"warnings,omitempty"`
}

// StoreMigrate copies a valid Loki store to a missing/empty destination and can rewire local state.
func (s *Service) StoreMigrate(ctx context.Context, req StoreMigrateRequest) (StoreMigrateResult, error) {
	if s == nil {
		return StoreMigrateResult{}, fmt.Errorf("store migrate: service is nil")
	}
	if req.DryRun == req.Yes {
		return StoreMigrateResult{}, fmt.Errorf("store migrate: run exactly one of --dry-run or --yes")
	}
	toPath := s.resolver.CleanStoreOverride(req.ToPath)
	if toPath == "" {
		return StoreMigrateResult{}, fmt.Errorf("store migrate: --to is required")
	}
	fromPath := s.resolver.CleanStoreOverride(req.FromPath)
	if fromPath == "" {
		var err error
		fromPath, err = s.effectiveStorePath(ctx, "")
		if err != nil {
			return StoreMigrateResult{}, err
		}
	}
	result := StoreMigrateResult{
		OldStorePath: fromPath,
		NewStorePath: toPath,
		Provider:     req.Provider,
		DryRun:       req.DryRun,
		CopyOnly:     req.CopyOnly,
		Warnings:     []string{},
	}

	runMigration := func() error {
		plan, err := storemigrate.BuildPlan(storemigrate.PlanOptions{FromPath: fromPath, ToPath: toPath, Provider: req.Provider})
		result.Plan = plan
		if err != nil {
			return err
		}
		capturePlan, err := activation.BuildCapturePlan(ctx, s.database)
		if err != nil {
			return err
		}
		if capturePlan.HasChanges() {
			result.CapturePlan = capturePlan
			if capturePlan.HasBlocking() {
				return fmt.Errorf("store migrate: local changes cannot be captured automatically; resolve conflicts or unsupported modes before migrating")
			}
			if req.DryRun {
				result.CaptureRequired = !req.CaptureLocal
				return nil
			}
			if !req.CaptureLocal {
				result.CaptureRequired = true
				return fmt.Errorf("store migrate: local changes detected; rerun with --capture-local to write them back before migrating")
			}
			captured, err := activation.ApplyCaptures(ctx, s.database, capturePlan, time.Now())
			if err != nil {
				return err
			}
			result.Captured = captured
			plan, err = storemigrate.BuildPlan(storemigrate.PlanOptions{FromPath: fromPath, ToPath: toPath, Provider: req.Provider})
			result.Plan = plan
			if err != nil {
				return err
			}
		}
		if req.DryRun {
			return nil
		}
		copyResult, err := storemigrate.CopyPlan(plan)
		result.CopiedFiles = copyResult.CopiedFiles
		result.CopiedDirs = copyResult.CopiedDirs
		result.CopiedSymlinks = copyResult.CopiedSymlinks
		result.CopiedBytes = copyResult.CopiedBytes
		result.DestinationStoreValid = copyResult.Valid
		result.DestinationStoreMissing = copyResult.Missing
		if err != nil {
			return err
		}
		if req.CopyOnly {
			return nil
		}
		rebased, err := s.rebaseAndPersistStorePath(ctx, fromPath, toPath)
		if err != nil {
			return err
		}
		result.RebasedManagedTargets = rebased
		result.Switched = true
		return nil
	}
	if req.DryRun {
		return result, runMigration()
	}
	err := s.withStoreOperationLock(ctx, fromPath, "store migrate", false, func(machineID string) error {
		_ = machineID
		return runMigration()
	})
	return result, err
}

func (s *Service) rebaseAndPersistStorePath(ctx context.Context, oldRoot, newRoot string) (int, error) {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store migrate: begin local rewire transaction: %w", err)
	}
	defer tx.Rollback()
	rebased, err := activation.RebaseManagedTargetSourcePathsTx(ctx, tx, oldRoot, newRoot)
	if err != nil {
		return 0, err
	}
	if err := db.SetKVTx(ctx, tx, kvStorePath, newRoot); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store migrate: commit local rewire transaction: %w", err)
	}
	return rebased, nil
}
