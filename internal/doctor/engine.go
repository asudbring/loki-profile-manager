package doctor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/allensu/loki-profile-manager/internal/activation"
	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/infisical"
	"github.com/allensu/loki-profile-manager/internal/machine"
	"github.com/allensu/loki-profile-manager/internal/store"
)

const (
	machineStaleAfter = 30 * 24 * time.Hour
	conflictScanLimit = 100
)

type Request struct {
	Version         string
	StorePath       string
	StoreOverride   string
	LocalPaths      config.LocalPaths
	Resolver        config.PathResolver
	Database        *sql.DB
	DatabaseMissing bool
	DatabaseError   string
	Now             func() time.Time
}

func Run(ctx context.Context, req Request) Report {
	if ctx == nil {
		ctx = context.Background()
	}
	now := req.Now
	if now == nil {
		now = time.Now
	}
	report := Report{
		Healthy:       true,
		Version:       req.Version,
		Runtime:       RuntimeInfo{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH},
		StorePath:     strings.TrimSpace(req.StorePath),
		StoreOverride: strings.TrimSpace(req.StoreOverride),
		LocalPaths:    req.LocalPaths,
		Checks:        []Check{},
	}

	addEnvironmentChecks(&report, req)
	addSQLiteChecks(ctx, &report, req.Database, req.DatabaseMissing, req.DatabaseError)
	addProviderChecks(&report, req)

	storeUsable := addStoreChecks(&report, report.StorePath)
	addSnapshotChecks(ctx, &report, req.Database, req.LocalPaths.SnapshotDir)
	addDependencyChecks(ctx, &report)
	if report.StorePath != "" {
		addLockChecks(&report, report.StorePath, now())
		addConflictChecks(&report, report.StorePath)
	}
	if storeUsable {
		addMachineChecks(&report, report.StorePath, req.LocalPaths.MachineIDPath, now())
	}

	report.Healthy = report.Summary.Blocking == 0
	return report
}

func addEnvironmentChecks(report *Report, req Request) {
	report.add(Check{
		Severity: SeverityInfo,
		Code:     "environment.runtime",
		Category: "environment",
		Message:  fmt.Sprintf("runtime is %s/%s", runtime.GOOS, runtime.GOARCH),
		Details:  map[string]string{"goos": runtime.GOOS, "goarch": runtime.GOARCH},
	})
	addDirStatCheck(report, "environment.state_dir", "environment", req.LocalPaths.StateDir, "local state directory exists")
	addDirStatCheck(report, "environment.log_dir", "environment", req.LocalPaths.LogDir, "log directory exists")
	if req.LocalPaths.SnapshotDir != "" {
		if info, err := os.Stat(req.LocalPaths.SnapshotDir); err == nil && info.IsDir() {
			report.add(Check{Severity: SeverityInfo, Code: "environment.snapshot_dir", Category: "environment", Message: "snapshot directory exists", Path: req.LocalPaths.SnapshotDir})
		} else if errors.Is(err, os.ErrNotExist) {
			report.add(Check{Severity: SeverityInfo, Code: "environment.snapshot_dir_missing", Category: "environment", Message: "snapshot directory has not been created yet", Path: req.LocalPaths.SnapshotDir})
		} else if err != nil {
			report.add(Check{Severity: SeverityWarning, Code: "environment.snapshot_dir_unreadable", Category: "environment", Message: err.Error(), Path: req.LocalPaths.SnapshotDir})
		}
	}
	if req.Resolver.HomeDir == "" {
		report.add(Check{Severity: SeverityWarning, Code: "environment.home_missing", Category: "environment", Message: "home directory could not be resolved", Remediation: "Set HOME/USERPROFILE or pass an injected resolver in tests."})
	} else if info, err := os.Stat(req.Resolver.HomeDir); err != nil || !info.IsDir() {
		message := "home directory does not exist or is not a directory"
		if err != nil {
			message = err.Error()
		}
		report.add(Check{Severity: SeverityWarning, Code: "environment.home_invalid", Category: "environment", Message: message, Path: req.Resolver.HomeDir})
	} else {
		report.add(Check{Severity: SeverityInfo, Code: "environment.home", Category: "environment", Message: "home directory exists", Path: req.Resolver.HomeDir})
	}
}

