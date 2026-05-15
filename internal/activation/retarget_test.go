package activation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetargetManagedSymlinksMovesActiveLinksToNewStore(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	oldRoot := filepath.Join(root, "old")
	newRoot := filepath.Join(root, "new")
	oldSource := filepath.Join(oldRoot, "profiles", "work", "core", "files", "settings.json")
	newSource := filepath.Join(newRoot, "profiles", "work", "core", "files", "settings.json")
	target := filepath.Join(root, "target.json")
	writeFile(t, oldSource, "old")
	writeFile(t, newSource, "new")
	if err := ApplySymlink(oldSource, target); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := PutManagedTarget(ctx, database, ManagedTarget{
		TargetPath:    target,
		SourcePath:    newSource,
		Mode:          string(OperationSymlink),
		LayerKind:     "core",
		LayerName:     "work",
		LastAppliedAt: time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("PutManagedTarget() error = %v", err)
	}

	changed, err := RetargetManagedSymlinks(ctx, database, oldRoot, newRoot)
	if err != nil {
		t.Fatalf("RetargetManagedSymlinks() error = %v", err)
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink(target) error = %v", err)
	}
	if got != newSource {
		t.Fatalf("link target = %q, want %q", got, newSource)
	}
}

func TestRetargetManagedSymlinksRetargetsBrokenLinksToNewRoot(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	oldRoot := filepath.Join(root, "old")
	newRoot := filepath.Join(root, "new")
	oldSource := filepath.Join(oldRoot, "profiles", "writer", "core", "files", ".stow-local-ignore")
	newSource := filepath.Join(newRoot, "profiles", "writer", "core", "files", ".stow-local-ignore")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll(target parent) error = %v", err)
	}
	if err := os.Symlink(oldSource, target); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := PutManagedTarget(ctx, database, ManagedTarget{
		TargetPath:    target,
		SourcePath:    newSource,
		Mode:          string(OperationSymlink),
		LayerKind:     "core",
		LayerName:     "writer",
		LastAppliedAt: time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("PutManagedTarget() error = %v", err)
	}
	changed, err := RetargetManagedSymlinks(ctx, database, oldRoot, newRoot)
	if err != nil {
		t.Fatalf("RetargetManagedSymlinks() error = %v", err)
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink(target) error = %v", err)
	}
	if got != newSource {
		t.Fatalf("link target = %q, want %q", got, newSource)
	}
}

func TestRetargetManagedSymlinksSkipsCopyManagedTargets(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	oldRoot := filepath.Join(root, "old")
	newRoot := filepath.Join(root, "new")
	oldSource := filepath.Join(oldRoot, "profiles", "work", "core", "files", "settings.json")
	target := filepath.Join(root, "target.json")
	writeFile(t, oldSource, "old")
	if err := os.Symlink(oldSource, target); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := PutManagedTarget(ctx, database, ManagedTarget{TargetPath: target, SourcePath: oldSource, Mode: string(OperationCopy), LastAppliedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatalf("PutManagedTarget() error = %v", err)
	}
	changed, err := RetargetManagedSymlinks(ctx, database, oldRoot, newRoot)
	if err != nil {
		t.Fatalf("RetargetManagedSymlinks() error = %v", err)
	}
	if changed != 0 {
		t.Fatalf("changed = %d, want 0", changed)
	}
}
