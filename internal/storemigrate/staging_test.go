package storemigrate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/store"
)

func TestNewStagingPathIsSiblingAndHidden(t *testing.T) {
	final := filepath.Join(t.TempDir(), "LokiProfileManager")
	staging := NewStaging(final, time.Date(2026, 5, 15, 1, 2, 3, 0, time.UTC))
	if filepath.Dir(staging.Path) != filepath.Dir(final) {
		t.Fatalf("staging path = %s, want sibling of %s", staging.Path, final)
	}
	if filepath.Base(staging.Path) == filepath.Base(final) || filepath.Base(staging.Path)[0] != '.' {
		t.Fatalf("staging path = %s, want hidden non-final path", staging.Path)
	}
}

func TestPromoteStagingRenamesValidatedStoreToFinal(t *testing.T) {
	root := t.TempDir()
	final := filepath.Join(root, "LokiProfileManager")
	staging := NewStaging(final, time.Date(2026, 5, 15, 1, 2, 3, 0, time.UTC))
	if _, err := store.EnsureLayout(staging.Path); err != nil {
		t.Fatalf("EnsureLayout(staging) error = %v", err)
	}
	if err := writeStagingMarker(staging); err != nil {
		t.Fatalf("writeStagingMarker() error = %v", err)
	}
	if err := PromoteStaging(staging); err != nil {
		t.Fatalf("PromoteStaging() error = %v", err)
	}
	if validation := store.ValidateLayout(final); !validation.Valid {
		t.Fatalf("final invalid: %+v", validation)
	}
	if _, err := os.Stat(staging.Path); !os.IsNotExist(err) {
		t.Fatalf("staging remains or stat err = %v", err)
	}
}

func TestPrepareStagingRefusesExistingDirectory(t *testing.T) {
	final := filepath.Join(t.TempDir(), "LokiProfileManager")
	staging := NewStaging(final, time.Date(2026, 5, 15, 1, 2, 3, 0, time.UTC))
	if err := os.MkdirAll(staging.Path, 0o755); err != nil {
		t.Fatalf("MkdirAll(staging) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(staging.Path, "user-data.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("WriteFile(user-data) error = %v", err)
	}
	if err := PrepareStaging(staging); err == nil {
		t.Fatalf("PrepareStaging(existing) error = nil, want failure")
	}
	if got := mustReadStoreMigrateTest(t, filepath.Join(staging.Path, "user-data.txt")); string(got) != "keep" {
		t.Fatalf("existing data changed: %q", got)
	}
}

func TestCleanupStagingPathRefusesUnmarkedDirectory(t *testing.T) {
	final := filepath.Join(t.TempDir(), "LokiProfileManager")
	staging := NewStaging(final, time.Date(2026, 5, 15, 1, 2, 3, 0, time.UTC))
	if err := os.MkdirAll(staging.Path, 0o755); err != nil {
		t.Fatalf("MkdirAll(staging) error = %v", err)
	}
	if err := CleanupStagingPath(staging); err == nil {
		t.Fatalf("CleanupStagingPath(unmarked) error = nil, want refusal")
	}
	if _, err := os.Stat(staging.Path); err != nil {
		t.Fatalf("unmarked staging removed or stat err = %v", err)
	}
}

func TestCleanupStagingRemovesInterruptedSiblings(t *testing.T) {
	root := t.TempDir()
	final := filepath.Join(root, "LokiProfileManager")
	staging := NewStaging(final, time.Date(2026, 5, 15, 1, 2, 3, 0, time.UTC))
	if err := PrepareStaging(staging); err != nil {
		t.Fatalf("PrepareStaging() error = %v", err)
	}
	unmarked := filepath.Join(root, ".LokiProfileManager.incomplete-unmarked")
	if err := os.MkdirAll(unmarked, 0o755); err != nil {
		t.Fatalf("MkdirAll(unmarked) error = %v", err)
	}
	unrelated := filepath.Join(root, ".Other.incomplete-20260515")
	if err := os.MkdirAll(unrelated, 0o755); err != nil {
		t.Fatalf("MkdirAll(unrelated) error = %v", err)
	}
	removed, err := CleanupStaging(final)
	if err != nil {
		t.Fatalf("CleanupStaging() error = %v", err)
	}
	if len(removed) != 1 || removed[0] != staging.Path {
		t.Fatalf("removed = %+v, want %s", removed, staging.Path)
	}
	if _, err := os.Stat(staging.Path); !os.IsNotExist(err) {
		t.Fatalf("staging remains or stat err = %v", err)
	}
	if _, err := os.Stat(unmarked); err != nil {
		t.Fatalf("unmarked matching directory removed or stat err = %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated removed or stat err = %v", err)
	}
}