func addDirStatCheck(report *Report, code, category, pathValue, okMessage string) {
	if pathValue == "" {
		report.add(Check{Severity: SeverityWarning, Code: code + "_missing", Category: category, Message: "path is not configured"})
		return
	}
	info, err := os.Stat(pathValue)
	if err != nil {
		severity := SeverityWarning
		if errors.Is(err, os.ErrNotExist) {
			report.add(Check{Severity: severity, Code: code + "_missing", Category: category, Message: "directory does not exist", Path: pathValue})
			return
		}
		report.add(Check{Severity: severity, Code: code + "_unreadable", Category: category, Message: err.Error(), Path: pathValue})
		return
	}
	if !info.IsDir() {
		report.add(Check{Severity: SeverityWarning, Code: code + "_not_directory", Category: category, Message: "path exists but is not a directory", Path: pathValue})
		return
	}
	report.add(Check{Severity: SeverityInfo, Code: code, Category: category, Message: okMessage, Path: pathValue})
}

func addSQLiteChecks(ctx context.Context, report *Report, database *sql.DB, databaseMissing bool, databaseError string) {
	if database == nil {
		if databaseError != "" {
			report.add(Check{Severity: SeverityBlocking, Code: "sqlite.open_failed", Category: "sqlite", Message: databaseError})
			return
		}
		if databaseMissing {
			report.add(Check{Severity: SeverityWarning, Code: "sqlite.database_missing", Category: "sqlite", Message: "local SQLite database does not exist yet", Remediation: "Run `loki status` or another stateful command to initialize local state."})
			return
		}
		report.add(Check{Severity: SeverityBlocking, Code: "sqlite.database_unavailable", Category: "sqlite", Message: "local SQLite database is not open"})
		return
	}
	if err := database.PingContext(ctx); err != nil {
		report.add(Check{Severity: SeverityBlocking, Code: "sqlite.ping_failed", Category: "sqlite", Message: err.Error()})
		return
	}
	report.add(Check{Severity: SeverityInfo, Code: "sqlite.ping_ok", Category: "sqlite", Message: "local SQLite database responds"})
	rows, err := database.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		report.add(Check{Severity: SeverityBlocking, Code: "sqlite.integrity_failed", Category: "sqlite", Message: err.Error()})
		return
	}
	var results []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			_ = rows.Close()
			report.add(Check{Severity: SeverityBlocking, Code: "sqlite.integrity_failed", Category: "sqlite", Message: err.Error()})
			return
		}
		results = append(results, value)
	}
	if err := rows.Close(); err != nil {
		report.add(Check{Severity: SeverityBlocking, Code: "sqlite.integrity_failed", Category: "sqlite", Message: err.Error()})
		return
	}
	if len(results) == 1 && strings.EqualFold(results[0], "ok") {
		report.add(Check{Severity: SeverityInfo, Code: "sqlite.integrity_ok", Category: "sqlite", Message: "SQLite integrity check passed"})
	} else {
		report.add(Check{Severity: SeverityBlocking, Code: "sqlite.integrity_failed", Category: "sqlite", Message: "SQLite integrity check returned issues", Details: map[string]string{"results": strings.Join(results, "; ")}})
	}
	addSQLiteTableChecks(ctx, report, database)
}

