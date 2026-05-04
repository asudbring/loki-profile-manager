package activation

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRestoreDryRunPlanFileDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	entryPath := filepath.Join(root, "snap", "entries", "000")
	writeFile(t, target, "new")
	writeFile(t, entryPath, "old")
	oldHash, err := HashPath(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	newHash, err := HashPath(target)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{SnapshotID: "snap", Path: filepath.Join(root, "snap"), Targets: []SnapshotEntry{{TargetPath: target, Kind: "file", Hash: oldHash, SnapshotPath: entryPath}}}
	plan, err := BuildRestoreDryRunPlan(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("BuildRestoreDryRunPlan() error = %v", err)
	}
	if len(plan.Targets) != 1 || plan.Targets[0].Action != RestoreActionRestoreFile || plan.Targets[0].CurrentHash != newHash {
		t.Fatalf("plan = %+v", plan)
	}
	if got := readFile(t, target); got != "new" {
		t.Fatalf("target changed to %q", got)
	}
}

func TestBuildRestoreDryRunPlanMissingEntryRemoveOrSkip(t *testing.T) {
	root := t.TempDir()
	created := filepath.Join(root, "created.txt")
	writeFile(t, created, "created")
	hash, err := HashPath(created)
	if err != nil {
		t.Fatal(err)
	}
	skip := filepath.Join(root, "already-gone.txt")
	snapshot := Snapshot{SnapshotID: "snap", Targets: []SnapshotEntry{
		{TargetPath: created, Kind: "missing", ExpectedHash: hash},
		{TargetPath: skip, Kind: "missing", ExpectedHash: "unused"},
	}}
	plan, err := BuildRestoreDryRunPlan(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("BuildRestoreDryRunPlan() error = %v", err)
	}
	if len(plan.Targets) != 2 || plan.Targets[0].Action != RestoreActionRemoveCreatedTarget || plan.Targets[1].Action != RestoreActionSkipMissingTargetAbsent {
		t.Fatalf("plan = %+v", plan.Targets)
	}
	if got := readFile(t, created); got != "created" {
		t.Fatalf("created target changed to %q", got)
	}
}

func TestBuildRestoreDryRunPlanRejectsSnapshotEntryOutsideSnapshot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	outside := filepath.Join(root, "outside-entry")
	writeFile(t, target, "new")
	writeFile(t, outside, "old")
	snapshot := Snapshot{SnapshotID: "snap", Path: filepath.Join(root, "snap"), Targets: []SnapshotEntry{{TargetPath: target, Kind: "file", SnapshotPath: outside}}}
	plan, err := BuildRestoreDryRunPlan(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("BuildRestoreDryRunPlan() error = %v", err)
	}
	if !strings.Contains(strings.Join(plan.Targets[0].Warnings, "; "), "outside snapshot") {
		t.Fatalf("warnings = %+v", plan.Targets[0].Warnings)
	}
}

func TestBuildRestoreDryRunPlanTargetFiltersOneExactPath(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	firstEntry := filepath.Join(root, "snap", "entries", "000")
	secondEntry := filepath.Join(root, "snap", "entries", "001")
	writeFile(t, first, "first-new")
	writeFile(t, second, "second-new")
	writeFile(t, firstEntry, "first-old")
	writeFile(t, secondEntry, "second-old")
	firstHash, err := HashPath(firstEntry)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := HashPath(secondEntry)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{SnapshotID: "snap", Path: filepath.Join(root, "snap"), Targets: []SnapshotEntry{
		{TargetPath: first, Kind: "file", Hash: firstHash, SnapshotPath: firstEntry},
		{TargetPath: second, Kind: "file", Hash: secondHash, SnapshotPath: secondEntry},
	}}
	full, err := BuildRestoreDryRunPlan(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("BuildRestoreDryRunPlan() error = %v", err)
	}
	filtered, err := BuildRestoreDryRunPlanWithOptions(context.Background(), snapshot, RestorePlanOptions{Target: second})
	if err != nil {
		t.Fatalf("BuildRestoreDryRunPlanWithOptions() error = %v", err)
	}
	if len(filtered.Targets) != 1 || filtered.Targets[0].Entry.TargetPath != second || filtered.TargetFilter != second {
		t.Fatalf("filtered = %+v", filtered)
	}
	if filtered.Fingerprint == full.Fingerprint {
		t.Fatal("filtered fingerprint equals full fingerprint")
	}
}

func TestBuildRestoreDryRunPlanTargetMissing(t *testing.T) {
	snapshot := Snapshot{SnapshotID: "snap", Targets: []SnapshotEntry{{TargetPath: filepath.Join(t.TempDir(), "target"), Kind: "missing"}}}
	_, err := BuildRestoreDryRunPlanWithOptions(context.Background(), snapshot, RestorePlanOptions{Target: filepath.Join(t.TempDir(), "other")})
	if err == nil || !strings.Contains(err.Error(), "target not found") {
		t.Fatalf("BuildRestoreDryRunPlanWithOptions() error = %v", err)
	}
}

func TestBuildRestoreDryRunPlanTargetDoesNotMatchDirectoryChild(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dir")
	snapshot := Snapshot{SnapshotID: "snap", Targets: []SnapshotEntry{{TargetPath: dir, Kind: "directory"}}}
	_, err := BuildRestoreDryRunPlanWithOptions(context.Background(), snapshot, RestorePlanOptions{Target: filepath.Join(dir, "child.txt")})
	if err == nil || !strings.Contains(err.Error(), "target not found") {
		t.Fatalf("BuildRestoreDryRunPlanWithOptions() error = %v", err)
	}
}

func TestBuildRestoreDryRunPlanSensitiveTargetSkipsHash(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, ".ssh", "id_ed25519")
	writeFile(t, target, "not-a-real-key")
	snapshot := Snapshot{SnapshotID: "snap", Targets: []SnapshotEntry{{TargetPath: target, Kind: "missing", ExpectedHash: "hash"}}}
	plan, err := BuildRestoreDryRunPlan(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("BuildRestoreDryRunPlan() error = %v", err)
	}
	if !plan.Targets[0].SensitivePath || plan.Targets[0].CurrentHash != "" {
		t.Fatalf("sensitive target = %+v", plan.Targets[0])
	}
	if !strings.Contains(strings.Join(plan.Targets[0].Warnings, "; "), "sensitive") {
		t.Fatalf("warnings = %+v", plan.Targets[0].Warnings)
	}
}

func TestBuildRestoreDryRunPlanUnknownKindWarns(t *testing.T) {
	snapshot := Snapshot{SnapshotID: "snap", Targets: []SnapshotEntry{{TargetPath: filepath.Join(t.TempDir(), "target"), Kind: "mystery"}}}
	plan, err := BuildRestoreDryRunPlan(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("BuildRestoreDryRunPlan() error = %v", err)
	}
	if len(plan.Targets) != 1 || plan.Targets[0].Action != RestoreActionUnknown || len(plan.Targets[0].Warnings) == 0 {
		t.Fatalf("plan = %+v", plan.Targets)
	}
}
