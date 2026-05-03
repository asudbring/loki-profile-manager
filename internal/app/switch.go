package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/allensu/loki-profile-manager/internal/activation"
	"github.com/allensu/loki-profile-manager/internal/db"
	"github.com/allensu/loki-profile-manager/internal/infisical"
	"github.com/allensu/loki-profile-manager/internal/machine"
	"github.com/allensu/loki-profile-manager/internal/store"
)

const (
	kvActiveProfile = "active_profile"
	kvActiveBuckets = "active_buckets"
)

type SwitchRequest struct {
	StorePath     string
	ParentProfile string
	Buckets       []string
	DryRun        bool
	Yes           bool
	FailAfter     int
}

type SwitchResult struct {
	Plan       activation.Plan     `json:"plan"`
	SnapshotID string              `json:"snapshot_id,omitempty"`
	DryRun     bool                `json:"dry_run"`
	Changed    int                 `json:"changed"`
	Warnings   []string            `json:"warnings,omitempty"`
	Execution  activation.Snapshot `json:"snapshot,omitempty"`
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

	var warnings []string
	machineID, err := s.EnsureMachineID(ctx)
	if err != nil {
		return SwitchResult{}, err
	}
	record, registered, err := machine.GetMachine(storePath, machineID)
	if err != nil {
		return SwitchResult{}, err
	}
	if registered {
		if err := machine.ValidatePolicy(record, req.ParentProfile, req.Buckets); err != nil {
			return SwitchResult{}, err
		}
	} else {
		warnings = append(warnings, fmt.Sprintf("machine %s is not registered; policy and heartbeat update skipped", machineID))
	}

	plan, err := activation.BuildPlan(ctx, activation.PlanRequest{StorePath: storePath, Profile: req.ParentProfile, Buckets: req.Buckets, Resolver: s.resolver})
	if err != nil {
		return SwitchResult{}, err
	}
	previousProfile, previousBuckets := s.previousActiveState(ctx, record, registered)
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
	result := SwitchResult{Plan: execResult.Plan, DryRun: req.DryRun, Changed: execResult.Changed, Warnings: warnings, Execution: execResult.Snapshot}
	if execResult.Snapshot.SnapshotID != "" {
		result.SnapshotID = execResult.Snapshot.SnapshotID
	}
	if err != nil {
		return result, err
	}
	if !req.DryRun && registered {
		if _, err := s.WriteHeartbeat(ctx, HeartbeatRequest{StorePath: storePath, ActiveProfile: req.ParentProfile, ActiveBuckets: req.Buckets}); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("heartbeat update failed: %v", err))
		}
	}
	return result, nil
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
