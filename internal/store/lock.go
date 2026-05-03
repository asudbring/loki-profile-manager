package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	operationLockFile              = ".loki-operation.lock"
	defaultOperationLockTimeout    = 10 * time.Second
	defaultOperationLockStaleAfter = 30 * time.Minute
	operationLockPollInterval      = 50 * time.Millisecond
)

// OperationLockOptions controls cooperative store-level mutation locking.
type OperationLockOptions struct {
	Operation  string
	MachineID  string
	Timeout    time.Duration
	StaleAfter time.Duration
	Now        func() time.Time
}

// OperationLockInfo is the JSON body written to the operation lock file.
type OperationLockInfo struct {
	Version    int    `json:"version"`
	PID        int    `json:"pid"`
	MachineID  string `json:"machine_id,omitempty"`
	Operation  string `json:"operation"`
	Hostname   string `json:"hostname,omitempty"`
	AcquiredAt string `json:"acquired_at"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	Token      string `json:"token"`
}

// OperationLockPath returns the shared store operation lock path.
func OperationLockPath(storeRoot string) string {
	return filepath.Join(filepath.Clean(storeRoot), operationLockFile)
}

// WithOperationLock acquires the store operation lock, runs fn, then releases the lock.
func WithOperationLock(ctx context.Context, storeRoot string, opts OperationLockOptions, fn func() error) error {
	unlock, err := AcquireOperationLock(ctx, storeRoot, opts)
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

// AcquireOperationLock acquires a cooperative file lock for store-mutating operations.
func AcquireOperationLock(ctx context.Context, storeRoot string, opts OperationLockOptions) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if storeRoot == "" {
		return nil, fmt.Errorf("acquire store operation lock: store root is required")
	}
	operation := opts.Operation
	if operation == "" {
		operation = "operation"
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultOperationLockTimeout
	}
	staleAfter := opts.StaleAfter
	if staleAfter == 0 {
		staleAfter = defaultOperationLockStaleAfter
	}
	lockPath := OperationLockPath(storeRoot)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create store operation lock directory: %w", err)
	}

	deadline := now().Add(timeout)
	var lastInfo *OperationLockInfo
	var lastReadErr error
	for {
		info, content, err := newOperationLockInfo(operation, opts.MachineID, staleAfter, now)
		if err != nil {
			return nil, err
		}
		file, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			if _, writeErr := file.Write(content); writeErr != nil {
				_ = file.Close()
				_ = os.Remove(lockPath)
				return nil, fmt.Errorf("write store operation lock %s: %w", lockPath, writeErr)
			}
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(lockPath)
				return nil, fmt.Errorf("close store operation lock %s: %w", lockPath, closeErr)
			}
			return operationUnlockFunc(lockPath, info.Token), nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create store operation lock %s: %w", lockPath, err)
		}

		lastInfo = nil
		lastReadErr = nil
		if existing, readErr := readOperationLockInfo(lockPath); readErr == nil {
			lastInfo = &existing
			if operationLockIsStale(existing, now(), staleAfter) {
				return nil, operationLockWaitError(lockPath, operation, fmt.Errorf("stale lock detected"), lastInfo, nil)
			}
		} else {
			lastReadErr = readErr
		}

		if !now().Before(deadline) {
			return nil, operationLockWaitError(lockPath, operation, fmt.Errorf("timed out"), lastInfo, lastReadErr)
		}

		select {
		case <-ctx.Done():
			return nil, operationLockWaitError(lockPath, operation, ctx.Err(), lastInfo, lastReadErr)
		case <-time.After(operationLockPollInterval):
		}
	}
}

func newOperationLockInfo(operation, machineID string, staleAfter time.Duration, now func() time.Time) (OperationLockInfo, []byte, error) {
	acquiredAt := now().UTC()
	hostname, _ := os.Hostname()
	token, err := operationLockToken()
	if err != nil {
		return OperationLockInfo{}, nil, err
	}
	info := OperationLockInfo{
		Version:    1,
		PID:        os.Getpid(),
		MachineID:  machineID,
		Operation:  operation,
		Hostname:   hostname,
		AcquiredAt: acquiredAt.Format(time.RFC3339Nano),
		Token:      token,
	}
	if staleAfter > 0 {
		info.ExpiresAt = acquiredAt.Add(staleAfter).Format(time.RFC3339Nano)
	}
	content, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return OperationLockInfo{}, nil, fmt.Errorf("marshal store operation lock: %w", err)
	}
	return info, append(content, '\n'), nil
}

func operationLockToken() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate store operation lock token: %w", err)
	}
	return fmt.Sprintf("%d-%d-%s", os.Getpid(), time.Now().UnixNano(), hex.EncodeToString(random[:])), nil
}

func operationUnlockFunc(lockPath, token string) func() {
	return func() {
		info, err := readOperationLockInfo(lockPath)
		if err == nil && info.Token == token {
			_ = os.Remove(lockPath)
		}
	}
}

func readOperationLockInfo(path string) (OperationLockInfo, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return OperationLockInfo{}, err
	}
	var info OperationLockInfo
	if err := json.Unmarshal(content, &info); err != nil {
		return OperationLockInfo{}, fmt.Errorf("parse store operation lock %s: %w", path, err)
	}
	return info, nil
}

func operationLockIsStale(info OperationLockInfo, now time.Time, staleAfter time.Duration) bool {
	if staleAfter <= 0 {
		return false
	}
	if info.ExpiresAt != "" {
		if expiresAt, err := time.Parse(time.RFC3339Nano, info.ExpiresAt); err == nil {
			return now.UTC().After(expiresAt)
		}
	}
	if info.AcquiredAt != "" {
		if acquiredAt, err := time.Parse(time.RFC3339Nano, info.AcquiredAt); err == nil {
			return now.UTC().Sub(acquiredAt) > staleAfter
		}
	}
	return false
}

func operationLockWaitError(lockPath, operation string, cause error, info *OperationLockInfo, readErr error) error {
	message := fmt.Sprintf("acquire store operation lock %s: %v waiting for %s", lockPath, cause, operation)
	if info != nil {
		message += fmt.Sprintf("; held by %s pid=%d host=%s machine=%s since=%s", info.Operation, info.PID, info.Hostname, info.MachineID, info.AcquiredAt)
	} else if readErr != nil {
		message += fmt.Sprintf("; existing lock is unreadable or invalid: %v", readErr)
	}
	message += "; remove stale lock only if no Loki process is active"
	return fmt.Errorf("%s", message)
}
