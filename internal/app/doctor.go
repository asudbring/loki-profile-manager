package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/db"
	diagnostics "github.com/asudbring/loki-profile-manager/internal/doctor"
)

type DoctorRequest struct {
	StorePath          string
	RepairManagedState bool
	WriteSafeFiles     bool
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
		Version:             Version,
		StorePath:           storePath,
		StoreOverride:       s.storeOverride,
		LocalPaths:          s.paths,
		Resolver:            s.resolver,
		Database:            s.database,
		SecretStatusChecker: s.secretStatusChecker,
		RepairManagedState:  req.RepairManagedState,
		WriteSafeFiles:      req.WriteSafeFiles,
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
	var database *sql.DB
	var exists bool
	var openErr error
	if opts.DoctorRepairManagedState {
		if _, err := os.Stat(paths.DBPath); err == nil {
			exists = true
			database, openErr = db.Bootstrap(ctx, paths.DBPath)
		} else if os.IsNotExist(err) {
			exists = false
		} else {
			exists = true
			openErr = err
		}
	} else {
		database, exists, openErr = db.OpenExistingReadOnly(ctx, paths.DBPath)
	}
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
		Version:             Version,
		StorePath:           storePath,
		StoreOverride:       storeOverride,
		LocalPaths:          paths,
		Resolver:            resolver,
		Database:            database,
		DatabaseMissing:     !exists && openErr == nil,
		DatabaseError:       databaseError,
		SecretStatusChecker: opts.SecretStatusChecker,
		RepairManagedState:  opts.DoctorRepairManagedState,
		WriteSafeFiles:      opts.DoctorWriteSafeFiles,
	}), nil
}
