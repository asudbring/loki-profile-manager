package doctor

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/db"
	"github.com/asudbring/loki-profile-manager/internal/machine"
	"github.com/asudbring/loki-profile-manager/internal/secrets"
	"github.com/asudbring/loki-profile-manager/internal/store"
)

func TestRunDetectsStaleLockStaleMachineAndConflictCopy(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	database := openDoctorTestDB(t, ctx, paths.DBPath)
	defer database.Close()

	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}

	machineID := "11111111-1111-4111-8111-111111111111"
	if err := os.MkdirAll(filepath.Dir(paths.MachineIDPath), 0o700); err != nil {
		t.Fatalf("mkdir machine id: %v", err)
	}
	if err := os.WriteFile(paths.MachineIDPath, []byte(machineID+"\n"), 0o600); err != nil {
		t.Fatalf("write machine id: %v", err)
	}

	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	old := now.Add(-31 * 24 * time.Hour)
	record := machine.NewRecord(machineID, "old mac", "darwin", "old-host", []string{"work"}, nil, "dev", old)
	if err := machine.UpsertMachine(storePath, record); err != nil {
		t.Fatalf("UpsertMachine() error = %v", err)
	}

	lockInfo := store.OperationLockInfo{Version: 1, PID: 123, Operation: "switch", AcquiredAt: old.Format(time.RFC3339Nano), ExpiresAt: old.Add(30 * time.Minute).Format(time.RFC3339Nano), Token: "test-token"}
	lockContent, err := json.Marshal(lockInfo)
	if err != nil {
		t.Fatalf("marshal lock: %v", err)
	}
	if err := os.WriteFile(store.OperationLockPath(storePath), lockContent, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	conflictPath := filepath.Join(storePath, "profiles", "work", "core", "files", "settings conflicted copy.txt")
	if err := os.WriteFile(conflictPath, []byte("conflict"), 0o600); err != nil {
		t.Fatalf("write conflict file: %v", err)
	}

	report := Run(ctx, Request{Version: "test", StorePath: storePath, LocalPaths: paths, Resolver: resolver, Database: database, Now: func() time.Time { return now }})
	for _, code := range []string{"lock.operation_stale", "machine.heartbeat_stale", "sync.conflict_copy_found"} {
		if !hasCheck(report, code) {
			t.Fatalf("report missing %s: %+v", code, report.Checks)
		}
	}
}

func TestRunDetectsStaleManagedState(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	database := openDoctorTestDB(t, ctx, paths.DBPath)
	defer database.Close()
	storePath, sourcePath, targetPath := prepareStaleManagedState(t, ctx, database, resolver)

	report := Run(ctx, Request{Version: "test", StorePath: storePath, LocalPaths: paths, Resolver: resolver, Database: database})

	check, ok := findCheck(report, "managed_state.stale")
	if !ok {
		t.Fatalf("report missing managed_state.stale: %+v", report.Checks)
	}
	if check.Details["repairable"] != "1" || check.Path != targetPath || !strings.Contains(check.Remediation, "--repair-managed-state") {
		t.Fatalf("stale check = %+v, want repairable target", check)
	}
	if got, _ := os.ReadFile(targetPath); string(got) != "{\"packages\":[\"a\",\"b\"]}\n" {
		t.Fatalf("target changed during detect-only doctor: %q", got)
	}
	if got, _ := os.ReadFile(sourcePath); string(got) == "" {
		t.Fatal("source missing after detect-only doctor")
	}
}

func TestRunRepairsManagedStateAndWritesSafeFile(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	database := openDoctorTestDB(t, ctx, paths.DBPath)
	defer database.Close()
	storePath, sourcePath, targetPath := prepareStaleManagedState(t, ctx, database, resolver)
	now := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)

	report := Run(ctx, Request{Version: "test", StorePath: storePath, LocalPaths: paths, Resolver: resolver, Database: database, RepairManagedState: true, WriteSafeFiles: true, Now: func() time.Time { return now }})

	check, ok := findCheck(report, "managed_state.repaired")
	if !ok || check.Details["repaired"] != "1" || check.Details["wrote_files"] != "1" {
		t.Fatalf("repair check = %+v, want one repaired write", check)
	}
	want, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("target = %q, want canonical source %q", got, want)
	}
	record, ok, err := activation.GetManagedTarget(ctx, database, targetPath)
	if err != nil || !ok {
		t.Fatalf("GetManagedTarget() = %+v, %v, %v", record, ok, err)
	}
	hash, err := activation.HashPath(targetPath)
	if err != nil {
		t.Fatalf("HashPath() error = %v", err)
	}
	if record.ContentHash != hash || record.Mode != "merge" || record.SourcePath != sourcePath || record.LayerKind != "common" || record.LayerName != "common" {
		t.Fatalf("managed record = %+v, want repaired state hash=%s", record, hash)
	}
}

