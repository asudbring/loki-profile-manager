package activation

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/config"
)

func TestRestoreRestoresFileAndActiveState(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	writeFile(t, target, "old")
	snapshot, err := CreateSnapshot(ctx, CreateSnapshotRequest{
		Database:              database,
		SnapshotRoot:          filepath.Join(root, "snapshots"),
		Plan:                  Plan{Profile: "work", Operations: []Operation{{Type: OperationCopy, TargetPath: target}}},
		PreviousActiveProfile: "dev",
		PreviousActiveBuckets: []string{"old-bucket"},
	})
	if err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}
	writeFile(t, target, "new")
	if err := SetActiveState(ctx, database, "work", []string{"new-bucket"}); err != nil {
		t.Fatalf("SetActiveState() error = %v", err)
	}
	restored, err := Restore(ctx, RestoreRequest{Database: database, LocalPaths: config.LocalPaths{SnapshotDir: filepath.Join(root, "snapshots")}, Snapshot: snapshot, PreviousActiveProfile: "work", PreviousActiveBuckets: []string{"new-bucket"}})
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if restored.Changed != 1 || restored.PreRestoreSnapshot.SnapshotID == "" || restored.PreRestoreSnapshot.SourceSnapshotID != snapshot.SnapshotID {
		t.Fatalf("restored = %+v", restored)
	}
	if got := readFile(t, target); got != "old" {
		t.Fatalf("target = %q", got)
	}
	profile, buckets, ok, err := activeStateForTest(ctx, database)
	if err != nil || !ok || profile != "dev" || len(buckets) != 1 || buckets[0] != "old-bucket" {
		t.Fatalf("active state = %q %+v %v %v", profile, buckets, ok, err)
	}
}

func TestRestoreRemovesCreatedTarget(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	target := filepath.Join(root, "created.txt")
	writeFile(t, source, "new")
	executed, err := Execute(ctx, ExecuteRequest{Database: database, LocalPaths: config.LocalPaths{SnapshotDir: filepath.Join(root, "snapshots")}, Plan: Plan{Profile: "work", Operations: []Operation{{Type: OperationCopy, SourcePath: source, TargetPath: target}}}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	restored, err := Restore(ctx, RestoreRequest{Database: database, LocalPaths: config.LocalPaths{SnapshotDir: filepath.Join(root, "snapshots")}, Snapshot: executed.Snapshot})
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if restored.Changed != 1 {
		t.Fatalf("restored changed = %d", restored.Changed)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after restore or stat err = %v", err)
	}
}

func TestRestoreTargetOnlyChangesSelectedPath(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	writeFile(t, first, "first-old")
	writeFile(t, second, "second-old")
	snapshot, err := CreateSnapshot(ctx, CreateSnapshotRequest{Database: database, SnapshotRoot: filepath.Join(root, "snapshots"), Plan: Plan{Profile: "work", Operations: []Operation{{Type: OperationCopy, TargetPath: first}, {Type: OperationCopy, TargetPath: second}}}, PreviousActiveProfile: "dev"})
	if err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}
	writeFile(t, first, "first-new")
	writeFile(t, second, "second-new")
	restored, err := Restore(ctx, RestoreRequest{Database: database, LocalPaths: config.LocalPaths{SnapshotDir: filepath.Join(root, "snapshots")}, Snapshot: snapshot, Target: first})
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if restored.Changed != 1 || len(restored.Plan.Targets) != 1 || restored.Plan.Targets[0].Entry.TargetPath != first {
		t.Fatalf("restored = %+v", restored)
	}
	if got := readFile(t, first); got != "first-old" {
		t.Fatalf("first = %q", got)
	}
	if got := readFile(t, second); got != "second-new" {
		t.Fatalf("second = %q", got)
	}
}

func TestRestoreTargetDoesNotRestoreActiveState(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	writeFile(t, target, "old")
	snapshot, err := CreateSnapshot(ctx, CreateSnapshotRequest{Database: database, SnapshotRoot: filepath.Join(root, "snapshots"), Plan: Plan{Profile: "work", Operations: []Operation{{Type: OperationCopy, TargetPath: target}}}, PreviousActiveProfile: "dev", PreviousActiveBuckets: []string{"old"}})
	if err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}
	writeFile(t, target, "new")
	if err := SetActiveState(ctx, database, "work", []string{"new"}); err != nil {
		t.Fatalf("SetActiveState() error = %v", err)
	}
	if _, err := Restore(ctx, RestoreRequest{Database: database, LocalPaths: config.LocalPaths{SnapshotDir: filepath.Join(root, "snapshots")}, Snapshot: snapshot, Target: target}); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	profile, buckets, ok, err := activeStateForTest(ctx, database)
	if err != nil || !ok || profile != "work" || len(buckets) != 1 || buckets[0] != "new" {
		t.Fatalf("active state = %q %+v %v %v", profile, buckets, ok, err)
	}
}

func TestRestoreBlocksSensitiveTarget(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	target := filepath.Join(root, ".ssh", "id_ed25519")
	writeFile(t, target, "not-a-real-key")
	hash, err := HashPath(target)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{SnapshotID: "snap", Path: filepath.Join(root, "snap"), Targets: []SnapshotEntry{{TargetPath: target, Kind: "missing", ExpectedHash: hash}}}
	if _, err := Restore(ctx, RestoreRequest{Database: database, LocalPaths: config.LocalPaths{SnapshotDir: filepath.Join(root, "snapshots")}, Snapshot: snapshot}); err == nil {
		t.Fatal("Restore() error = nil, want sensitive blocker")
	}
	if got := readFile(t, target); got != "not-a-real-key" {
		t.Fatalf("target changed to %q", got)
	}
}

func activeStateForTest(ctx context.Context, database *sql.DB) (string, []string, bool, error) {
	var profile string
	if err := database.QueryRowContext(ctx, `SELECT value FROM kv_state WHERE key = 'active_profile'`).Scan(&profile); err != nil {
		return "", nil, false, err
	}
	var bucketsRaw string
	if err := database.QueryRowContext(ctx, `SELECT value FROM kv_state WHERE key = 'active_buckets'`).Scan(&bucketsRaw); err != nil {
		return "", nil, false, err
	}
	var buckets []string
	if err := json.Unmarshal([]byte(bucketsRaw), &buckets); err != nil {
		return "", nil, false, err
	}
	return profile, buckets, true, nil
}
