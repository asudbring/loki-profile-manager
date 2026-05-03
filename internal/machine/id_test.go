package machine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestEnsureIDCreatesAndReusesUUID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "machine_id")
	first, err := EnsureID(path)
	if err != nil {
		t.Fatalf("EnsureID() error = %v", err)
	}
	if _, err := uuid.Parse(first); err != nil {
		t.Fatalf("machine id is not UUID: %v", err)
	}
	second, err := EnsureID(path)
	if err != nil {
		t.Fatalf("EnsureID() second error = %v", err)
	}
	if second != first {
		t.Fatalf("second id = %q, want %q", second, first)
	}
}

func TestEnsureIDInvalidExistingFileFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "machine_id")
	if err := os.WriteFile(path, []byte("not-a-uuid\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := EnsureID(path); err == nil {
		t.Fatal("EnsureID() error = nil, want error")
	}
}
