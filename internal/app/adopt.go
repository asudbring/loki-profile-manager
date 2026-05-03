package app

import (
	"context"
	"fmt"

	"github.com/allensu/loki-profile-manager/internal/migration"
	"github.com/allensu/loki-profile-manager/internal/store"
)

type AdoptRequest struct {
	StorePath  string
	Target     string
	Profile    string
	Bucket     string
	Mode       string
	SourceName string
	DryRun     bool
	Yes        bool
}

type AdoptResult struct {
	Plan     migration.Plan `json:"plan"`
	DryRun   bool           `json:"dry_run"`
	Changed  int            `json:"changed"`
	Warnings []string       `json:"warnings,omitempty"`
}

func (s *Service) Adopt(ctx context.Context, req AdoptRequest) (AdoptResult, error) {
	if s == nil {
		return AdoptResult{}, fmt.Errorf("adopt: service is nil")
	}
	storePath, err := s.effectiveStorePath(ctx, req.StorePath)
	if err != nil {
		return AdoptResult{}, err
	}
	if validation := store.ValidateLayout(storePath); !validation.Valid {
		return AdoptResult{}, fmt.Errorf("adopt: invalid store layout: missing %v", validation.Missing)
	}
	plan, err := migration.BuildAdoptPlan(migration.AdoptRequest{BuildRequest: migration.BuildRequest{StorePath: storePath, Profile: req.Profile, Bucket: req.Bucket}, Resolver: s.resolver, Target: req.Target, Mode: req.Mode, SourceName: req.SourceName})
	if err != nil {
		return AdoptResult{}, err
	}
	execResult, err := migration.Execute(ctx, migration.ExecuteRequest{Database: s.database, Resolver: s.resolver, Plan: plan, DryRun: req.DryRun, Yes: req.Yes})
	result := AdoptResult{Plan: execResult.Plan, DryRun: req.DryRun, Changed: execResult.Changed, Warnings: execResult.Plan.Warnings}
	return result, err
}
