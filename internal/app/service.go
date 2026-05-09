package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/db"
	lokilog "github.com/asudbring/loki-profile-manager/internal/log"
	"github.com/asudbring/loki-profile-manager/internal/machine"
	"github.com/asudbring/loki-profile-manager/internal/secrets"
	"github.com/asudbring/loki-profile-manager/internal/store"
)

var Version = "dev"

const kvStorePath = "store_path"

const (
	restoreGuardPrefix = "snapshot_restore_guard:"
	restoreGuardTTL    = 15 * time.Minute
)

type restoreGuard struct {
	Version      int    `json:"version"`
	SnapshotID   string `json:"snapshot_id"`
	TargetFilter string `json:"target_filter,omitempty"`
	Fingerprint  string `json:"fingerprint"`
	TargetCount  int    `json:"target_count"`
	CreatedAt    string `json:"created_at"`
	ExpiresAt    string `json:"expires_at"`
}

type Options struct {
	Resolver            config.PathResolver
	StoreOverride       string
	Verbose             bool
	Stderr              io.Writer
	SecretProvider      activation.SecretProvider
	SecretStatusChecker secrets.StatusChecker
	SecretLoginRunner   secrets.LoginRunner
}

type Service struct {
	resolver            config.PathResolver
	paths               config.LocalPaths
	storeOverride       string
	logger              *lokilog.Logger
	database            *sql.DB
	secretProvider      activation.SecretProvider
	secretStatusChecker secrets.StatusChecker
	secretLoginRunner   secrets.LoginRunner
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

type SnapshotListRequest struct{}

type SnapshotListResult struct {
	SnapshotDir string                       `json:"snapshot_dir"`
	Snapshots   []activation.SnapshotSummary `json:"snapshots"`
}

type SnapshotShowRequest struct {
	SnapshotID string
}

type SnapshotShowResult struct {
	SnapshotDir string              `json:"snapshot_dir"`
	Snapshot    activation.Snapshot `json:"snapshot"`
}

type SnapshotRestoreRequest struct {
	SnapshotID string
	DryRun     bool
	Yes        bool
	Target     string
}

type SnapshotRestoreDryRunRequest struct {
	SnapshotID string
	DryRun     bool
	Target     string
}

type SnapshotRestoreResult struct {
	SnapshotDir          string                        `json:"snapshot_dir"`
	SnapshotID           string                        `json:"snapshot_id"`
	TargetFilter         string                        `json:"target_filter,omitempty"`
	TargetFilterRedacted bool                          `json:"target_filter_redacted,omitempty"`
	DryRun               bool                          `json:"dry_run"`
	WouldWrite           bool                          `json:"would_write"`
	Restored             bool                          `json:"restored"`
	Changed              int                           `json:"changed"`
	PreRestoreSnapshotID string                        `json:"pre_restore_snapshot_id,omitempty"`
	GuardRecorded        bool                          `json:"guard_recorded,omitempty"`
	GuardExpiresAt       string                        `json:"guard_expires_at,omitempty"`
	Summary              SnapshotRestoreDryRunSummary  `json:"summary"`
	Targets              []SnapshotRestoreDryRunTarget `json:"targets"`
	Warnings             []string                      `json:"warnings,omitempty"`
	Blockers             []string                      `json:"blockers,omitempty"`
}

type SnapshotRestoreDryRunResult = SnapshotRestoreResult

type SnapshotRestoreDryRunSummary struct {
	TargetCount                   int      `json:"target_count"`
	RestoreFileCount              int      `json:"restore_file_count"`
	RestoreDirectoryCount         int      `json:"restore_directory_count"`
	RestoreSymlinkCount           int      `json:"restore_symlink_count"`
	RemoveCreatedTargetCount      int      `json:"remove_created_target_count"`
	SkipMissingTargetAbsentCount  int      `json:"skip_missing_target_already_absent_count"`
	UnknownCount                  int      `json:"unknown_count"`
	PreviousActiveProfile         string   `json:"previous_active_profile,omitempty"`
	PreviousActiveBuckets         []string `json:"previous_active_buckets"`
	WouldRestoreManagedTargetRows int      `json:"would_restore_managed_target_rows"`
	WouldRestoreActiveState       bool     `json:"would_restore_active_state"`
}

type SnapshotRestoreDryRunTarget struct {
	TargetPath         string   `json:"target_path,omitempty"`
	TargetPathRedacted bool     `json:"target_path_redacted,omitempty"`
	Kind               string   `json:"kind"`
	Action             string   `json:"action"`
	CurrentExists      bool     `json:"current_exists"`
	CurrentKind        string   `json:"current_kind,omitempty"`
	CurrentMode        string   `json:"current_mode,omitempty"`
	CurrentHashPrefix  string   `json:"current_hash_prefix,omitempty"`
	SnapshotHashPrefix string   `json:"snapshot_hash_prefix,omitempty"`
	ExpectedHashPrefix string   `json:"expected_hash_prefix,omitempty"`
	ExpectedMode       string   `json:"expected_mode,omitempty"`
	LinkTarget         string   `json:"link_target,omitempty"`
	LinkTargetRedacted bool     `json:"link_target_redacted,omitempty"`
	SensitivePath      bool     `json:"sensitive_path"`
	Warnings           []string `json:"warnings,omitempty"`
}

type StoreStatusRequest struct{}

type StoreStatusResult struct {
	StoreOverride      string   `json:"store_override,omitempty"`
	PersistedStorePath string   `json:"persisted_store_path,omitempty"`
	EffectiveStorePath string   `json:"effective_store_path,omitempty"`
	EffectiveSource    string   `json:"effective_source"`
	LocalStatePath     string   `json:"local_state_path"`
	DatabasePath       string   `json:"database_path"`
	Valid              bool     `json:"valid"`
	Missing            []string `json:"missing,omitempty"`
	Message            string   `json:"message"`
}

type DiscoverStoresRequest struct {
	ManualPath string
}

type StoreCandidate struct {
	Provider       store.ProviderType `json:"provider"`
	ProviderPath   string             `json:"provider_path"`
	StorePath      string             `json:"store_path"`
	Source         string             `json:"source"`
	ProviderExists bool               `json:"provider_exists"`
	StoreExists    bool               `json:"store_exists"`
	StoreEmpty     bool               `json:"store_empty"`
	StoreIsDir     bool               `json:"store_is_dir"`
	StoreValid     bool               `json:"store_valid"`
	Missing        []string           `json:"missing,omitempty"`
}

type DiscoverStoresResult struct {
	Candidates []StoreCandidate `json:"candidates"`
}

type UseStoreRequest struct {
	StorePath string
}

type ForgetStoreRequest struct{}

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
	secretStatusChecker := opts.SecretStatusChecker
	if secretStatusChecker == nil {
		secretStatusChecker = defaultSecretStatusChecker()
	}
	secretLoginRunner := opts.SecretLoginRunner
	if secretLoginRunner == nil {
		secretLoginRunner = defaultSecretLoginRunner()
	}
	return &Service{
		resolver:            resolver,
		paths:               paths,
		storeOverride:       storeOverride,
		logger:              logger,
		database:            database,
		secretProvider:      secretProvider,
		secretStatusChecker: secretStatusChecker,
		secretLoginRunner:   secretLoginRunner,
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

func (s *Service) ListSnapshots(ctx context.Context, req SnapshotListRequest) (SnapshotListResult, error) {
	if s == nil {
		return SnapshotListResult{}, fmt.Errorf("list snapshots: service is nil")
	}
	snapshots, err := activation.ListSnapshots(ctx, s.database, s.paths.SnapshotDir)
	if err != nil {
		return SnapshotListResult{}, err
	}
	return SnapshotListResult{SnapshotDir: s.paths.SnapshotDir, Snapshots: snapshots}, nil
}

func (s *Service) ShowSnapshot(ctx context.Context, req SnapshotShowRequest) (SnapshotShowResult, error) {
	if s == nil {
		return SnapshotShowResult{}, fmt.Errorf("show snapshot: service is nil")
	}
	snapshot, err := activation.LoadSnapshot(ctx, s.database, s.paths.SnapshotDir, req.SnapshotID)
	if err != nil {
		return SnapshotShowResult{}, err
	}
	return SnapshotShowResult{SnapshotDir: s.paths.SnapshotDir, Snapshot: snapshot}, nil
}

func (s *Service) RestoreSnapshotDryRun(ctx context.Context, req SnapshotRestoreDryRunRequest) (SnapshotRestoreDryRunResult, error) {
	return s.RestoreSnapshot(ctx, SnapshotRestoreRequest{SnapshotID: req.SnapshotID, DryRun: req.DryRun, Target: req.Target})
}

func (s *Service) RestoreSnapshot(ctx context.Context, req SnapshotRestoreRequest) (SnapshotRestoreResult, error) {
	if s == nil {
		return SnapshotRestoreResult{}, fmt.Errorf("snapshots restore: service is nil")
	}
	if req.DryRun == req.Yes {
		return SnapshotRestoreResult{}, fmt.Errorf("snapshots restore: run exactly one of --dry-run or --yes")
	}
	if req.DryRun {
		snapshot, err := activation.LoadSnapshot(ctx, s.database, s.paths.SnapshotDir, req.SnapshotID)
		if err != nil {
			return SnapshotRestoreResult{}, err
		}
		plan, err := activation.BuildRestoreDryRunPlanWithOptions(ctx, snapshot, activation.RestorePlanOptions{Target: req.Target})
		if err != nil {
			return SnapshotRestoreResult{}, err
		}
		result := snapshotRestoreResult(s.paths.SnapshotDir, plan)
		if plan.CanRestore {
			expiresAt, err := s.recordRestoreGuard(ctx, plan, time.Now())
			if err != nil {
				return SnapshotRestoreResult{}, err
			}
			result.GuardRecorded = true
			result.GuardExpiresAt = expiresAt.Format(time.RFC3339Nano)
		}
		return result, nil
	}

	var result SnapshotRestoreResult
	if err := s.withLocalOperationLock(ctx, "snapshots restore", func(machineID string) error {
		snapshot, err := activation.LoadSnapshot(ctx, s.database, s.paths.SnapshotDir, req.SnapshotID)
		if err != nil {
			return err
		}
		plan, err := activation.BuildRestoreDryRunPlanWithOptions(ctx, snapshot, activation.RestorePlanOptions{Target: req.Target})
		if err != nil {
			return err
		}
		result = snapshotRestoreResult(s.paths.SnapshotDir, plan)
		result.DryRun = false
		if err := activation.ValidateRestorePlan(plan); err != nil {
			return err
		}
		if err := s.requireRestoreGuard(ctx, plan, time.Now()); err != nil {
			return err
		}
		if err := s.clearRestoreGuard(ctx, plan.Snapshot.SnapshotID, plan.TargetFilter); err != nil {
			return err
		}
		profile, buckets, _, err := s.currentLocalActiveState(ctx)
		if err != nil {
			return err
		}
		restoreResult, err := activation.Restore(ctx, activation.RestoreRequest{
			Database:              s.database,
			LocalPaths:            s.paths,
			Snapshot:              snapshot,
			MachineID:             machineID,
			PreviousActiveProfile: profile,
			PreviousActiveBuckets: buckets,
			Target:                req.Target,
			ExpectedFingerprint:   plan.Fingerprint,
		})
		if err != nil {
			return err
		}
		result.Restored = true
		result.WouldWrite = true
		result.Changed = restoreResult.Changed
		result.PreRestoreSnapshotID = restoreResult.PreRestoreSnapshot.SnapshotID
		return nil
	}); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) StoreStatus(ctx context.Context, req StoreStatusRequest) (StoreStatusResult, error) {
	if s == nil {
		return StoreStatusResult{}, fmt.Errorf("store status: service is nil")
	}
	_ = req
	persisted, ok, err := db.GetKV(ctx, s.database, kvStorePath)
	if err != nil {
		return StoreStatusResult{}, err
	}
	if !ok {
		persisted = ""
	}
	persisted = s.resolver.CleanStoreOverride(persisted)
	result := StoreStatusResult{
		StoreOverride:      s.storeOverride,
		PersistedStorePath: persisted,
		LocalStatePath:     s.paths.StateDir,
		DatabasePath:       s.paths.DBPath,
		EffectiveSource:    "none",
		Message:            "Loki store is not configured.",
	}
	if s.storeOverride != "" {
		result.EffectiveStorePath = s.storeOverride
		result.EffectiveSource = "override"
	} else if persisted != "" {
		result.EffectiveStorePath = persisted
		result.EffectiveSource = "persisted"
	}
	if result.EffectiveStorePath == "" {
		return result, nil
	}
	inspection, err := store.InspectLayout(result.EffectiveStorePath)
	if err != nil {
		result.Missing = inspection.Missing
		result.Message = err.Error()
		return result, nil
	}
	result.Valid = inspection.Valid
	result.Missing = inspection.Missing
	if inspection.Valid {
		result.Message = "Loki store is configured."
	} else {
		result.Message = "Loki store path is configured but layout is invalid."
	}
	return result, nil
}

func (s *Service) DiscoverStores(ctx context.Context, req DiscoverStoresRequest) (DiscoverStoresResult, error) {
	if s == nil {
		return DiscoverStoresResult{}, fmt.Errorf("discover stores: service is nil")
	}
	manualPath := req.ManualPath
	if manualPath == "" {
		manualPath = s.storeOverride
	}
	providerCandidates := store.DiscoverProviderFolders(store.DiscoveryOptions{
		GOOS:       s.resolver.GOOS,
		HomeDir:    s.resolver.HomeDir,
		ManualPath: manualPath,
		Env:        s.resolver.Env,
	})
	candidates := make([]StoreCandidate, 0, len(providerCandidates))
	for _, candidate := range providerCandidates {
		out := StoreCandidate{
			Provider:       candidate.Provider,
			ProviderPath:   candidate.Path,
			StorePath:      candidate.StorePath,
			Source:         candidate.Source,
			ProviderExists: candidate.Exists,
		}
		inspection, err := store.InspectLayout(candidate.StorePath)
		out.StoreExists = inspection.Exists
		out.StoreEmpty = inspection.Empty
		out.StoreIsDir = inspection.IsDir
		out.StoreValid = inspection.Valid
		out.Missing = inspection.Missing
		if err != nil {
			out.Missing = append(out.Missing, err.Error())
		}
		candidates = append(candidates, out)
	}
	return DiscoverStoresResult{Candidates: candidates}, nil
}

func (s *Service) UseStore(ctx context.Context, req UseStoreRequest) (EnsureStoreResult, error) {
	if s == nil {
		return EnsureStoreResult{}, fmt.Errorf("store use: service is nil")
	}
	storePath := s.resolver.CleanStoreOverride(req.StorePath)
	if storePath == "" {
		return EnsureStoreResult{}, fmt.Errorf("store use: store path is required")
	}
	inspection, err := store.InspectLayout(storePath)
	if err != nil {
		return EnsureStoreResult{StorePath: storePath, Valid: false, Missing: inspection.Missing}, err
	}
	if !inspection.Exists {
		return EnsureStoreResult{StorePath: storePath, Valid: false}, fmt.Errorf("store use: store path does not exist: %s", storePath)
	}
	if !inspection.Valid {
		return EnsureStoreResult{StorePath: storePath, Valid: false, Missing: inspection.Missing}, fmt.Errorf("store use: invalid store layout: missing %v", inspection.Missing)
	}
	if err := db.SetKV(ctx, s.database, kvStorePath, storePath); err != nil {
		return EnsureStoreResult{}, err
	}
	return EnsureStoreResult{StorePath: storePath, Created: false, Valid: true}, nil
}

func (s *Service) ForgetStore(ctx context.Context, req ForgetStoreRequest) (StoreStatusResult, error) {
	if s == nil {
		return StoreStatusResult{}, fmt.Errorf("store unset: service is nil")
	}
	_ = req
	if err := db.DeleteKV(ctx, s.database, kvStorePath); err != nil {
		return StoreStatusResult{}, err
	}
	return s.StoreStatus(ctx, StoreStatusRequest{})
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

func snapshotRestoreResult(snapshotDir string, plan activation.RestoreDryRunPlan) SnapshotRestoreResult {
	result := SnapshotRestoreResult{
		SnapshotDir: snapshotDir,
		SnapshotID:  plan.Snapshot.SnapshotID,
		DryRun:      true,
		WouldWrite:  false,
		Blockers:    sanitizeRestoreWarnings(plan.Blockers, restorePlanHasSensitiveTarget(plan)),
		Summary: SnapshotRestoreDryRunSummary{
			TargetCount:                   len(plan.Targets),
			PreviousActiveProfile:         plan.Snapshot.PreviousActiveProfile,
			PreviousActiveBuckets:         cloneStrings(plan.Snapshot.PreviousActiveBuckets),
			WouldRestoreManagedTargetRows: len(plan.Snapshot.ManagedTargets),
			WouldRestoreActiveState:       plan.Snapshot.PreviousActiveProfile != "" || len(plan.Snapshot.PreviousActiveBuckets) > 0,
		},
	}
	if plan.TargetFilter != "" {
		if activation.PathLooksSensitive(plan.TargetFilter) {
			result.TargetFilterRedacted = true
		} else {
			result.TargetFilter = plan.TargetFilter
		}
	}
	if plan.TargetFilter != "" {
		result.Summary.WouldRestoreActiveState = false
		result.Summary.WouldRestoreManagedTargetRows = selectedManagedTargetCount(plan)
	}
	for _, target := range plan.Targets {
		result.Targets = append(result.Targets, snapshotRestoreDryRunTarget(target))
		switch target.Action {
		case activation.RestoreActionRestoreFile:
			result.Summary.RestoreFileCount++
		case activation.RestoreActionRestoreDirectory:
			result.Summary.RestoreDirectoryCount++
		case activation.RestoreActionRestoreSymlink:
			result.Summary.RestoreSymlinkCount++
		case activation.RestoreActionRemoveCreatedTarget:
			result.Summary.RemoveCreatedTargetCount++
		case activation.RestoreActionSkipMissingTargetAbsent:
			result.Summary.SkipMissingTargetAbsentCount++
		default:
			result.Summary.UnknownCount++
		}
	}
	return result
}

func selectedManagedTargetCount(plan activation.RestoreDryRunPlan) int {
	selected := map[string]bool{}
	for _, target := range plan.Targets {
		selected[target.Entry.TargetPath] = true
	}
	count := 0
	for _, target := range plan.Snapshot.ManagedTargets {
		if selected[target.TargetPath] {
			count++
		}
	}
	return count
}

func snapshotRestoreDryRunTarget(target activation.RestoreDryRunTarget) SnapshotRestoreDryRunTarget {
	out := SnapshotRestoreDryRunTarget{
		Kind:               target.Entry.Kind,
		Action:             target.Action,
		CurrentExists:      target.CurrentExists,
		CurrentKind:        target.CurrentKind,
		CurrentMode:        target.CurrentMode,
		CurrentHashPrefix:  shortHash(target.CurrentHash),
		SnapshotHashPrefix: shortHash(firstNonEmptyString(target.SnapshotHash, target.Entry.Hash)),
		ExpectedHashPrefix: shortHash(target.Entry.ExpectedHash),
		ExpectedMode:       target.Entry.ExpectedMode,
		SensitivePath:      target.SensitivePath,
		Warnings:           sanitizeRestoreWarnings(target.Warnings, target.SensitivePath || target.SensitiveLinkTarget),
	}
	if target.SensitivePath {
		out.TargetPathRedacted = true
	} else {
		out.TargetPath = target.Entry.TargetPath
	}
	if target.Entry.LinkTarget != "" {
		if target.SensitivePath || target.SensitiveLinkTarget {
			out.LinkTargetRedacted = true
		} else {
			out.LinkTarget = target.Entry.LinkTarget
		}
	}
	if target.SensitivePath && len(out.Warnings) == 0 {
		out.Warnings = append(out.Warnings, "sensitive-looking target path redacted")
	}
	return out
}

func restorePlanHasSensitiveTarget(plan activation.RestoreDryRunPlan) bool {
	for _, target := range plan.Targets {
		if target.SensitivePath || target.SensitiveLinkTarget {
			return true
		}
	}
	return false
}

func sanitizeRestoreWarnings(warnings []string, sensitive bool) []string {
	if len(warnings) == 0 {
		return nil
	}
	if !sensitive {
		return cloneStrings(warnings)
	}
	sanitized := []string{}
	for _, warning := range warnings {
		if strings.Contains(strings.ToLower(warning), "sensitive") {
			sanitized = append(sanitized, warning)
		}
	}
	if len(sanitized) == 0 {
		sanitized = append(sanitized, "sensitive-looking path redacted; some restore checks were omitted")
	}
	return sanitized
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func shortHash(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func (s *Service) recordRestoreGuard(ctx context.Context, plan activation.RestoreDryRunPlan, now time.Time) (time.Time, error) {
	expiresAt := now.UTC().Add(restoreGuardTTL)
	guard := restoreGuard{
		Version:      1,
		SnapshotID:   plan.Snapshot.SnapshotID,
		TargetFilter: plan.TargetFilter,
		Fingerprint:  plan.Fingerprint,
		TargetCount:  len(plan.Targets),
		CreatedAt:    now.UTC().Format(time.RFC3339Nano),
		ExpiresAt:    expiresAt.Format(time.RFC3339Nano),
	}
	content, err := json.Marshal(guard)
	if err != nil {
		return time.Time{}, fmt.Errorf("marshal snapshot restore guard: %w", err)
	}
	if err := db.SetKV(ctx, s.database, restoreGuardKey(plan.Snapshot.SnapshotID, plan.TargetFilter), string(content)); err != nil {
		return time.Time{}, err
	}
	return expiresAt, nil
}

func (s *Service) requireRestoreGuard(ctx context.Context, plan activation.RestoreDryRunPlan, now time.Time) error {
	raw, ok, err := db.GetKV(ctx, s.database, restoreGuardKey(plan.Snapshot.SnapshotID, plan.TargetFilter))
	if err != nil {
		return err
	}
	if !ok || raw == "" {
		return fmt.Errorf("snapshots restore: matching --dry-run guard is required; run `loki snapshots restore %s --dry-run` first", plan.Snapshot.SnapshotID)
	}
	var guard restoreGuard
	if err := json.Unmarshal([]byte(raw), &guard); err != nil {
		_ = s.clearRestoreGuard(ctx, plan.Snapshot.SnapshotID, plan.TargetFilter)
		return fmt.Errorf("snapshots restore: restore guard is invalid; rerun --dry-run")
	}
	if guard.Version != 1 || guard.SnapshotID != plan.Snapshot.SnapshotID || guard.TargetFilter != plan.TargetFilter || guard.Fingerprint != plan.Fingerprint || guard.TargetCount != len(plan.Targets) {
		_ = s.clearRestoreGuard(ctx, plan.Snapshot.SnapshotID, plan.TargetFilter)
		return fmt.Errorf("snapshots restore: restore guard no longer matches current target state; rerun --dry-run")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, guard.ExpiresAt)
	if err != nil || !now.UTC().Before(expiresAt) {
		_ = s.clearRestoreGuard(ctx, plan.Snapshot.SnapshotID, plan.TargetFilter)
		return fmt.Errorf("snapshots restore: restore guard expired; rerun --dry-run")
	}
	return nil
}

func (s *Service) clearRestoreGuard(ctx context.Context, snapshotID string, targetFilter string) error {
	return db.DeleteKV(ctx, s.database, restoreGuardKey(snapshotID, targetFilter))
}

func restoreGuardKey(snapshotID string, targetFilter string) string {
	snapshotID = strings.TrimSpace(snapshotID)
	targetFilter = strings.TrimSpace(targetFilter)
	if targetFilter == "" {
		return restoreGuardPrefix + snapshotID + ":all"
	}
	sum := sha256.Sum256([]byte(targetFilter))
	return restoreGuardPrefix + snapshotID + ":target:" + hex.EncodeToString(sum[:])
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

func (s *Service) withLocalOperationLock(ctx context.Context, operation string, fn func(machineID string) error) error {
	var machineID string
	if id, ok, err := machine.ReadID(s.paths.MachineIDPath); err != nil {
		return err
	} else if ok {
		machineID = id
	}
	return store.WithOperationLock(ctx, s.paths.StateDir, store.OperationLockOptions{Operation: operation, MachineID: machineID}, func() error {
		return fn(machineID)
	})
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
