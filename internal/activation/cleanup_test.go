package activation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupRemovesChangedObsoleteRenderTargets(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	storeRoot := filepath.Join(root, "store")
	source := filepath.Join(storeRoot, "profiles", "work", "core", "templates", "config.toml.template")
	target := filepath.Join(root, "home", "config.toml")
	writeFile(t, source, "profile = \"work\"\n")
	writeFile(t, target, "profile = \"work\"\n")
	hash, err := HashPath(target)
	if err != nil {
		t.Fatal(err)
	}
	op := Operation{ID: "config", Type: OperationRender, TargetPath: target, SourcePath: source, LayerName: "work-core", LayerKind: "core"}
	if err := UpsertManagedTarget(ctx, database, op, hash, time.Now()); err != nil {
		t.Fatalf("UpsertManagedTarget() error = %v", err)
	}
	writeFile(t, target, "profile = \"runtime\"\n")

	cleanup, err := BuildCleanupPlanForTargets(ctx, database, map[string]bool{}, storeRoot)
	if err != nil {
		t.Fatalf("BuildCleanupPlanForTargets() error = %v", err)
	}
	if len(cleanup.Changes) != 1 || cleanup.Changes[0].Status != CleanupRemovable {
		t.Fatalf("cleanup changes = %+v", cleanup.Changes)
	}
	cleaned, err := ApplyCleanup(ctx, database, cleanup, map[string]bool{})
	if err != nil {
		t.Fatalf("ApplyCleanup() error = %v", err)
	}
	if cleaned != 1 {
		t.Fatalf("cleaned = %d, want 1", cleaned)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists or stat err = %v", err)
	}
}

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
