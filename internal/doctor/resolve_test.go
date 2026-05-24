package doctor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/db"
	"github.com/asudbring/loki-profile-manager/internal/store"
)

func TestFindSwitchBlockersDetectsMergeDivergence(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	database, err := db.Bootstrap(ctx, paths.DBPath)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	defer database.Close()

	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}

	// Create a common profile with a merge-mode settings.json.
	commonDir := filepath.Join(storePath, "profiles", "common")
	if err := os.MkdirAll(filepath.Join(commonDir, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	storeContent := `{"defaultModel": "gpt-5.5", "packages": ["npm:context-mode"]}`
	if err := os.WriteFile(filepath.Join(commonDir, "files", "settings.json"), []byte(storeContent), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `version: 1
name: common
files:
    - id: settings-json
      source: files/settings.json
      target: ` + filepath.Join(home, "settings.json") + `
      mode: merge
      format: json
`
	if err := os.WriteFile(filepath.Join(commonDir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write the local target with diverged content.
	localContent := `{"defaultModel": "claude-opus-4.6", "packages": ["npm:context-mode"]}`
	targetPath := filepath.Join(home, "settings.json")
	if err := os.WriteFile(targetPath, []byte(localContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Insert a managed target record with the STORE hash (simulating a previous apply).
	storeHash := activation.HashBytes([]byte(storeContent))
	record := activation.ManagedTarget{
		TargetPath:    targetPath,
		SourcePath:    "",
		Mode:          "merge",
		ContentHash:   storeHash,
		LayerKind:     "common",
		LayerName:     "common",
		LastAppliedAt: time.Now().UTC().Format(time.RFC3339),
		MetadataJSON:  `{"sources":[{"path":"` + filepath.Join(commonDir, "files", "settings.json") + `","layer_name":"common","layer_kind":"common","file_id":"settings-json","order":0}]}`,
	}
	if err := activation.PutManagedTarget(ctx, database, record); err != nil {
		t.Fatalf("PutManagedTarget() error = %v", err)
	}

	// FindSwitchBlockers should detect this.
	blockers, err := FindSwitchBlockers(ctx, database, storePath, resolver)
	if err != nil {
		t.Fatalf("FindSwitchBlockers() error = %v", err)
	}
	if len(blockers) != 1 {
		t.Fatalf("FindSwitchBlockers() = %d blockers, want 1", len(blockers))
	}
	if blockers[0].TargetPath != targetPath {
		t.Errorf("blocker.TargetPath = %q, want %q", blockers[0].TargetPath, targetPath)
	}
	if blockers[0].Format != "json" {
		t.Errorf("blocker.Format = %q, want %q", blockers[0].Format, "json")
	}
	if len(blockers[0].AvailableLayers) != 1 {
		t.Fatalf("blocker.AvailableLayers = %d, want 1", len(blockers[0].AvailableLayers))
	}
	if blockers[0].AvailableLayers[0].Name != "common" {
		t.Errorf("layer name = %q, want %q", blockers[0].AvailableLayers[0].Name, "common")
	}
}

func TestResolveBlockerPromotesContentAndRepairsState(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	database, err := db.Bootstrap(ctx, paths.DBPath)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	defer database.Close()

	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}

	// Set up store source.
	commonDir := filepath.Join(storePath, "profiles", "common")
	if err := os.MkdirAll(filepath.Join(commonDir, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	storeContent := `{"defaultModel": "gpt-5.5"}`
	sourcePath := filepath.Join(commonDir, "files", "settings.json")
	if err := os.WriteFile(sourcePath, []byte(storeContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Local target with different content.
	localContent := `{"defaultModel": "claude-opus-4.6"}`
	targetPath := filepath.Join(home, "settings.json")
	if err := os.WriteFile(targetPath, []byte(localContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Insert managed target record.
	storeHash := activation.HashBytes([]byte(storeContent))
	record := activation.ManagedTarget{
		TargetPath:    targetPath,
		Mode:          "merge",
		ContentHash:   storeHash,
		LayerKind:     "common",
		LayerName:     "common",
		LastAppliedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := activation.PutManagedTarget(ctx, database, record); err != nil {
		t.Fatalf("PutManagedTarget() error = %v", err)
	}

	// Resolve the blocker.
	blocker := ResolvableBlocker{
		Change: activation.CaptureChange{
			TargetPath: targetPath,
			Mode:       "merge",
			Status:     activation.CaptureUnsupported,
		},
		Format:     "json",
		TargetPath: targetPath,
		AvailableLayers: []LayerChoice{{
			Name:       "common",
			Kind:       "common",
			SourcePath: sourcePath,
			RootDir:    commonDir,
		}},
	}

	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	err = ResolveBlocker(ctx, ResolveBlockerOptions{
		Blocker:     blocker,
		ChosenLayer: blocker.AvailableLayers[0],
		Database:    database,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("ResolveBlocker() error = %v", err)
	}

	// Verify source file was updated with formatted local content.
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(sourceBytes, &parsed); err != nil {
		t.Fatalf("parse source: %v", err)
	}
	if got := parsed["defaultModel"]; got != "claude-opus-4.6" {
		t.Errorf("source defaultModel = %v, want claude-opus-4.6", got)
	}

	// Verify managed target record was updated.
	updated, found, err := activation.GetManagedTarget(ctx, database, targetPath)
	if err != nil {
		t.Fatalf("GetManagedTarget() error = %v", err)
	}
	if !found {
		t.Fatal("managed target record not found after resolve")
	}
	// Hash should match the local target's hash.
	expectedHash, _ := activation.HashPath(targetPath)
	if updated.ContentHash != expectedHash {
		t.Errorf("record.ContentHash = %q, want %q", updated.ContentHash, expectedHash)
	}
	if updated.LastAppliedAt != "2026-05-24T12:00:00Z" {
		t.Errorf("record.LastAppliedAt = %q, want 2026-05-24T12:00:00Z", updated.LastAppliedAt)
	}
	// Metadata should contain doctor_resolve marker.
	var meta map[string]any
	if err := json.Unmarshal([]byte(updated.MetadataJSON), &meta); err != nil {
		t.Fatalf("parse metadata: %v", err)
	}
	if meta["doctor_resolve"] != true {
		t.Error("metadata missing doctor_resolve=true")
	}
	if meta["promoted_to_layer"] != "common" {
		t.Errorf("metadata promoted_to_layer = %v, want common", meta["promoted_to_layer"])
	}
}

func TestFindSwitchBlockersNoneWhenAligned(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	database, err := db.Bootstrap(ctx, paths.DBPath)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	defer database.Close()

	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}

	// Common profile with settings.json.
	commonDir := filepath.Join(storePath, "profiles", "common")
	if err := os.MkdirAll(filepath.Join(commonDir, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"defaultModel": "gpt-5.5"}`
	if err := os.WriteFile(filepath.Join(commonDir, "files", "settings.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `version: 1
name: common
files:
    - id: settings-json
      source: files/settings.json
      target: ` + filepath.Join(home, "settings.json") + `
      mode: merge
      format: json
`
	if err := os.WriteFile(filepath.Join(commonDir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	// Local target matches store — no divergence.
	targetPath := filepath.Join(home, "settings.json")
	if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	hash := activation.HashBytes([]byte(content))
	record := activation.ManagedTarget{
		TargetPath:    targetPath,
		Mode:          "merge",
		ContentHash:   hash,
		LayerKind:     "common",
		LayerName:     "common",
		LastAppliedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := activation.PutManagedTarget(ctx, database, record); err != nil {
		t.Fatalf("PutManagedTarget() error = %v", err)
	}

	blockers, err := FindSwitchBlockers(ctx, database, storePath, resolver)
	if err != nil {
		t.Fatalf("FindSwitchBlockers() error = %v", err)
	}
	if len(blockers) != 0 {
		t.Fatalf("FindSwitchBlockers() = %d blockers, want 0", len(blockers))
	}
}

func TestDoctorReportIncludesSwitchBlockerCheck(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	database, err := db.Bootstrap(ctx, paths.DBPath)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	defer database.Close()

	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}

	// Set up a diverged merge target.
	commonDir := filepath.Join(storePath, "profiles", "common")
	if err := os.MkdirAll(filepath.Join(commonDir, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	storeContent := `{"key": "store"}`
	if err := os.WriteFile(filepath.Join(commonDir, "files", "test.json"), []byte(storeContent), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `version: 1
name: common
files:
    - id: test-json
      source: files/test.json
      target: ` + filepath.Join(home, "test.json") + `
      mode: merge
      format: json
`
	if err := os.WriteFile(filepath.Join(commonDir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	// Local target diverged.
	targetPath := filepath.Join(home, "test.json")
	if err := os.WriteFile(targetPath, []byte(`{"key": "local"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	storeHash := activation.HashBytes([]byte(storeContent))
	record := activation.ManagedTarget{
		TargetPath:    targetPath,
		Mode:          "merge",
		ContentHash:   storeHash,
		LayerKind:     "common",
		LayerName:     "common",
		LastAppliedAt: time.Now().UTC().Format(time.RFC3339),
		MetadataJSON:  `{"sources":[{"path":"` + filepath.Join(commonDir, "files", "test.json") + `","layer_name":"common","layer_kind":"common","file_id":"test-json","order":0}]}`,
	}
	if err := activation.PutManagedTarget(ctx, database, record); err != nil {
		t.Fatal(err)
	}

	report := Run(ctx, Request{
		Version:   "test",
		StorePath: storePath,
		Resolver:  resolver,
		Database:  database,
		Now:       func() time.Time { return time.Now() },
	})

	if !hasCheck(report, "switch_blocker.capture_unsupported") {
		t.Error("report missing switch_blocker.capture_unsupported check")
		for _, c := range report.Checks {
			t.Logf("check: %s %s %s", c.Severity, c.Code, c.Message)
		}
	}
	check, _ := findCheck(report, "switch_blocker.capture_unsupported")
	if check.Severity != SeverityBlocking {
		t.Errorf("check severity = %q, want blocking", check.Severity)
	}
	if check.Path != targetPath {
		t.Errorf("check path = %q, want %q", check.Path, targetPath)
	}
	if check.Remediation == "" || !contains(check.Remediation, "--resolve-blockers") {
		t.Errorf("check remediation = %q, want to contain --resolve-blockers", check.Remediation)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
