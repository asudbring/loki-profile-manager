package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/db"
	"github.com/asudbring/loki-profile-manager/internal/infisical"
	"github.com/asudbring/loki-profile-manager/internal/machine"
	"github.com/asudbring/loki-profile-manager/internal/secrets"
	"github.com/asudbring/loki-profile-manager/internal/store"
)

const (
	kvActiveProfile = "active_profile"
	kvActiveBuckets = "active_buckets"
)

type SwitchRequest struct {
	StorePath       string
	ParentProfile   string
	Buckets         []string
	DryRun          bool
	Yes             bool
	CaptureLocal    bool
	BackupUnmanaged bool
	FailAfter       int
}

// UnmanagedBackup records one pre-switch unmanaged target moved aside before activation.
// The backup is local machine state only; it is not written to the synced Loki store.
type UnmanagedBackup struct {
	TargetPath  string                 `json:"target_path"`
	BackupPath  string                 `json:"backup_path"`
	SafetyClass activation.SafetyClass `json:"safety_class"`
}

type SwitchResult struct {
	Plan                activation.Plan        `json:"plan"`
	SnapshotID          string                 `json:"snapshot_id,omitempty"`
	DryRun              bool                   `json:"dry_run"`
	Changed             int                    `json:"changed"`
	Captured            int                    `json:"captured"`
	CapturePlan         activation.CapturePlan `json:"capture_plan,omitempty"`
	CaptureRequired     bool                   `json:"capture_required,omitempty"`
	CleanupPlan         activation.CleanupPlan `json:"cleanup_plan,omitempty"`
	Cleaned             int                    `json:"cleaned,omitempty"`
	UnmanagedBackupRoot string                 `json:"unmanaged_backup_root,omitempty"`
	UnmanagedBackups    []UnmanagedBackup      `json:"unmanaged_backups,omitempty"`
	Warnings            []string               `json:"warnings,omitempty"`
	Execution           activation.Snapshot    `json:"snapshot,omitempty"`
}

