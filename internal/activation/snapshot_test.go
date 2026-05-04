package activation

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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

func TestListSnapshotsFromDatabase(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	writeFile(t, target, "old")
	plan := Plan{Profile: "work", Operations: []Operation{{Type: OperationCopy, TargetPath: target}}}
	snapshot, err := CreateSnapshot(ctx, CreateSnapshotRequest{Database: database, SnapshotRoot: filepath.Join(root, "snapshots"), MachineID: "machine-1", Plan: plan, PreviousActiveProfile: "dev", PreviousActiveBuckets: []string{"old"}})
	if err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}
	summaries, err := ListSnapshots(ctx, database, filepath.Join(root, "snapshots"))
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0].SnapshotID != snapshot.SnapshotID || summaries[0].TargetCount != 1 || !summaries[0].Exists {
		t.Fatalf("summaries = %+v", summaries)
	}
	if summaries[0].PreviousActiveProfile != "dev" || len(summaries[0].PreviousActiveBuckets) != 1 || summaries[0].PreviousActiveBuckets[0] != "old" {
		t.Fatalf("previous state = %+v", summaries[0])
	}
}

func TestListSnapshotsFallsBackToMetadataFiles(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	writeFile(t, target, "old")
	snapshotRoot := filepath.Join(root, "snapshots")
	snapshot, err := CreateSnapshot(ctx, CreateSnapshotRequest{SnapshotRoot: snapshotRoot, Plan: Plan{Profile: "work", Operations: []Operation{{Type: OperationCopy, TargetPath: target}}}})
	if err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}
	summaries, err := ListSnapshots(ctx, database, snapshotRoot)
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0].SnapshotID != snapshot.SnapshotID || summaries[0].TargetCount != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}
}

func TestListSnapshotsMarksStaleDatabasePath(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	writeFile(t, target, "old")
	snapshotRoot := filepath.Join(root, "snapshots")
	snapshot, err := CreateSnapshot(ctx, CreateSnapshotRequest{Database: database, SnapshotRoot: snapshotRoot, Plan: Plan{Profile: "work", Operations: []Operation{{Type: OperationCopy, TargetPath: target}}}})
	if err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}
	if err := os.RemoveAll(snapshot.Path); err != nil {
		t.Fatal(err)
	}
	summaries, err := ListSnapshots(ctx, database, snapshotRoot)
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0].SnapshotID != snapshot.SnapshotID || summaries[0].Exists || summaries[0].TargetCount != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}
}

func TestLoadSnapshotFromMetadata(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	writeFile(t, target, "old")
	snapshotRoot := filepath.Join(root, "snapshots")
	snapshot, err := CreateSnapshot(ctx, CreateSnapshotRequest{Database: database, SnapshotRoot: snapshotRoot, Plan: Plan{Profile: "work", Operations: []Operation{{Type: OperationCopy, TargetPath: target}}}})
	if err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}
	loaded, err := LoadSnapshot(ctx, database, snapshotRoot, snapshot.SnapshotID)
	if err != nil {
		t.Fatalf("LoadSnapshot() error = %v", err)
	}
	if loaded.SnapshotID != snapshot.SnapshotID || len(loaded.Targets) != 1 || loaded.Targets[0].TargetPath != target {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestRollbackRemovesCreatedTargets(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	target := filepath.Join(root, "created.txt")
	writeFile(t, target, "new")
	hash, err := HashPath(target)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{Path: filepath.Join(root, "snap"), PreviousActiveProfile: "dev", PreviousActiveBuckets: []string{"old"}, Targets: []SnapshotEntry{{TargetPath: target, Kind: "missing", ExpectedHash: hash, ExpectedMode: info.Mode().String()}}}
	if err := Rollback(ctx, RollbackRequest{Database: database, Snapshot: snapshot}); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target still exists or stat err = %v", err)
	}
}

func TestRollbackRefusesToRemoveChangedCreatedTarget(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	target := filepath.Join(root, "created.txt")
	writeFile(t, target, "new")
	hash, err := HashPath(target)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, target, "user data")
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{Path: filepath.Join(root, "snap"), Targets: []SnapshotEntry{{TargetPath: target, Kind: "missing", ExpectedHash: hash, ExpectedMode: info.Mode().String()}}}
	if err := Rollback(ctx, RollbackRequest{Database: database, Snapshot: snapshot}); err == nil {
		t.Fatal("Rollback() error = nil, want conflict")
	}
	if got := readFile(t, target); got != "user data" {
		t.Fatalf("target = %q", got)
	}
}

