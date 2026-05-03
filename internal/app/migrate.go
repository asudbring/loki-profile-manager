package app

import (
	"context"
	"fmt"

	"github.com/allensu/loki-profile-manager/internal/migration"
	"github.com/allensu/loki-profile-manager/internal/store"
)

type MigrateRepoRequest struct {
	StorePath string
	RepoPath  string
	Profile   string
	Bucket    string
	DryRun    bool
	Yes       bool
}

type MigrateLocalRequest struct {
	StorePath string
	Profile   string
	Bucket    string
	DryRun    bool
	Yes       bool
}

type MigrateResult struct {
	Plan     migration.Plan `json:"plan"`
	DryRun   bool           `json:"dry_run"`
	Changed  int            `json:"changed"`
	Warnings []string       `json:"warnings,omitempty"`
}

func (s *Service) MigrateRepo(ctx context.Context, req MigrateRepoRequest) (MigrateResult, error) {
	if s == nil {
		return MigrateResult{}, fmt.Errorf("migrate repo: service is nil")
	}
	storePath, err := s.effectiveStorePath(ctx, req.StorePath)
	if err != nil {
		return MigrateResult{}, err
	}
	if validation := store.ValidateLayout(storePath); !validation.Valid {
		return MigrateResult{}, fmt.Errorf("migrate repo: invalid store layout: missing %v", validation.Missing)
	}
	plan, err := migration.BuildRepoPlan(migration.RepoRequest{BuildRequest: migration.BuildRequest{StorePath: storePath, Profile: req.Profile, Bucket: req.Bucket}, Resolver: s.resolver, RepoPath: req.RepoPath})
	if err != nil {
		return MigrateResult{}, err
	}
	return s.executeMigration(ctx, plan, req.DryRun, req.Yes)
}

func (s *Service) MigrateLocal(ctx context.Context, req MigrateLocalRequest) (MigrateResult, error) {
	if s == nil {
		return MigrateResult{}, fmt.Errorf("migrate local: service is nil")
	}
	storePath, err := s.effectiveStorePath(ctx, req.StorePath)
	if err != nil {
		return MigrateResult{}, err
	}
	if validation := store.ValidateLayout(storePath); !validation.Valid {
		return MigrateResult{}, fmt.Errorf("migrate local: invalid store layout: missing %v", validation.Missing)
	}
	plan, err := migration.BuildLocalPlan(migration.LocalRequest{BuildRequest: migration.BuildRequest{StorePath: storePath, Profile: req.Profile, Bucket: req.Bucket}, Resolver: s.resolver})
	if err != nil {
		return MigrateResult{}, err
	}
	return s.executeMigration(ctx, plan, req.DryRun, req.Yes)
}

func (s *Service) executeMigration(ctx context.Context, plan migration.Plan, dryRun, yes bool) (MigrateResult, error) {
	execResult, err := migration.Execute(ctx, migration.ExecuteRequest{Database: s.database, Resolver: s.resolver, Plan: plan, DryRun: dryRun, Yes: yes})
	result := MigrateResult{Plan: execResult.Plan, DryRun: dryRun, Changed: execResult.Changed, Warnings: execResult.Plan.Warnings}
	return result, err
}