func addSQLiteTableChecks(ctx context.Context, report *Report, database *sql.DB) {
	rows, err := database.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		report.add(Check{Severity: SeverityBlocking, Code: "sqlite.tables_unreadable", Category: "sqlite", Message: err.Error()})
		return
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			report.add(Check{Severity: SeverityBlocking, Code: "sqlite.tables_unreadable", Category: "sqlite", Message: err.Error()})
			return
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		report.add(Check{Severity: SeverityBlocking, Code: "sqlite.tables_unreadable", Category: "sqlite", Message: err.Error()})
		return
	}
	required := []string{"schema_migrations", "kv_state", "managed_targets", "snapshots", "pending_captures", "command_history"}
	missing := []string{}
	for _, name := range required {
		if !found[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		report.add(Check{Severity: SeverityBlocking, Code: "sqlite.table_missing", Category: "sqlite", Message: "required SQLite table(s) missing", Details: map[string]string{"tables": strings.Join(missing, ", ")}})
		return
	}
	report.add(Check{Severity: SeverityInfo, Code: "sqlite.tables_ok", Category: "sqlite", Message: "required SQLite tables exist"})
}

func addProviderChecks(report *Report, req Request) {
	manualPath := req.StoreOverride
	if manualPath == "" {
		manualPath = req.StorePath
	}
	candidates := store.DiscoverProviderFolders(store.DiscoveryOptions{
		GOOS:       req.Resolver.GOOS,
		HomeDir:    req.Resolver.HomeDir,
		ManualPath: manualPath,
		Env:        req.Resolver.Env,
	})
	report.ProviderCandidates = candidates
	if len(candidates) == 0 {
		report.add(Check{Severity: SeverityWarning, Code: "provider.candidate_missing", Category: "provider", Message: "no OneDrive, Dropbox, or manual provider candidates found", Remediation: "Pass --store <path> or configure a synced OneDrive/Dropbox folder."})
		return
	}
	existing := 0
	for _, candidate := range candidates {
		if candidate.Exists {
			existing++
		}
	}
	if existing == 0 {
		report.add(Check{Severity: SeverityWarning, Code: "provider.root_missing", Category: "provider", Message: "provider candidates were detected but none exist locally", Remediation: "Start OneDrive/Dropbox sync or pass a valid --store path."})
		return
	}
	report.add(Check{Severity: SeverityInfo, Code: "provider.candidate_found", Category: "provider", Message: fmt.Sprintf("%d provider candidate(s) found, %d exist locally", len(candidates), existing), Details: map[string]string{"candidates": strconv.Itoa(len(candidates)), "existing": strconv.Itoa(existing)}})
}

func addStoreChecks(report *Report, storePath string) bool {
	if strings.TrimSpace(storePath) == "" {
		report.add(Check{Severity: SeverityWarning, Code: "store.not_configured", Category: "store", Message: "Loki store is not configured", Remediation: "Pass --store <path> or initialize a Loki store."})
		return false
	}
	info, err := os.Stat(storePath)
	if errors.Is(err, os.ErrNotExist) {
		report.add(Check{Severity: SeverityBlocking, Code: "store.root_missing", Category: "store", Message: "configured store path does not exist", Path: storePath, Remediation: "Choose a valid --store path or initialize the store layout."})
		return false
	}
	if err != nil {
		report.add(Check{Severity: SeverityBlocking, Code: "store.root_unreadable", Category: "store", Message: err.Error(), Path: storePath})
		return false
	}
	if !info.IsDir() {
		report.add(Check{Severity: SeverityBlocking, Code: "store.root_not_directory", Category: "store", Message: "configured store path is not a directory", Path: storePath})
		return false
	}
	validation := store.ValidateLayout(storePath)
	if !validation.Valid {
		report.add(Check{Severity: SeverityBlocking, Code: "store.layout_missing", Category: "store", Message: fmt.Sprintf("store layout is missing %d required path(s)", len(validation.Missing)), Path: storePath, Remediation: "Run store setup/ensure layout or choose a valid --store path.", Details: map[string]string{"missing": joinLimited(validation.Missing, 5)}})
		return false
	}
	report.add(Check{Severity: SeverityInfo, Code: "store.layout_valid", Category: "store", Message: "store layout is valid", Path: storePath})
	return true
}