func TestRollbackRefusesToRemoveChmodCreatedTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows chmod does not change file mode bits enough for this rollback conflict test")
	}
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	target := filepath.Join(root, "created.txt")
	writeFile(t, target, "new")
	hash, err := HashPath(target)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{Path: filepath.Join(root, "snap"), Targets: []SnapshotEntry{{TargetPath: target, Kind: "missing", ExpectedHash: hash, ExpectedMode: info.Mode().String()}}}
	if err := Rollback(ctx, RollbackRequest{Database: database, Snapshot: snapshot}); err == nil {
		t.Fatal("Rollback() error = nil, want mode conflict")
	}
	if got := readFile(t, target); got != "new" {
		t.Fatalf("target = %q", got)
	}
}

func TestRollbackSkipsDBRestoreWhenFilesystemFails(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	target := filepath.Join(root, "created.txt")
	writeFile(t, target, "new")
	if err := SetActiveState(ctx, database, "new", []string{"bucket"}); err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{Path: filepath.Join(root, "snap"), PreviousActiveProfile: "old", Targets: []SnapshotEntry{{TargetPath: target, Kind: "missing", ExpectedHash: "different"}}}
	if err := Rollback(ctx, RollbackRequest{Database: database, Snapshot: snapshot}); err == nil {
		t.Fatal("Rollback() error = nil, want filesystem conflict")
	}
	var active string
	if err := database.QueryRowContext(ctx, `SELECT value FROM kv_state WHERE key = 'active_profile'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != "new" {
		t.Fatalf("active profile = %q, want new", active)
	}
}

func TestRollbackRestoresManagedTargetRows(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	target := filepath.Join(root, "target.txt")
	writeFile(t, source, "new")
	writeFile(t, target, "old")
	oldOp := Operation{ID: "old", Type: OperationCopy, SourcePath: source, TargetPath: target, LayerName: "old", LayerKind: "core"}
	if err := UpsertManagedTarget(ctx, database, oldOp, "old-hash", time.Now()); err != nil {
		t.Fatalf("UpsertManagedTarget(old) error = %v", err)
	}
	plan := Plan{Profile: "work", Operations: []Operation{{ID: "new", Type: OperationCopy, SourcePath: source, TargetPath: target, LayerName: "new", LayerKind: "core"}}}
	snapshot, err := CreateSnapshot(ctx, CreateSnapshotRequest{Database: database, SnapshotRoot: filepath.Join(root, "snapshots"), Plan: plan})
	if err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}
	if err := UpsertManagedTarget(ctx, database, plan.Operations[0], "new-hash", time.Now()); err != nil {
		t.Fatalf("UpsertManagedTarget(new) error = %v", err)
	}
	if err := Rollback(ctx, RollbackRequest{Database: database, Snapshot: snapshot}); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	record, found, err := GetManagedTarget(ctx, database, target)
	if err != nil || !found {
		t.Fatalf("GetManagedTarget() = %+v, %v, %v", record, found, err)
	}
	if record.LayerName != "old" || record.ContentHash != "old-hash" {
		t.Fatalf("record = %+v", record)
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
	if result.Snapshot.Targets[0].ExpectedHash == "" || result.Snapshot.Targets[0].ExpectedMode == "" {
		t.Fatalf("snapshot target did not record created target hash/mode: %+v", result.Snapshot.Targets[0])
	}
	loaded, err := LoadSnapshot(ctx, database, filepath.Join(root, "snapshots"), result.Snapshot.SnapshotID)
	if err != nil {
		t.Fatalf("LoadSnapshot() error = %v", err)
	}
	if loaded.Targets[0].ExpectedHash == "" || loaded.Targets[0].ExpectedMode == "" {
		t.Fatalf("persisted snapshot target did not record created target hash/mode: %+v", loaded.Targets[0])
	}
	if got := readFile(t, target); got != "new" {
		t.Fatalf("target = %q", got)
	}
}
