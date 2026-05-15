package app

import (
	"context"
	"fmt"
	"io/fs"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/db"
	"github.com/asudbring/loki-profile-manager/internal/store"
	"github.com/asudbring/loki-profile-manager/internal/storemigrate"
)

// StoreMigrateRequest controls a store-root copy and optional local rewire.
type StoreMigrateRequest struct {
	FromPath         string
	ToPath           string
	Provider         store.ProviderType
	DryRun           bool
	Yes              bool
	CopyOnly         bool
	CaptureLocal     bool
	Hydrate          bool
	Cleanup          bool
	FileTimeout      time.Duration
	ProgressInterval time.Duration
	Reporter         storemigrate.Reporter
	Dataless         func(path string, info fs.FileInfo) bool
	Detector         storemigrate.CloudPlaceholderDetector
}

// StoreMigrateResult summarizes a store migration dry-run or execution.
type StoreMigrateResult struct {
	OldStorePath            string                 `json:"old_store_path"`
	NewStorePath            string                 `json:"new_store_path"`
	Provider                store.ProviderType     `json:"provider,omitempty"`
	DryRun                  bool                   `json:"dry_run"`
	CopyOnly                bool                   `json:"copy_only"`
	Plan                    storemigrate.Plan      `json:"plan"`
	StagingPath             string                 `json:"staging_path,omitempty"`
	HydratedFiles           int                    `json:"hydrated_files,omitempty"`
	HydratedBytes           int64                  `json:"hydrated_bytes,omitempty"`
	CopiedFiles             int                    `json:"copied_files"`
	CopiedDirs              int                    `json:"copied_dirs"`
	CopiedSymlinks          int                    `json:"copied_symlinks"`
	CopiedBytes             int64                  `json:"copied_bytes"`
	Captured                int                    `json:"captured"`
	CapturePlan             activation.CapturePlan `json:"capture_plan,omitempty"`
	CaptureRequired         bool                   `json:"capture_required,omitempty"`
	RebasedManagedTargets   int                    `json:"rebased_managed_targets"`
	RetargetedSymlinks      int                    `json:"retargeted_symlinks"`
	Switched                bool                   `json:"switched"`
	DestinationStoreValid   bool                   `json:"destination_store_valid"`
	DestinationStoreMissing []string               `json:"destination_store_missing,omitempty"`
	CleanedStaging          []string               `json:"cleaned_staging,omitempty"`
	Warnings                []string               `json:"warnings,omitempty"`
}