func addLockChecks(report *Report, storePath string, now time.Time) {
	lockPath := store.OperationLockPath(storePath)
	content, err := os.ReadFile(lockPath)
	if errors.Is(err, os.ErrNotExist) {
		report.add(Check{Severity: SeverityInfo, Code: "lock.operation_absent", Category: "lock", Message: "no store operation lock present", Path: lockPath})
	} else if err != nil {
		report.add(Check{Severity: SeverityWarning, Code: "lock.operation_unreadable", Category: "lock", Message: err.Error(), Path: lockPath})
	} else {
		var info store.OperationLockInfo
		if err := json.Unmarshal(content, &info); err != nil {
			report.add(Check{Severity: SeverityWarning, Code: "lock.operation_unreadable", Category: "lock", Message: err.Error(), Path: lockPath, Remediation: "Remove this lock only after confirming no Loki process is active."})
		} else if operationLockExpired(info, now) {
			report.add(Check{Severity: SeverityWarning, Code: "lock.operation_stale", Category: "lock", Message: fmt.Sprintf("store operation lock for %q appears stale", info.Operation), Path: lockPath, Remediation: "Remove this lock only after confirming no Loki process is active."})
		} else {
			report.add(Check{Severity: SeverityWarning, Code: "lock.operation_present", Category: "lock", Message: fmt.Sprintf("store operation lock for %q is present", info.Operation), Path: lockPath, Remediation: "Wait for the active Loki command to finish."})
		}
	}
	registryLock := filepath.Join(storePath, "registry", "machines.json.lock")
	if _, err := os.Stat(registryLock); err == nil {
		report.add(Check{Severity: SeverityWarning, Code: "lock.registry_present", Category: "lock", Message: "machine registry lock is present", Path: registryLock, Remediation: "Remove this lock only after confirming no Loki process is active."})
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		report.add(Check{Severity: SeverityWarning, Code: "lock.registry_unreadable", Category: "lock", Message: err.Error(), Path: registryLock})
	}
}

func operationLockExpired(info store.OperationLockInfo, now time.Time) bool {
	if info.ExpiresAt != "" {
		if expiresAt, err := time.Parse(time.RFC3339Nano, info.ExpiresAt); err == nil {
			return now.UTC().After(expiresAt)
		}
	}
	if info.AcquiredAt != "" {
		if acquiredAt, err := time.Parse(time.RFC3339Nano, info.AcquiredAt); err == nil {
			return now.UTC().Sub(acquiredAt) > 30*time.Minute
		}
	}
	return false
}

func addMachineChecks(report *Report, storePath, machineIDPath string, now time.Time) {
	machineID, ok, err := machine.ReadID(machineIDPath)
	if err != nil {
		report.add(Check{Severity: SeverityWarning, Code: "machine.id_invalid", Category: "machine", Message: err.Error(), Path: machineIDPath, Remediation: "Move the invalid machine ID file aside and re-run `loki machine register --allow-profile <profile>`."})
		return
	}
	registry, err := machine.ReadRegistry(storePath)
	if err != nil {
		report.add(Check{Severity: SeverityBlocking, Code: "machine.registry_unreadable", Category: "machine", Message: err.Error(), Path: filepath.Join(storePath, "registry", "machines.json")})
		return
	}
	if !ok {
		report.add(Check{Severity: SeverityWarning, Code: "machine.id_missing", Category: "machine", Message: "local machine ID has not been created", Path: machineIDPath, Remediation: "Run `loki machine register --allow-profile <profile>` before switching profiles."})
		addStaleMachineChecks(report, registry.Machines, now)
		return
	}
	var current *machine.Record
	for i := range registry.Machines {
		if registry.Machines[i].MachineID == machineID {
			current = &registry.Machines[i]
			break
		}
	}
	if current == nil {
		report.add(Check{Severity: SeverityWarning, Code: "machine.record_missing", Category: "machine", Message: fmt.Sprintf("local machine %s is not registered in store", machineID), Path: filepath.Join(storePath, "registry", "machines.json"), Remediation: "Run `loki machine register --allow-profile <profile>`."})
		addStaleMachineChecks(report, registry.Machines, now)
		return
	}
	report.add(Check{Severity: SeverityInfo, Code: "machine.registered", Category: "machine", Message: fmt.Sprintf("machine %s is registered", machineID), Path: filepath.Join(storePath, "registry", "machines.json")})
	if _, err := machine.ReadHeartbeat(storePath, machineID); err != nil {
		report.add(Check{Severity: SeverityWarning, Code: "machine.heartbeat_missing", Category: "machine", Message: err.Error(), Path: filepath.Join(storePath, "registry", "machines", machineID+".json"), Remediation: "Run `loki machine register --allow-profile <profile>` or `loki switch <profile> --yes` to refresh heartbeat."})
	} else {
		report.add(Check{Severity: SeverityInfo, Code: "machine.heartbeat_present", Category: "machine", Message: "current machine heartbeat exists", Path: filepath.Join(storePath, "registry", "machines", machineID+".json")})
	}
	addStaleMachineChecks(report, registry.Machines, now)
}

