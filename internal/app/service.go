package app

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/allensu/loki-profile-manager/internal/activation"
	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/db"
	lokilog "github.com/allensu/loki-profile-manager/internal/log"
	"github.com/allensu/loki-profile-manager/internal/machine"
	"github.com/allensu/loki-profile-manager/internal/store"
)

const Version = "dev"

const kvStorePath = "store_path"

type Options struct {
	Resolver       config.PathResolver
	StoreOverride  string
	Verbose        bool
	Stderr         io.Writer
	SecretProvider activation.SecretProvider
}

type Service struct {
	resolver       config.PathResolver
	paths          config.LocalPaths
	storeOverride  string
	logger         *lokilog.Logger
	database       *sql.DB
	secretProvider activation.SecretProvider
}

type StatusRequest struct{}

type StatusResult struct {
	Configured     bool     `json:"configured"`
	StorePath      string   `json:"store_path"`
	StoreOverride  string   `json:"store_override"`
	LocalStatePath string   `json:"local_state_path"`
	DatabasePath   string   `json:"database_path"`
	Message        string   `json:"message"`
	Version        string   `json:"version"`
	Missing        []string `json:"missing,omitempty"`
}

type DiscoverStoresRequest struct {
	ManualPath string
}

type DiscoverStoresResult struct {
	Candidates []store.ProviderCandidate `json:"candidates"`
}

type EnsureStoreRequest struct {
	StorePath string
}

type EnsureStoreResult struct {
	StorePath string   `json:"store_path"`
	Created   bool     `json:"created"`
	Valid     bool     `json:"valid"`
	Missing   []string `json:"missing"`
}

type RegisterMachineRequest struct {
	StorePath             string
	DisplayName           string
	AllowedParentProfiles []string
	AllowedBuckets        []string
	ActiveProfile         string
	ActiveBuckets         []string
}

type HeartbeatRequest struct {
	StorePath     string
	ActiveProfile string
	ActiveBuckets []string
}

type ValidatePolicyRequest struct {
	StorePath     string
	MachineID     string
	ParentProfile string
	Buckets       []string
}

func NewService(ctx context.Context, opts Options) (*Service, error) {
	resolver := opts.Resolver
	if resolver.GOOS == "" && resolver.HomeDir == "" && resolver.LocalAppData == "" && resolver.Env == nil {
		resolver = config.NewPathResolverFromEnv()
	}
	resolver = resolver.WithDefaults()

	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		return nil, err
	}

	logger, err := lokilog.NewLogger(paths, lokilog.Options{
		Verbose:        opts.Verbose,
		TerminalWriter: opts.Stderr,
		Redactor:       lokilog.NewRedactor(),
	})
	if err != nil {
		return nil, fmt.Errorf("initialize logger: %w", err)
	}

	database, err := db.Bootstrap(ctx, paths.DBPath)
	if err != nil {
		logger.Close()
		return nil, fmt.Errorf("initialize sqlite: %w", err)
	}

	storeOverride := resolver.CleanStoreOverride(opts.StoreOverride)
	secretProvider := opts.SecretProvider
	if secretProvider == nil {
		secretProvider = defaultSecretProvider()
	}
	return &Service{
		resolver:       resolver,
		paths:          paths,
		storeOverride:  storeOverride,
		logger:         logger,
		database:       database,
		secretProvider: secretProvider,
	}, nil
}