func TestRunSkipsManagedStateRepairWhenOperationLockHeld(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	database := openDoctorTestDB(t, ctx, paths.DBPath)
	defer database.Close()
	storePath, _, targetPath := prepareStaleManagedState(t, ctx, database, resolver)
	unlock, err := store.AcquireOperationLock(ctx, storePath, store.OperationLockOptions{Operation: "test-holder"})
	if err != nil {
		t.Fatalf("AcquireOperationLock() error = %v", err)
	}
	defer unlock()

	report := Run(ctx, Request{Version: "test", StorePath: storePath, LocalPaths: paths, Resolver: resolver, Database: database, RepairManagedState: true, WriteSafeFiles: true})

	if _, ok := findCheck(report, "managed_state.repaired"); ok {
		t.Fatalf("unexpected repair while lock held: %+v", report.Checks)
	}
	if _, ok := findCheck(report, "managed_state.repair_skipped_locked"); !ok {
		t.Fatalf("missing repair_skipped_locked check: %+v", report.Checks)
	}
	record, ok, err := activation.GetManagedTarget(ctx, database, targetPath)
	if err != nil || !ok {
		t.Fatalf("GetManagedTarget() = %+v, %v, %v", record, ok, err)
	}
	if record.ContentHash != "stale-hash" || record.Mode != "copy" {
		t.Fatalf("record = %+v, want unrepaired stale state", record)
	}
}

