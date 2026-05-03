package machine

import (
	"os"
	"path/filepath"
	"sync"
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

func TestEnsureIDConcurrentReturnsSameUUID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "machine_id")
	const workers = 16
	ids := make([]string, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ids[i], errs[i] = EnsureID(path)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("EnsureID(%d) error = %v", i, err)
		}
		if ids[i] != ids[0] {
			t.Fatalf("ids[%d] = %q, want %q", i, ids[i], ids[0])
		}
	}
}