func (s *Service) Close() error {
	var firstErr error
	if s == nil {
		return nil
	}
	if s.database != nil {
		if err := s.database.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.logger != nil {
		if err := s.logger.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Service) Status(ctx context.Context, req StatusRequest) (StatusResult, error) {
	if s == nil {
		return StatusResult{}, fmt.Errorf("status: service is nil")
	}
	s.logger.Slog().InfoContext(ctx, "status requested", "store_override", s.storeOverride)

	status := StatusResult{
		Configured:     false,
		StoreOverride:  s.storeOverride,
		LocalStatePath: s.paths.StateDir,
		DatabasePath:   s.paths.DBPath,
		Message:        "Loki is not configured. Setup is not implemented in Phase 1.",
		Version:        Version,
	}
	storePath, ok, err := s.configuredStorePath(ctx)
	if err != nil {
		return StatusResult{}, err
	}
	if !ok {
		return status, nil
	}
	status.StorePath = storePath
	validation := store.ValidateLayout(storePath)
	status.Configured = validation.Valid
	status.Missing = validation.Missing
	if validation.Valid {
		status.Message = "Loki store is configured."
	} else {
		status.Message = "Loki store path is configured but layout is invalid."
	}
	return status, nil
}

func (s *Service) DiscoverStores(ctx context.Context, req DiscoverStoresRequest) (DiscoverStoresResult, error) {
	if s == nil {
		return DiscoverStoresResult{}, fmt.Errorf("discover stores: service is nil")
	}
	manualPath := req.ManualPath
	if manualPath == "" {
		manualPath = s.storeOverride
	}
	candidates := store.DiscoverProviderFolders(store.DiscoveryOptions{
		GOOS:       s.resolver.GOOS,
		HomeDir:    s.resolver.HomeDir,
		ManualPath: manualPath,
		Env:        s.resolver.Env,
	})
	return DiscoverStoresResult{Candidates: candidates}, nil
}

func (s *Service) EnsureStore(ctx context.Context, req EnsureStoreRequest) (EnsureStoreResult, error) {
	storePath := s.resolver.CleanStoreOverride(req.StorePath)
	if storePath == "" {
		storePath = s.storeOverride
	}
	if storePath == "" {
		return EnsureStoreResult{}, fmt.Errorf("ensure store: store path is required")
	}
	result, err := store.EnsureLayout(storePath)
	if err != nil {
		return EnsureStoreResult{}, err
	}
	if result.Valid {
		if err := db.SetKV(ctx, s.database, kvStorePath, storePath); err != nil {
			return EnsureStoreResult{}, err
		}
	}
	return EnsureStoreResult{StorePath: storePath, Created: result.Created, Valid: result.Valid, Missing: result.Missing}, nil
}

func (s *Service) EnsureMachineID(ctx context.Context) (string, error) {
	if s == nil {
		return "", fmt.Errorf("ensure machine id: service is nil")
	}
	return machine.EnsureID(s.paths.MachineIDPath)
}

func (s *Service) RegisterMachine(ctx context.Context, req RegisterMachineRequest) (machine.Record, error) {
	storePath, err := s.effectiveStorePath(ctx, req.StorePath)
	if err != nil {
		return machine.Record{}, err
	}
	if validation := store.ValidateLayout(storePath); !validation.Valid {
		return machine.Record{}, fmt.Errorf("register machine: invalid store layout: missing %v", validation.Missing)
	}
	machineID, err := s.EnsureMachineID(ctx)
	if err != nil {
		return machine.Record{}, err
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	displayName := req.DisplayName
	if displayName == "" {
		displayName = hostname
	}
	goos := s.resolver.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	record := machine.NewRecord(machineID, displayName, goos, hostname, req.AllowedParentProfiles, req.AllowedBuckets, Version, time.Now())
	record.ActiveProfile = req.ActiveProfile
	record.ActiveBuckets = cloneStrings(req.ActiveBuckets)
	if err := machine.UpsertMachine(storePath, record); err != nil {
		return machine.Record{}, err
	}
	return record, nil
}

func (s *Service) WriteHeartbeat(ctx context.Context, req HeartbeatRequest) (machine.Record, error) {
	storePath, err := s.effectiveStorePath(ctx, req.StorePath)
	if err != nil {
		return machine.Record{}, err
	}
	machineID, err := s.EnsureMachineID(ctx)
	if err != nil {
		return machine.Record{}, err
	}
	return machine.UpdateHeartbeat(storePath, machineID, req.ActiveProfile, req.ActiveBuckets, Version, time.Now())
}

func (s *Service) ValidateMachinePolicy(ctx context.Context, req ValidatePolicyRequest) error {
	storePath, err := s.effectiveStorePath(ctx, req.StorePath)
	if err != nil {
		return err
	}
	machineID := req.MachineID
	if machineID == "" {
		machineID, err = s.EnsureMachineID(ctx)
		if err != nil {
			return err
		}
	}
	record, ok, err := machine.GetMachine(storePath, machineID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("validate machine policy: machine %s not found in registry", machineID)
	}
	return machine.ValidatePolicy(record, req.ParentProfile, req.Buckets)
}

func (s *Service) DeleteMachine(ctx context.Context, storePath, machineID string) error {
	effective, err := s.effectiveStorePath(ctx, storePath)
	if err != nil {
		return err
	}
	if machineID == "" {
		return fmt.Errorf("delete machine: machine id is required")
	}
	return machine.DeleteMachine(effective, machineID)
}

func (s *Service) effectiveStorePath(ctx context.Context, explicit string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("resolve store path: service is nil")
	}
	if path := s.resolver.CleanStoreOverride(explicit); path != "" {
		return path, nil
	}
	if s.storeOverride != "" {
		return s.storeOverride, nil
	}
	if value, ok, err := db.GetKV(ctx, s.database, kvStorePath); err != nil {
		return "", err
	} else if ok && value != "" {
		return s.resolver.CleanStoreOverride(value), nil
	}
	return "", fmt.Errorf("store is not configured; provide --store or initialize a Loki store")
}

func (s *Service) configuredStorePath(ctx context.Context) (string, bool, error) {
	if s.storeOverride != "" {
		return s.storeOverride, true, nil
	}
	value, ok, err := db.GetKV(ctx, s.database, kvStorePath)
	if err != nil {
		return "", false, err
	}
	if !ok || value == "" {
		return "", false, nil
	}
	return s.resolver.CleanStoreOverride(value), true, nil
}

func cloneStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
