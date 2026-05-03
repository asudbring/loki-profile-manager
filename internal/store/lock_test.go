package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOperationLockSerializes(t *testing.T) {
	root := t.TempDir()
	unlock, err := AcquireOperationLock(context.Background(), root, OperationLockOptions{Operation: "first"})
	if err != nil {
		t.Fatalf("AcquireOperationLock(first) error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := AcquireOperationLock(ctx, root, OperationLockOptions{Operation: "second", Timeout: time.Hour}); err == nil || !strings.Contains(err.Error(), OperationLockPath(root)) {
		t.Fatalf("AcquireOperationLock(second) error = %v", err)
	}

	unlock()
	secondUnlock, err := AcquireOperationLock(context.Background(), root, OperationLockOptions{Operation: "second"})
	if err != nil {
		t.Fatalf("AcquireOperationLock(second after unlock) error = %v", err)
	}
	secondUnlock()
}

func TestOperationLockUnlockDoesNotRemoveDifferentToken(t *testing.T) {
	root := t.TempDir()
	unlock, err := AcquireOperationLock(context.Background(), root, OperationLockOptions{Operation: "first"})
	if err != nil {
		t.Fatalf("AcquireOperationLock() error = %v", err)
	}
	path := OperationLockPath(root)
	replacement := testOperationLockInfo("replacement", time.Now().UTC(), time.Hour)
	replacement.Token = "different-token"
	writeOperationLockInfo(t, path, replacement)

	unlock()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("replacement lock removed or unreadable: %v", err)
	}
	if !strings.Contains(string(content), "different-token") {
		t.Fatalf("replacement lock content = %s", content)
	}
}

func TestOperationLockReportsStaleLock(t *testing.T) {
	root := t.TempDir()
	path := OperationLockPath(root)
	stale := testOperationLockInfo("old", time.Now().Add(-time.Hour), 10*time.Minute)
	writeOperationLockInfo(t, path, stale)

	_, err := AcquireOperationLock(context.Background(), root, OperationLockOptions{Operation: "new", StaleAfter: 10 * time.Minute})
	if err == nil || !strings.Contains(err.Error(), "stale lock detected") || !strings.Contains(err.Error(), "held by old") {
		t.Fatalf("AcquireOperationLock() error = %v", err)
	}
	current, err := readOperationLockInfo(path)
	if err != nil {
		t.Fatalf("readOperationLockInfo() error = %v", err)
	}
	if current.Token != stale.Token {
		t.Fatalf("stale lock was replaced: %+v", current)
	}
}

func TestOperationLockDoesNotBreakFreshLock(t *testing.T) {
	root := t.TempDir()
	path := OperationLockPath(root)
	fresh := testOperationLockInfo("active", time.Now().UTC(), time.Hour)
	writeOperationLockInfo(t, path, fresh)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := AcquireOperationLock(ctx, root, OperationLockOptions{Operation: "new", Timeout: time.Hour}); err == nil || !strings.Contains(err.Error(), "held by active") {
		t.Fatalf("AcquireOperationLock() error = %v", err)
	}
	current, err := readOperationLockInfo(path)
	if err != nil {
		t.Fatalf("readOperationLockInfo() error = %v", err)
	}
	if current.Token != fresh.Token {
		t.Fatalf("fresh lock was replaced: %+v", current)
	}
}

func TestOperationLockInvalidLockContentTimesOut(t *testing.T) {
	root := t.TempDir()
	path := OperationLockPath(root)
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := AcquireOperationLock(ctx, root, OperationLockOptions{Operation: "new", Timeout: time.Hour}); err == nil || !strings.Contains(err.Error(), "unreadable or invalid") {
		t.Fatalf("AcquireOperationLock() error = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "not json" {
		t.Fatalf("invalid lock changed: content=%q err=%v", got, err)
	}
}

func testOperationLockInfo(operation string, acquiredAt time.Time, ttl time.Duration) OperationLockInfo {
	return OperationLockInfo{
		Version:    1,
		PID:        12345,
		MachineID:  "test-machine",
		Operation:  operation,
		Hostname:   "test-host",
		AcquiredAt: acquiredAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:  acquiredAt.Add(ttl).UTC().Format(time.RFC3339Nano),
		Token:      operation + "-token",
	}
}

func writeOperationLockInfo(t *testing.T, path string, info OperationLockInfo) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	content, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(path, append(content, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