func TestRunDoesNotRepairJSONNumbersThatLoseFloatPrecision(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	database := openDoctorTestDB(t, ctx, paths.DBPath)
	defer database.Close()
	storePath, sourcePath, targetPath := prepareStaleManagedState(t, ctx, database, resolver)
	if err := os.WriteFile(sourcePath, []byte("{\"id\":9007199254740992}\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("{\"id\":9007199254740993}\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	report := Run(ctx, Request{Version: "test", StorePath: storePath, LocalPaths: paths, Resolver: resolver, Database: database, RepairManagedState: true, WriteSafeFiles: true})

	if _, ok := findCheck(report, "managed_state.repaired"); ok {
		t.Fatalf("unexpected repair for distinct large JSON numbers: %+v", report.Checks)
	}
	check, ok := findCheck(report, "managed_state.unrepairable")
	if !ok || check.Details["unrepairable"] != "1" {
		t.Fatalf("unrepairable check = %+v, want one unrepairable", check)
	}
}

func TestRunDoesNotRepairSemanticDifference(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	database := openDoctorTestDB(t, ctx, paths.DBPath)
	defer database.Close()
	storePath, _, targetPath := prepareStaleManagedState(t, ctx, database, resolver)
	if err := os.WriteFile(targetPath, []byte("{\"packages\":[\"local-only\"]}\n"), 0o600); err != nil {
		t.Fatalf("write divergent target: %v", err)
	}

	report := Run(ctx, Request{Version: "test", StorePath: storePath, LocalPaths: paths, Resolver: resolver, Database: database, RepairManagedState: true, WriteSafeFiles: true})

	if _, ok := findCheck(report, "managed_state.repaired"); ok {
		t.Fatalf("unexpected repair for divergent target: %+v", report.Checks)
	}
	check, ok := findCheck(report, "managed_state.unrepairable")
	if !ok || check.Details["unrepairable"] != "1" {
		t.Fatalf("unrepairable check = %+v, want one unrepairable", check)
	}
}

func TestRunRepairsMultiSourceMergeManagedState(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	database := openDoctorTestDB(t, ctx, paths.DBPath)
	defer database.Close()
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	commonSource := filepath.Join(storePath, "profiles", "common", "files", "base.json")
	workSource := filepath.Join(storePath, "profiles", "work", "core", "files", "overlay.json")
	for _, source := range []string{commonSource, workSource} {
		if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
			t.Fatalf("mkdir source: %v", err)
		}
	}
	if err := os.WriteFile(commonSource, []byte(`{"packages":["a"]}`), 0o600); err != nil {
		t.Fatalf("write common source: %v", err)
	}
	if err := os.WriteFile(workSource, []byte(`{"extra":true}`), 0o600); err != nil {
		t.Fatalf("write work source: %v", err)
	}
	commonManifest := `version: 1
name: common
files:
  - id: pi-settings-base
    source: files/base.json
    target: ~/.pi/agent/settings.json
    mode: merge
    format: json
skills: []
ignore: []
merge_rules: {}
targets: {}
`
	workManifest := `version: 1
name: work-core
files:
  - id: pi-settings-overlay
    source: files/overlay.json
    target: ~/.pi/agent/settings.json
    mode: merge
    format: json
skills: []
ignore: []
merge_rules: {}
targets: {}
`
	if err := os.WriteFile(filepath.Join(storePath, "profiles", "common", "manifest.yaml"), []byte(commonManifest), 0o600); err != nil {
		t.Fatalf("write common manifest: %v", err)
	}
	workManifestPath := filepath.Join(storePath, "profiles", "work", "core", "manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(workManifestPath), 0o755); err != nil {
		t.Fatalf("mkdir work manifest: %v", err)
	}
	if err := os.WriteFile(workManifestPath, []byte(workManifest), 0o600); err != nil {
		t.Fatalf("write work manifest: %v", err)
	}
	targetPath := filepath.Join(home, ".pi", "agent", "settings.json")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte(`{"extra":true,"packages":["a"]}`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	metadata, err := json.Marshal(map[string]any{"sources": []activation.Source{{Path: commonSource, LayerName: "common", LayerKind: "common", FileID: "pi-settings-base", Order: 0}, {Path: workSource, LayerName: "work-core", LayerKind: "core", FileID: "pi-settings-overlay", Order: 1}}})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := activation.PutManagedTarget(ctx, database, activation.ManagedTarget{TargetPath: targetPath, Mode: "copy", ContentHash: "stale-hash", LayerKind: "common", LayerName: "common", LastAppliedAt: "2026-05-18T00:00:00Z", MetadataJSON: string(metadata)}); err != nil {
		t.Fatalf("PutManagedTarget() error = %v", err)
	}

	report := Run(ctx, Request{Version: "test", StorePath: storePath, LocalPaths: paths, Resolver: resolver, Database: database, RepairManagedState: true, WriteSafeFiles: true})

	check, ok := findCheck(report, "managed_state.repaired")
	if !ok || check.Details["repaired"] != "1" || check.Details["wrote_files"] != "1" {
		t.Fatalf("repair check = %+v, want multi-source merge repair", check)
	}
	record, ok, err := activation.GetManagedTarget(ctx, database, targetPath)
	if err != nil || !ok {
		t.Fatalf("GetManagedTarget() = %+v, %v, %v", record, ok, err)
	}
	if record.SourcePath != "" || record.Mode != "merge" || record.LayerKind != "merged" || record.LayerName != "merged" {
		t.Fatalf("record = %+v, want merged state with empty source path", record)
	}
	var repairedMetadata struct {
		Sources []activation.Source `json:"sources"`
	}
	if err := json.Unmarshal([]byte(record.MetadataJSON), &repairedMetadata); err != nil {
		t.Fatalf("unmarshal repaired metadata: %v", err)
	}
	if len(repairedMetadata.Sources) != 2 || repairedMetadata.Sources[0].Path != commonSource || repairedMetadata.Sources[1].Path != workSource {
		t.Fatalf("metadata sources = %+v, want preserved merge source provenance", repairedMetadata.Sources)
	}
}

func TestRunDoesNotRepairMergeWhenCurrentManifestAddsSource(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	database := openDoctorTestDB(t, ctx, paths.DBPath)
	defer database.Close()
	storePath, _, targetPath := prepareStaleManagedState(t, ctx, database, resolver)
	// Convert fixture into a source-less merge record with one old metadata source,
	// then add a second current manifest source. Doctor must not repair from the stale subset.
	metadata, err := json.Marshal(map[string]any{"sources": []activation.Source{{Path: filepath.Join(storePath, "profiles", "common", "files", "dot-pi", "agent", "settings.json"), LayerName: "common", LayerKind: "common", FileID: "pi-settings", Order: 0}}})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := activation.PutManagedTarget(ctx, database, activation.ManagedTarget{TargetPath: targetPath, Mode: "merge", ContentHash: "stale-hash", LayerKind: "merged", LayerName: "merged", LastAppliedAt: "2026-05-18T00:00:00Z", MetadataJSON: string(metadata)}); err != nil {
		t.Fatalf("PutManagedTarget() error = %v", err)
	}
	manifestPath := filepath.Join(storePath, "profiles", "common", "manifest.yaml")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	extraSource := filepath.Join(storePath, "profiles", "common", "files", "dot-pi", "agent", "extra.json")
	if err := os.WriteFile(extraSource, []byte("{\"extra\":true}\n"), 0o600); err != nil {
		t.Fatalf("write extra source: %v", err)
	}
	extraEntry := `  - id: pi-settings-extra
    source: files/dot-pi/agent/extra.json
    target: ~/.pi/agent/settings.json
    mode: merge
    format: json
`
	content = []byte(strings.Replace(string(content), "skills: []", extraEntry+"skills: []", 1))
	if err := os.WriteFile(manifestPath, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	report := Run(ctx, Request{Version: "test", StorePath: storePath, LocalPaths: paths, Resolver: resolver, Database: database, RepairManagedState: true, WriteSafeFiles: true})

	if _, ok := findCheck(report, "managed_state.repaired"); ok {
		t.Fatalf("unexpected repair from stale merge metadata subset: %+v", report.Checks)
	}
}

func TestRunReportsInfisicalReadiness(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		status secrets.Status
		code   string
	}{
		{name: "missing", status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: false}, code: "dependency.infisical_missing"},
		{name: "not ready", status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Ready: false, Checks: []secrets.Check{{Severity: secrets.SeverityWarning, Message: "not ready"}}}, code: "dependency.infisical_not_ready"},
		{name: "ready", status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: true, Ready: true}, code: "dependency.infisical_ready"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := Run(ctx, Request{SecretStatusChecker: fakeDoctorSecretStatusChecker{status: tc.status}})
			if !hasCheck(report, tc.code) {
				t.Fatalf("report missing %s: %+v", tc.code, report.Checks)
			}
		})
	}
}

