package activation

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRebaseManagedTargetSourcePathsUpdatesSourceAndMetadataSources(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	oldRoot := filepath.Join(t.TempDir(), "old-store")
	newRoot := filepath.Join(t.TempDir(), "new-store")
	target := filepath.Join(t.TempDir(), "target.json")
	oldSource := filepath.Join(oldRoot, "profiles", "work", "core", "files", "target.json")
	oldLayerSource := filepath.Join(oldRoot, "profiles", "common", "files", "target.json")
	outsideSource := filepath.Join(t.TempDir(), "outside.json")
	metadata := map[string]any{
		"sources": []map[string]any{
			{"path": oldLayerSource, "layer_name": "common"},
			{"path": outsideSource, "layer_name": "external"},
		},
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Marshal(metadata) error = %v", err)
	}
	if err := PutManagedTarget(ctx, database, ManagedTarget{
		TargetPath:    target,
		SourcePath:    oldSource,
		Mode:          string(OperationMerge),
		ContentHash:   "hash",
		LayerKind:     "core",
		LayerName:     "work",
		LastAppliedAt: time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		MetadataJSON:  string(metadataJSON),
	}); err != nil {
		t.Fatalf("PutManagedTarget() error = %v", err)
	}

	changed, err := RebaseManagedTargetSourcePaths(ctx, database, oldRoot, newRoot)
	if err != nil {
		t.Fatalf("RebaseManagedTargetSourcePaths() error = %v", err)
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}
	record, found, err := GetManagedTarget(ctx, database, target)
	if err != nil || !found {
		t.Fatalf("GetManagedTarget() found=%v err=%v", found, err)
	}
	wantSource := filepath.Join(newRoot, "profiles", "work", "core", "files", "target.json")
	if record.SourcePath != wantSource {
		t.Fatalf("SourcePath = %q, want %q", record.SourcePath, wantSource)
	}
	var got map[string][]Source
	if err := json.Unmarshal([]byte(record.MetadataJSON), &got); err != nil {
		t.Fatalf("metadata JSON invalid: %v\n%s", err, record.MetadataJSON)
	}
	wantLayerSource := filepath.Join(newRoot, "profiles", "common", "files", "target.json")
	if got["sources"][0].Path != wantLayerSource {
		t.Fatalf("metadata source[0] = %q, want %q", got["sources"][0].Path, wantLayerSource)
	}
	if got["sources"][1].Path != outsideSource {
		t.Fatalf("outside metadata source changed to %q", got["sources"][1].Path)
	}
}

func TestRebaseManagedTargetSourcePathsRejectsCaseVariantNestedRoots(t *testing.T) {
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	oldRoot := filepath.Join(root, "Store")
	newRoot := filepath.Join(root, "store", "nested")
	_, err := RebaseManagedTargetSourcePaths(context.Background(), database, oldRoot, newRoot)
	if err == nil || !strings.Contains(err.Error(), "must not be nested") {
		t.Fatalf("RebaseManagedTargetSourcePaths(case nested) error = %v", err)
	}
}

func TestRebasePathUnderRootPreservesRelativeCase(t *testing.T) {
	oldRoot := filepath.Join(t.TempDir(), "Store")
	newRoot := filepath.Join(t.TempDir(), "NewStore")
	pathValue := filepath.Join(filepath.Dir(oldRoot), "store", "Profiles", "Work", "File.JSON")
	got, ok, err := rebasePathUnderRoot(pathValue, oldRoot, newRoot)
	if err != nil || !ok {
		t.Fatalf("rebasePathUnderRoot() got=%q ok=%v err=%v", got, ok, err)
	}
	want := filepath.Join(newRoot, "Profiles", "Work", "File.JSON")
	if got != want {
		t.Fatalf("rebased path = %q, want %q", got, want)
	}
}

func TestRebaseManagedTargetSourcePathsRejectsNestedRoots(t *testing.T) {
	database := activationDB(t)
	defer database.Close()
	oldRoot := filepath.Join(t.TempDir(), "store")
	newRoot := filepath.Join(oldRoot, "nested")
	_, err := RebaseManagedTargetSourcePaths(context.Background(), database, oldRoot, newRoot)
	if err == nil {
		t.Fatal("RebaseManagedTargetSourcePaths(nested) error = nil, want error")
	}
}
