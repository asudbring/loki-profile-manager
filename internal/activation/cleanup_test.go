package activation

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupPlanOnlyIncludesCurrentStoreRecords(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	storeA := filepath.Join(root, "store-a")
	storeB := filepath.Join(root, "store-b")
	targetA := filepath.Join(root, "a.txt")
	targetB := filepath.Join(root, "b.txt")
	writeFile(t, targetA, "a")
	writeFile(t, targetB, "b")
	for _, item := range []struct {
		store  string
		target string
	}{
		{store: storeA, target: targetA},
		{store: storeB, target: targetB},
	} {
		source := filepath.Join(item.store, "profiles", "work", "core", "files", filepath.Base(item.target))
		writeFile(t, source, filepath.Base(item.target))
		hash, err := HashPath(item.target)
		if err != nil {
			t.Fatal(err)
		}
		op := Operation{ID: filepath.Base(item.target), Type: OperationCopy, TargetPath: item.target, SourcePath: source, LayerName: "work-core", LayerKind: "core"}
		if err := UpsertManagedTarget(ctx, database, op, hash, time.Now()); err != nil {
			t.Fatalf("UpsertManagedTarget() error = %v", err)
		}
	}

	plan := Plan{StorePath: storeA, Profile: "work"}
	cleanup, err := BuildCleanupPlanForPlan(ctx, database, plan)
	if err != nil {
		t.Fatalf("BuildCleanupPlanForPlan() error = %v", err)
	}
	if len(cleanup.Changes) != 1 || cleanup.Changes[0].TargetPath != targetA {
		t.Fatalf("cleanup changes = %+v, want only %s", cleanup.Changes, targetA)
	}
}