type fakeDoctorSecretStatusChecker struct {
	status secrets.Status
}

func (f fakeDoctorSecretStatusChecker) CheckStatus(ctx context.Context) secrets.Status {
	return f.status
}

func prepareStaleManagedState(t *testing.T, ctx context.Context, database *sql.DB, resolver config.PathResolver) (storePath, sourcePath, targetPath string) {
	t.Helper()
	storePath = filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	commonRoot := filepath.Join(storePath, "profiles", "common")
	sourcePath = filepath.Join(commonRoot, "files", "dot-pi", "agent", "settings.json")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("{\n  \"packages\": [\n    \"a\",\n    \"b\"\n  ]\n}\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	manifest := `version: 1
name: common
files:
  - id: pi-settings
    source: files/dot-pi/agent/settings.json
    target: ~/.pi/agent/settings.json
    mode: merge
    format: json
skills: []
ignore: []
merge_rules: {}
targets: {}
`
	if err := os.WriteFile(filepath.Join(commonRoot, "manifest.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	targetPath = filepath.Join(resolver.HomeDir, ".pi", "agent", "settings.json")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("{\"packages\":[\"a\",\"b\"]}\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := activation.PutManagedTarget(ctx, database, activation.ManagedTarget{TargetPath: targetPath, SourcePath: sourcePath, Mode: "copy", ContentHash: "stale-hash", LayerKind: "common", LayerName: "common", LastAppliedAt: "2026-05-18T00:00:00Z"}); err != nil {
		t.Fatalf("PutManagedTarget() error = %v", err)
	}
	return storePath, sourcePath, targetPath
}

func openDoctorTestDB(t *testing.T, ctx context.Context, path string) *sql.DB {
	t.Helper()
	database, err := db.Bootstrap(ctx, path)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	return database
}

func hasCheck(report Report, code string) bool {
	_, ok := findCheck(report, code)
	return ok
}

func findCheck(report Report, code string) (Check, bool) {
	for _, check := range report.Checks {
		if check.Code == code {
			return check, true
		}
	}
	return Check{}, false
}
