package app

import (
	"context"
	"fmt"

	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/db"
	diagnostics "github.com/allensu/loki-profile-manager/internal/doctor"
)

type DoctorRequest struct {
	StorePath string
}

type DoctorResult = diagnostics.Report

func (s *Service) Doctor(ctx context.Context, req DoctorRequest) (DoctorResult, error) {
	if s == nil {
		return DoctorResult{}, fmt.Errorf("doctor: service is nil")
	}
	storePath := s.resolver.CleanStoreOverride(req.StorePath)
	if storePath == "" {
		var ok bool
		var err error
		storePath, ok, err = s.configuredStorePath(ctx)
		if err != nil {
			return DoctorResult{}, err
		}
		if !ok {
			storePath = ""
		}
	}
	return diagnostics.Run(ctx, diagnostics.Request{
		Version:       Version,
		StorePath:     storePath,
		StoreOverride: s.storeOverride,
		LocalPaths:    s.paths,
		Resolver:      s.resolver,
		Database:      s.database,
	}), nil
}

func RunDoctor(ctx context.Context, opts Options) (DoctorResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	resolver := opts.Resolver
	if resolver.GOOS == "" && resolver.HomeDir == "" && resolver.LocalAppData == "" && resolver.Env == nil {
		resolver = config.NewPathResolverFromEnv()
	}
	resolver = resolver.WithDefaults()

	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		return DoctorResult{}, err
	}
	storeOverride := resolver.CleanStoreOverride(opts.StoreOverride)
	database, exists, openErr := db.OpenExistingReadOnly(ctx, paths.DBPath)
	if database != nil {
		defer database.Close()
	}

	storePath := storeOverride
	if storePath == "" && database != nil {
		if value, ok, err := db.GetKV(ctx, database, kvStorePath); err == nil && ok && value != "" {
			storePath = resolver.CleanStoreOverride(value)
		}
	}

	databaseError := ""
	if openErr != nil {
		databaseError = openErr.Error()
	}
	return diagnostics.Run(ctx, diagnostics.Request{
		Version:         Version,
		StorePath:       storePath,
		StoreOverride:   storeOverride,
		LocalPaths:      paths,
		Resolver:        resolver,
		Database:        database,
		DatabaseMissing: !exists && openErr == nil,
		DatabaseError:   databaseError,
	}), nil
}
