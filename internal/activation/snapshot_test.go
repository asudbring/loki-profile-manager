package activation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/allensu/loki-profile-manager/internal/config"
)

func TestCreateSnapshotMetadataAndRetention(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	writeFile(t, target, "old")
	plan := Plan{Profile: "work", Buckets: []string{"azure"}, Operations: []Operation{{Type: OperationCopy, TargetPath: target}}}
	snapshotRoot := filepath.Join(root, "snapshots")
	times := []time.Time{
		time.Date(2026, 5, 3, 1, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 3, 2, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 3, 3, 0, 0, 0, time.UTC),
	}
	for _, ts := range times {
		snapshot, err := CreateSnapshot(ctx, CreateSnapshotRequest{
			Database:              database,
			SnapshotRoot:          snapshotRoot,
			MachineID:             "machine-1",
			Plan:                  plan,
			PreviousActiveProfile: "dev",
			PreviousActiveBuckets: []string{"old"},
			Now:                   func() time.Time { return ts },
			Keep:                  2,
		})
		if err != nil {
			t.Fatalf("CreateSnapshot() error = %v", err)
		}
		if snapshot.PreviousActiveProfile != "dev" || len(snapshot.Targets) != 1 || snapshot.Targets[0].SnapshotPath == "" {
			t.Fatalf("snapshot = %+v", snapshot)
		}
	}
	entries, err := os.ReadDir(snapshotRoot)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("snapshot count = %d, want 2", len(entries))
	}
}

func TestRollbackRemovesCreatedTargets(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	target := filepath.Join(root, "created.txt")
	snapshot := Snapshot{Path: filepath.Join(root, "snap"), PreviousActiveProfile: "dev", PreviousActiveBuckets: []string{"old"}, Targets: []SnapshotEntry{{TargetPath: target, Kind: "missing"}}}
	writeFile(t, target, "new")
	if err := Rollback(ctx, RollbackRequest{Database: database, Snapshot: snapshot}); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target still exists or stat err = %v", err)
	}
}

func TestExecuteCreatesSnapshot(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	target := filepath.Join(root, "target.txt")
	writeFile(t, source, "new")
	plan := Plan{Profile: "work", Operations: []Operation{{ID: "copy", Type: OperationCopy, SourcePath: source, TargetPath: target, LayerName: "work", LayerKind: "core"}}}
	result, err := Execute(ctx, ExecuteRequest{Database: database, LocalPaths: config.LocalPaths{SnapshotDir: filepath.Join(root, "snapshots")}, Plan: plan})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Snapshot.SnapshotID == "" || result.Changed != 1 {
		t.Fatalf("result = %+v", result)
	}
	if got := readFile(t, target); got != "new" {
		t.Fatalf("target = %q", got)
	}
}