func addStaleMachineChecks(report *Report, records []machine.Record, now time.Time) {
	stale := 0
	for _, record := range records {
		if strings.TrimSpace(record.LastSeen) == "" {
			report.add(Check{Severity: SeverityInfo, Code: "machine.last_seen_missing", Category: "machine", Message: fmt.Sprintf("machine %s has no last_seen timestamp", record.MachineID), Remediation: "Re-register or switch on that machine to refresh heartbeat."})
			continue
		}
		lastSeen, err := time.Parse(time.RFC3339, record.LastSeen)
		if err != nil {
			report.add(Check{Severity: SeverityWarning, Code: "machine.last_seen_invalid", Category: "machine", Message: fmt.Sprintf("machine %s has invalid last_seen timestamp", record.MachineID), Remediation: "Re-register the machine or fix registry/machines.json after backup."})
			continue
		}
		if now.UTC().Sub(lastSeen) > machineStaleAfter {
			stale++
			report.add(Check{Severity: SeverityWarning, Code: "machine.heartbeat_stale", Category: "machine", Message: fmt.Sprintf("machine %s heartbeat is older than 30 days", record.MachineID), Remediation: "Confirm the device is retired before removing it from registry/machines.json."})
		}
	}
	if len(records) > 0 && stale == 0 {
		report.add(Check{Severity: SeverityInfo, Code: "machine.heartbeats_fresh", Category: "machine", Message: "registered machine heartbeats are fresh"})
	}
}

func addSnapshotChecks(ctx context.Context, report *Report, database *sql.DB, snapshotDir string) {
	if snapshotDir == "" {
		report.add(Check{Severity: SeverityInfo, Code: "snapshot.dir_missing", Category: "snapshot", Message: "snapshot directory is not configured"})
		return
	}
	if _, err := os.Stat(snapshotDir); errors.Is(err, os.ErrNotExist) {
		report.add(Check{Severity: SeverityInfo, Code: "snapshot.dir_missing", Category: "snapshot", Message: "snapshot directory has not been created yet", Path: snapshotDir})
		return
	} else if err != nil {
		report.add(Check{Severity: SeverityWarning, Code: "snapshot.dir_unreadable", Category: "snapshot", Message: err.Error(), Path: snapshotDir})
		return
	}
	summaries, err := activation.ListSnapshots(ctx, database, snapshotDir)
	if err != nil {
		report.add(Check{Severity: SeverityWarning, Code: "snapshot.list_failed", Category: "snapshot", Message: err.Error(), Path: snapshotDir})
		return
	}
	if len(summaries) == 0 {
		report.add(Check{Severity: SeverityInfo, Code: "snapshot.none", Category: "snapshot", Message: "no local snapshots found", Path: snapshotDir})
		return
	}
	report.add(Check{Severity: SeverityInfo, Code: "snapshot.found", Category: "snapshot", Message: fmt.Sprintf("%d local snapshot(s) found", len(summaries)), Path: snapshotDir, Details: map[string]string{"count": strconv.Itoa(len(summaries))}})
	if len(summaries) > 2 {
		report.add(Check{Severity: SeverityInfo, Code: "snapshot.retention_excess", Category: "snapshot", Message: "more than two snapshots are present", Path: snapshotDir, Remediation: "Retention normally keeps the latest two snapshots after successful switch operations."})
	}
	for _, summary := range summaries {
		if !summary.Exists {
			report.add(Check{Severity: SeverityWarning, Code: "snapshot.path_missing", Category: "snapshot", Message: fmt.Sprintf("snapshot %s metadata points to missing directory", summary.SnapshotID), Path: safePath(summary.Path)})
		}
		if summary.MetadataError != "" {
			report.add(Check{Severity: SeverityWarning, Code: "snapshot.metadata_invalid", Category: "snapshot", Message: fmt.Sprintf("snapshot %s metadata is invalid: %s", summary.SnapshotID, summary.MetadataError), Path: safePath(summary.Path)})
		}
	}
}