// StoreMigrate copies a valid Loki store to a missing/empty destination and can rewire local state.
func (s *Service) StoreMigrate(ctx context.Context, req StoreMigrateRequest) (StoreMigrateResult, error) {
	if s == nil {
		return StoreMigrateResult{}, fmt.Errorf("store migrate: service is nil")
	}
	toPath := s.resolver.CleanStoreOverride(req.ToPath)
	if toPath == "" {
		return StoreMigrateResult{}, fmt.Errorf("store migrate: --to is required")
	}
	if !req.Cleanup && req.DryRun == req.Yes {
		return StoreMigrateResult{}, fmt.Errorf("store migrate: run exactly one of --dry-run or --yes")
	}
	fromPath := s.resolver.CleanStoreOverride(req.FromPath)
	if fromPath == "" && !req.Cleanup {
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
	if req.Cleanup {
		storemigrateReport(ctx, req.Reporter, storemigrate.Event{Phase: storemigrate.PhaseCleanup, Message: "cleaning interrupted staging directories"})
		removed, err := storemigrate.CleanupStaging(toPath)
		result.CleanedStaging = removed
		return result, err
	}
	if req.FileTimeout == 0 {
		req.FileTimeout = 2 * time.Minute
	}
	reporter := req.Reporter
	if req.ProgressInterval > 0 && reporter != nil {
		reporter = storemigrate.NewThrottledReporter(reporter, req.ProgressInterval, time.Now)
	}

	buildPlan := func() (storemigrate.Plan, error) {
		return storemigrate.BuildPlan(storemigrate.PlanOptions{
			FromPath:      fromPath,
			ToPath:        toPath,
			Provider:      req.Provider,
			AllowDataless: req.Hydrate,
			Dataless:      req.Dataless,
			Detector:      req.Detector,
		})
	}

	runMigration := func() error {
		storemigrateReport(ctx, reporter, storemigrate.Event{Phase: storemigrate.PhasePreflight, Message: "building migration plan"})
		plan, err := buildPlan()
		result.Plan = plan
		if plan.Summary.DatalessCount > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%d cloud-only files require hydration before copy", plan.Summary.DatalessCount))
		}
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
			plan, err = buildPlan()
			result.Plan = plan
			if err != nil {
				return err
			}
		}
		if req.DryRun {
			return nil
		}
		if plan.Summary.DatalessCount > 0 {
			if !req.Hydrate {
				return fmt.Errorf("store migrate: source contains %d cloud-only files; rerun with --hydrate to materialize before copying", plan.Summary.DatalessCount)
			}
			hydrateResult, err := storemigrate.HydratePlan(ctx, storemigrate.HydrateOptions{Plan: plan, Reporter: reporter, FileTimeout: req.FileTimeout})
			result.HydratedFiles = hydrateResult.HydratedFiles
			result.HydratedBytes = hydrateResult.HydratedBytes
			if err != nil {
				return err
			}
		}
		staging := newStoreMigrateStaging(toPath, time.Now())
		result.StagingPath = staging.Path
		if err := storemigrate.PrepareStaging(staging); err != nil {
			return err
		}
		promoted := false
		defer func() {
			if !promoted {
				_ = storemigrate.CleanupStagingPath(staging)
			}
		}()
		stagingPlan := storemigrate.PlanWithDestination(plan, staging.Path)
		copyResult, err := storemigrate.CopyPlanWithOptions(ctx, storemigrate.CopyOptions{Plan: stagingPlan, Reporter: reporter, FileTimeout: req.FileTimeout})
		result.CopiedFiles = copyResult.CopiedFiles
		result.CopiedDirs = copyResult.CopiedDirs
		result.CopiedSymlinks = copyResult.CopiedSymlinks
		result.CopiedBytes = copyResult.CopiedBytes
		result.DestinationStoreValid = copyResult.Valid
		result.DestinationStoreMissing = copyResult.Missing
		if err != nil {
			return err
		}
		storemigrateReport(ctx, reporter, storemigrate.Event{Phase: storemigrate.PhasePromote, Message: "promoting staged store"})
		if err := storemigrate.PromoteStaging(staging); err != nil {
			return err
		}
		promoted = true
		result.StagingPath = ""
		if req.CopyOnly {
			storemigrateReport(ctx, reporter, storemigrate.Event{Phase: storemigrate.PhaseDone, Message: "store copied"})
			return nil
		}
		storemigrateReport(ctx, reporter, storemigrate.Event{Phase: storemigrate.PhaseRewire, Message: "rewiring local store metadata"})
		rebased, err := s.rebaseAndPersistStorePath(ctx, fromPath, toPath)
		if err != nil {
			return err
		}
		result.RebasedManagedTargets = rebased
		result.Switched = true
		storemigrateReport(ctx, reporter, storemigrate.Event{Phase: storemigrate.PhaseRetarget, Message: "retargeting active managed symlinks"})
		retargeted, err := retargetManagedSymlinks(ctx, s.database, fromPath, toPath)
		if err != nil {
			result.Warnings = append(result.Warnings, "local store path is already switched; rerun migration cleanup or repair active symlinks manually")
			return fmt.Errorf("store migrate: local store path switched, but managed symlink retarget failed: %w", err)
		}
		result.RetargetedSymlinks = retargeted
		storemigrateReport(ctx, reporter, storemigrate.Event{Phase: storemigrate.PhaseDone, Message: "store migrated"})
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

var retargetManagedSymlinks = activation.RetargetManagedSymlinks
var newStoreMigrateStaging = storemigrate.NewStaging

func storemigrateReport(ctx context.Context, reporter storemigrate.Reporter, event storemigrate.Event) {
	if reporter != nil {
		reporter.Report(ctx, event)
	}
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