func (s *Service) Switch(ctx context.Context, req SwitchRequest) (SwitchResult, error) {
	if s == nil {
		return SwitchResult{}, fmt.Errorf("switch: service is nil")
	}
	storePath, err := s.effectiveStorePath(ctx, req.StorePath)
	if err != nil {
		return SwitchResult{}, err
	}
	if validation := store.ValidateLayout(storePath); !validation.Valid {
		return SwitchResult{}, fmt.Errorf("switch: invalid store layout: missing %v", validation.Missing)
	}
	if req.BackupUnmanaged && !req.Yes {
		return SwitchResult{}, fmt.Errorf("switch: --backup-unmanaged requires --yes")
	}

	var result SwitchResult
	err = s.withStoreOperationLock(ctx, storePath, "switch", true, func(machineID string) error {
		var warnings []string
		record, registered, err := machine.GetMachine(storePath, machineID)
		if err != nil {
			return err
		}
		if !registered {
			return fmt.Errorf("switch: machine %s is not registered; run `%s` before switching", machineID, switchRegisterCommand(req.ParentProfile, req.Buckets))
		}
		if err := machine.ValidatePolicy(record, req.ParentProfile, req.Buckets); err != nil {
			return err
		}

		plan, err := activation.BuildPlan(ctx, activation.PlanRequest{StorePath: storePath, Profile: req.ParentProfile, Buckets: req.Buckets, Resolver: s.resolver})
		if err != nil {
			return err
		}
		previousProfile, previousBuckets := s.previousActiveState(ctx, record, registered)
		captureTargets := map[string]bool{}
		if previousProfile != "" {
			previousPlan, previousErr := activation.BuildPlan(ctx, activation.PlanRequest{StorePath: storePath, Profile: previousProfile, Buckets: previousBuckets, Resolver: s.resolver})
			if previousErr == nil {
				for _, op := range previousPlan.Operations {
					captureTargets[op.TargetPath] = true
				}
			}
		}
		capturePlan, err := activation.BuildCapturePlanForTargets(ctx, s.database, captureTargets)
		if err != nil {
			return err
		}
		if capturePlan.HasChanges() {
			result = SwitchResult{Plan: plan, DryRun: req.DryRun, CapturePlan: capturePlan, Warnings: warnings}
			if capturePlan.HasBlocking() {
				return fmt.Errorf("local changes cannot be captured automatically; resolve conflicts or unsupported modes before switching")
			}
			if req.DryRun {
				result.CaptureRequired = !req.CaptureLocal
				return nil
			}
			if !req.CaptureLocal {
				result.CaptureRequired = true
				return fmt.Errorf("local changes detected; rerun with --capture-local to write them back before switching")
			}
			captured, err := activation.ApplyCaptures(ctx, s.database, capturePlan, time.Now())
			if err != nil {
				return err
			}
			result.Captured = captured
			plan, err = activation.BuildPlan(ctx, activation.PlanRequest{StorePath: storePath, Profile: req.ParentProfile, Buckets: req.Buckets, Resolver: s.resolver})
			if err != nil {
				return err
			}
		}
		cleanupPlan, err := activation.BuildCleanupPlanForPlan(ctx, s.database, plan)
		if err != nil {
			return err
		}
		if cleanupPlan.HasBlocking() {
			result = SwitchResult{Plan: plan, DryRun: req.DryRun, Captured: result.Captured, CapturePlan: result.CapturePlan, CaptureRequired: result.CaptureRequired, CleanupPlan: cleanupPlan, Warnings: warnings}
			return fmt.Errorf("obsolete managed targets cannot be removed safely: %s", strings.Join(cleanupPlan.BlockingMessages(), "; "))
		}

		var backupRoot string
		var backups []UnmanagedBackup
		plan, backupRoot, backups, err = s.backupUnmanagedTargetsIfRequested(ctx, plan, req)
		if err != nil {
			result = SwitchResult{Plan: plan, DryRun: req.DryRun, Captured: result.Captured, CapturePlan: result.CapturePlan, CaptureRequired: result.CaptureRequired, CleanupPlan: cleanupPlan, UnmanagedBackupRoot: backupRoot, UnmanagedBackups: backups, Warnings: warnings}
			return err
		}

		execResult, err := activation.Execute(ctx, activation.ExecuteRequest{
			Database:              s.database,
			LocalPaths:            s.paths,
			Plan:                  plan,
			PreviousActiveProfile: previousProfile,
			PreviousActiveBuckets: previousBuckets,
			MachineID:             machineID,
			SecretProvider:        s.secretProvider,
			DryRun:                req.DryRun,
			FailAfter:             req.FailAfter,
		})
		captured := result.Captured
		captureRequired := result.CaptureRequired
		capturePlan = result.CapturePlan
		cleaned := 0
		if err == nil && !req.DryRun && cleanupPlan.HasChanges() {
			cleaned, err = activation.ApplyCleanup(ctx, s.database, cleanupPlan, activation.CleanupKeepTargets(execResult.Plan))
		}
		result = SwitchResult{Plan: execResult.Plan, DryRun: req.DryRun, Changed: execResult.Changed, Captured: captured, CapturePlan: capturePlan, CaptureRequired: captureRequired, CleanupPlan: cleanupPlan, Cleaned: cleaned, UnmanagedBackupRoot: backupRoot, UnmanagedBackups: backups, Warnings: warnings, Execution: execResult.Snapshot}
		if execResult.Snapshot.SnapshotID != "" {
			result.SnapshotID = execResult.Snapshot.SnapshotID
		}
		if err != nil {
			return err
		}
		if !req.DryRun && registered {
			if _, err := s.WriteHeartbeat(ctx, HeartbeatRequest{StorePath: storePath, ActiveProfile: req.ParentProfile, ActiveBuckets: req.Buckets}); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("heartbeat update failed: %v", err))
			}
		}
		return nil
	})
	return result, err
}

func switchRegisterCommand(parent string, buckets []string) string {
	var builder strings.Builder
	builder.WriteString("loki machine register")
	parent = strings.TrimSpace(parent)
	if parent != "" {
		builder.WriteString(" --allow-profile ")
		builder.WriteString(parent)
	}
	seen := map[string]bool{}
	for _, bucket := range buckets {
		bucket = strings.TrimSpace(bucket)
		if bucket == "" || seen[bucket] {
			continue
		}
		seen[bucket] = true
		builder.WriteString(" --allow-bucket ")
		builder.WriteString(bucket)
	}
	return builder.String()
}

func (s *Service) previousActiveState(ctx context.Context, record machine.Record, registered bool) (string, []string) {
	profile, ok, err := db.GetKV(ctx, s.database, kvActiveProfile)
	if err == nil && ok {
		if bucketsRaw, ok, err := db.GetKV(ctx, s.database, kvActiveBuckets); err == nil && ok {
			var buckets []string
			if json.Unmarshal([]byte(bucketsRaw), &buckets) == nil {
				return profile, buckets
			}
		}
		return profile, []string{}
	}
	if registered {
		return record.ActiveProfile, cloneStrings(record.ActiveBuckets)
	}
	return "", []string{}
}

func defaultSecretProvider() activation.SecretProvider {
	client := infisical.NewClient(nil)
	return client
}

func defaultSecretStatusChecker() secrets.StatusChecker {
	client := infisical.NewClient(nil)
	return client
}

func defaultSecretLoginRunner() secrets.LoginRunner {
	client := infisical.NewClient(nil)
	return client
}
