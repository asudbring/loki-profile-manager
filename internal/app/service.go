package app

import (
	"context"
	"database/sql"
	"encoding/json"
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
	Configured                   bool                  `json:"configured"`
	StorePath                    string                `json:"store_path"`
	StoreOverride                string                `json:"store_override"`
	LocalStatePath               string                `json:"local_state_path"`
	DatabasePath                 string                `json:"database_path"`
	Message                      string                `json:"message"`
	Version                      string                `json:"version"`
	Missing                      []string              `json:"missing,omitempty"`
	ActiveProfile                string                `json:"active_profile,omitempty"`
	ActiveBuckets                []string              `json:"active_buckets,omitempty"`
	ActiveSource                 string                `json:"active_source,omitempty"`
	ManagedTargetCount           int                   `json:"managed_target_count"`
	ManagedTargets               []StatusManagedTarget `json:"managed_targets,omitempty"`
	MachineID                    string                `json:"machine_id,omitempty"`
	MachineRegistered            bool                  `json:"machine_registered"`
	MachineDisplayName           string                `json:"machine_display_name,omitempty"`
	MachineAllowedParentProfiles []string              `json:"machine_allowed_parent_profiles,omitempty"`
	MachineAllowedBuckets        []string              `json:"machine_allowed_buckets,omitempty"`
	MachineActiveProfile         string                `json:"machine_active_profile,omitempty"`
	MachineActiveBuckets         []string              `json:"machine_active_buckets,omitempty"`
	MachineMessage               string                `json:"machine_message,omitempty"`
	MachineWarning               string                `json:"machine_warning,omitempty"`
}

type StatusManagedTarget struct {
	TargetPath    string `json:"target_path"`
	SourcePath    string `json:"source_path,omitempty"`
	Mode          string `json:"mode"`
	LayerKind     string `json:"layer_kind,omitempty"`
	LayerName     string `json:"layer_name,omitempty"`
	LastAppliedAt string `json:"last_applied_at,omitempty"`
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

type MachineStatusRequest struct {
	StorePath string
}

type MachineStatusResult struct {
	StorePath     string          `json:"store_path,omitempty"`
	MachineIDPath string          `json:"machine_id_path,omitempty"`
	MachineID     string          `json:"machine_id,omitempty"`
	Registered    bool            `json:"registered"`
	Record        *machine.Record `json:"record,omitempty"`
	Message       string          `json:"message,omitempty"`
	Warning       string          `json:"warning,omitempty"`
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
	if profile, buckets, ok, err := s.currentLocalActiveState(ctx); err != nil {
		return StatusResult{}, err
	} else if ok {
		status.ActiveProfile = profile
		status.ActiveBuckets = buckets
		status.ActiveSource = "local_state"
	}
	managedTargets, err := activation.ListManagedTargets(ctx, s.database)
	if err != nil {
		return StatusResult{}, err
	}
	status.ManagedTargetCount = len(managedTargets)
	status.ManagedTargets = statusManagedTargets(managedTargets)

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
		machineStatus, err := s.currentMachineStatus(storePath)
		status.MachineID = machineStatus.MachineID
		status.MachineRegistered = machineStatus.Registered
		status.MachineMessage = machineStatus.Message
		if err != nil {
			status.MachineWarning = err.Error()
		} else {
			status.MachineWarning = machineStatus.Warning
		}
		if machineStatus.Record != nil {
			status.MachineDisplayName = machineStatus.Record.DisplayName
			status.MachineAllowedParentProfiles = cloneStrings(machineStatus.Record.AllowedParentProfiles)
			status.MachineAllowedBuckets = cloneStrings(machineStatus.Record.AllowedBuckets)
			status.MachineActiveProfile = machineStatus.Record.ActiveProfile
			status.MachineActiveBuckets = cloneStrings(machineStatus.Record.ActiveBuckets)
			if status.ActiveProfile == "" && machineStatus.Record.ActiveProfile != "" {
				status.ActiveProfile = machineStatus.Record.ActiveProfile
				status.ActiveBuckets = cloneStrings(machineStatus.Record.ActiveBuckets)
				status.ActiveSource = "machine_registry"
			}
		}
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

func (s *Service) currentLocalActiveState(ctx context.Context) (string, []string, bool, error) {
	profile, ok, err := db.GetKV(ctx, s.database, kvActiveProfile)
	if err != nil {
		return "", nil, false, err
	}
	if !ok || profile == "" {
		return "", nil, false, nil
	}
	buckets := []string{}
	if raw, ok, err := db.GetKV(ctx, s.database, kvActiveBuckets); err != nil {
		return "", nil, false, err
	} else if ok && raw != "" {
		_ = json.Unmarshal([]byte(raw), &buckets)
	}
	return profile, buckets, true, nil
}

func statusManagedTargets(records []activation.ManagedTarget) []StatusManagedTarget {
	if len(records) == 0 {
		return nil
	}
	out := make([]StatusManagedTarget, 0, len(records))
	for _, record := range records {
		out = append(out, StatusManagedTarget{
			TargetPath:    record.TargetPath,
			SourcePath:    record.SourcePath,
			Mode:          record.Mode,
			LayerKind:     record.LayerKind,
			LayerName:     record.LayerName,
			LastAppliedAt: record.LastAppliedAt,
		})
	}
	return out
}

func (s *Service) MachineStatus(ctx context.Context, req MachineStatusRequest) (MachineStatusResult, error) {
	if s == nil {
		return MachineStatusResult{}, fmt.Errorf("machine status: service is nil")
	}
	storePath, err := s.effectiveStorePath(ctx, req.StorePath)
	if err != nil {
		return MachineStatusResult{}, err
	}
	if validation := store.ValidateLayout(storePath); !validation.Valid {
		return MachineStatusResult{}, fmt.Errorf("machine status: invalid store layout: missing %v", validation.Missing)
	}
	return s.currentMachineStatus(storePath)
}

func (s *Service) currentMachineStatus(storePath string) (MachineStatusResult, error) {
	result := MachineStatusResult{
		StorePath:     storePath,
		MachineIDPath: s.paths.MachineIDPath,
		Message:       "Machine ID has not been created. Run `loki machine register --allow-profile <profile>` to register this device.",
	}
	machineID, ok, err := machine.ReadID(s.paths.MachineIDPath)
	if err != nil {
		return result, fmt.Errorf("machine status: %w", err)
	}
	if !ok {
		return result, nil
	}
	result.MachineID = machineID
	result.Message = "Machine ID exists locally but is missing from synced registry."
	record, registered, err := machine.GetMachine(storePath, machineID)
	if err != nil {
		return result, fmt.Errorf("machine status: %w", err)
	}
	if !registered {
		result.Warning = fmt.Sprintf("machine %s is not registered; run `loki machine register --allow-profile <profile>` to add it to registry/machines.json", machineID)
		return result, nil
	}
	result.Registered = true
	result.Record = &record
	result.Message = "Machine is registered."
	return result, nil
}

func (s *Service) withStoreOperationLock(ctx context.Context, storePath, operation string, createMachineID bool, fn func(machineID string) error) error {
	var machineID string
	if createMachineID {
		id, err := s.EnsureMachineID(ctx)
		if err != nil {
			return err
		}
		machineID = id
	} else if id, ok, err := machine.ReadID(s.paths.MachineIDPath); err != nil {
		return err
	} else if ok {
		machineID = id
	}
	return store.WithOperationLock(ctx, storePath, store.OperationLockOptions{Operation: operation, MachineID: machineID}, func() error {
		return fn(machineID)
	})
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
