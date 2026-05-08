package activation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildCapturePlanDetectsCopyLocalOnlyChange(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	source := filepath.Join(root, "store", "settings.json")
	target := filepath.Join(root, "home", "settings.json")
	writeFile(t, source, `{"theme":"old"}`)
	writeFile(t, target, `{"theme":"old"}`)
	hash, err := HashPath(target)
	if err != nil {
		t.Fatal(err)
	}
	op := Operation{Type: OperationCopy, SourcePath: source, TargetPath: target, LayerKind: "core", LayerName: "work"}
	if err := UpsertManagedTarget(ctx, database, op, hash, time.Now()); err != nil {
		t.Fatalf("UpsertManagedTarget() error = %v", err)
	}
	writeFile(t, target, `{"theme":"new"}`)

	plan, err := BuildCapturePlan(ctx, database)
	if err != nil {
		t.Fatalf("BuildCapturePlan() error = %v", err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Status != CaptureCapturable || plan.Changes[0].TargetPath != target || plan.Changes[0].SourcePath != source {
		t.Fatalf("capture plan = %+v", plan)
	}
}

func TestApplyCapturesWritesCopyTargetBackToStoreAndUpdatesManagedHash(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	source := filepath.Join(root, "store", "settings.json")
	target := filepath.Join(root, "home", "settings.json")
	writeFile(t, source, `{"theme":"old"}`)
	writeFile(t, target, `{"theme":"old"}`)
	hash, err := HashPath(target)
	if err != nil {
		t.Fatal(err)
	}
	op := Operation{Type: OperationCopy, SourcePath: source, TargetPath: target, LayerKind: "core", LayerName: "work"}
	if err := UpsertManagedTarget(ctx, database, op, hash, time.Now()); err != nil {
		t.Fatalf("UpsertManagedTarget() error = %v", err)
	}
	writeFile(t, target, `{"theme":"new"}`)
	plan, err := BuildCapturePlan(ctx, database)
	if err != nil {
		t.Fatalf("BuildCapturePlan() error = %v", err)
	}

	changed, err := ApplyCaptures(ctx, database, plan, time.Now())
	if err != nil {
		t.Fatalf("ApplyCaptures() error = %v", err)
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}
	if got := string(mustReadActivation(t, source)); got != `{"theme":"new"}` {
		t.Fatalf("source = %q", got)
	}
	record, found, err := GetManagedTarget(ctx, database, target)
	if err != nil || !found {
		t.Fatalf("GetManagedTarget() found=%v err=%v", found, err)
	}
	newHash, err := HashPath(target)
	if err != nil {
		t.Fatal(err)
	}
	if record.ContentHash != newHash {
		t.Fatalf("record hash = %q, want %q", record.ContentHash, newHash)
	}
}

func TestBuildCapturePlanConflictsWhenStoreAndTargetChanged(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	source := filepath.Join(root, "store", "settings.json")
	target := filepath.Join(root, "home", "settings.json")
	writeFile(t, source, "old")
	writeFile(t, target, "old")
	hash, err := HashPath(target)
	if err != nil {
		t.Fatal(err)
	}
	op := Operation{Type: OperationCopy, SourcePath: source, TargetPath: target, LayerKind: "core", LayerName: "work"}
	if err := UpsertManagedTarget(ctx, database, op, hash, time.Now()); err != nil {
		t.Fatalf("UpsertManagedTarget() error = %v", err)
	}
	writeFile(t, source, "store-new")
	writeFile(t, target, "local-new")

	plan, err := BuildCapturePlan(ctx, database)
	if err != nil {
		t.Fatalf("BuildCapturePlan() error = %v", err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Status != CaptureConflict {
		t.Fatalf("capture plan = %+v", plan)
	}
}

func TestBuildCapturePlanIgnoresSymlinkManagedTargets(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	source := filepath.Join(root, "store", "settings.txt")
	target := filepath.Join(root, "home", "settings.txt")
	writeFile(t, source, "old")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	hash, err := HashPath(target)
	if err != nil {
		t.Fatal(err)
	}
	op := Operation{Type: OperationSymlink, SourcePath: source, TargetPath: target, LayerKind: "core", LayerName: "work"}
	if err := UpsertManagedTarget(ctx, database, op, hash, time.Now()); err != nil {
		t.Fatalf("UpsertManagedTarget() error = %v", err)
	}
	writeFile(t, source, "new")

	plan, err := BuildCapturePlan(ctx, database)
	if err != nil {
		t.Fatalf("BuildCapturePlan() error = %v", err)
	}
	if len(plan.Changes) != 0 {
		t.Fatalf("capture plan = %+v, want no symlink changes", plan)
	}
}

func TestBuildCapturePlanBlocksRenderAndMergeTargets(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	for _, mode := range []OperationType{OperationRender, OperationMerge} {
		source := filepath.Join(root, string(mode), "source")
		target := filepath.Join(root, string(mode), "target")
		writeFile(t, source, "old")
		writeFile(t, target, "old")
		hash, err := HashPath(target)
		if err != nil {
			t.Fatal(err)
		}
		op := Operation{Type: mode, SourcePath: source, TargetPath: target, LayerKind: "core", LayerName: "work"}
		if err := UpsertManagedTarget(ctx, database, op, hash, time.Now()); err != nil {
			t.Fatalf("UpsertManagedTarget(%s) error = %v", mode, err)
		}
		writeFile(t, target, "new")
	}

	plan, err := BuildCapturePlan(ctx, database)
	if err != nil {
		t.Fatalf("BuildCapturePlan() error = %v", err)
	}
	if len(plan.Changes) != 2 || !plan.HasBlocking() {
		t.Fatalf("capture plan = %+v", plan)
	}
	for _, change := range plan.Changes {
		if change.Status != CaptureUnsupported {
			t.Fatalf("change = %+v, want unsupported", change)
		}
	}
}

func mustReadActivation(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return content
}