func addDependencyChecks(ctx context.Context, report *Report) {
	client := infisical.NewClient(nil)
	if err := client.CheckInstalled(ctx); err != nil {
		report.add(Check{Severity: SeverityWarning, Code: "dependency.infisical_missing", Category: "dependency", Message: err.Error(), Remediation: "Install and authenticate Infisical before activating render templates."})
		return
	}
	report.add(Check{Severity: SeverityInfo, Code: "dependency.infisical_found", Category: "dependency", Message: "Infisical CLI is installed"})
}

func addConflictChecks(report *Report, storePath string) {
	matches, truncated, err := findConflictCopies(storePath, conflictScanLimit)
	if err != nil {
		report.add(Check{Severity: SeverityWarning, Code: "sync.conflict_scan_failed", Category: "sync", Message: err.Error(), Path: storePath})
		return
	}
	if len(matches) == 0 {
		report.add(Check{Severity: SeverityInfo, Code: "sync.conflict_copy_none", Category: "sync", Message: "no provider conflict-copy filenames found", Path: storePath})
		return
	}
	details := map[string]string{"count": strconv.Itoa(len(matches))}
	if truncated {
		details["truncated"] = "true"
	}
	code := "sync.conflict_copy_found"
	if truncated {
		code = "sync.conflict_scan_truncated"
	}
	report.add(Check{Severity: SeverityWarning, Code: code, Category: "sync", Message: fmt.Sprintf("%d provider conflict-copy filename(s) found", len(matches)), Path: safePath(matches[0]), Remediation: "Inspect conflict copies before deleting or reconciling them.", Details: details})
}

func findConflictCopies(root string, limit int) ([]string, bool, error) {
	matches := []string{}
	truncated := false
	var scanErr error
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if scanErr == nil {
				scanErr = fmt.Errorf("scan %s: %w", path, err)
			}
			return nil
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "node_modules" || name == ".DS_Store" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isConflictCopyName(entry.Name()) {
			return nil
		}
		matches = append(matches, path)
		if len(matches) >= limit {
			truncated = true
			return fs.SkipAll
		}
		return nil
	})
	if errors.Is(err, fs.SkipAll) {
		err = nil
	}
	if err == nil {
		err = scanErr
	}
	sort.Strings(matches)
	return matches, truncated, err
}

func isConflictCopyName(name string) bool {
	lower := strings.ToLower(name)
	patterns := []string{"conflicted copy", "conflict copy", "sync conflict", "case conflict"}
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return strings.Contains(lower, "conflict") && (strings.Contains(lower, "onedrive") || strings.Contains(lower, "dropbox"))
}

func safePath(pathValue string) string {
	if activation.PathLooksSensitive(pathValue) {
		return "[redacted]"
	}
	return pathValue
}

func joinLimited(values []string, limit int) string {
	if len(values) == 0 {
		return ""
	}
	if len(values) <= limit {
		return strings.Join(values, ", ")
	}
	return strings.Join(values[:limit], ", ") + fmt.Sprintf(", ... (%d more)", len(values)-limit)
}
