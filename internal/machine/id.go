package machine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

func ReadID(machineIDPath string) (string, bool, error) {
	if strings.TrimSpace(machineIDPath) == "" {
		return "", false, fmt.Errorf("read machine id: path is required")
	}
	content, err := os.ReadFile(machineIDPath)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read machine id %s: %w", machineIDPath, err)
	}
	id := strings.TrimSpace(string(content))
	if _, parseErr := uuid.Parse(id); parseErr != nil {
		return "", false, fmt.Errorf("machine id file %s contains invalid UUID: %w", machineIDPath, parseErr)
	}
	return id, true, nil
}

var machineIDLocks sync.Map

func EnsureID(machineIDPath string) (string, error) {
	if strings.TrimSpace(machineIDPath) == "" {
		return "", fmt.Errorf("ensure machine id: path is required")
	}
	lock := lockForMachineID(machineIDPath)
	lock.Lock()
	defer lock.Unlock()
	if id, ok, err := ReadID(machineIDPath); err != nil || ok {
		if err != nil && machineIDFileEmpty(machineIDPath) {
			return readIDWithRetry(machineIDPath)
		}
		return id, err
	}

	id := uuid.NewString()
	if err := os.MkdirAll(filepath.Dir(machineIDPath), 0o700); err != nil {
		return "", fmt.Errorf("create machine id directory: %w", err)
	}
	file, err := os.OpenFile(machineIDPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return readIDWithRetry(machineIDPath)
	}
	if err != nil {
		return "", fmt.Errorf("create machine id %s: %w", machineIDPath, err)
	}
	if _, err := file.WriteString(id + "\n"); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write machine id %s: %w", machineIDPath, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close machine id %s: %w", machineIDPath, err)
	}
	return id, nil
}

func lockForMachineID(machineIDPath string) *sync.Mutex {
	key := filepath.Clean(machineIDPath)
	actual, _ := machineIDLocks.LoadOrStore(key, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

func machineIDFileEmpty(machineIDPath string) bool {
	content, err := os.ReadFile(machineIDPath)
	return err == nil && strings.TrimSpace(string(content)) == ""
}

func readIDWithRetry(machineIDPath string) (string, error) {
	deadline := time.Now().Add(time.Second)
	var lastErr error
	for {
		id, ok, err := ReadID(machineIDPath)
		if err == nil && ok {
			return id, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("machine id file %s disappeared during concurrent creation", machineIDPath)
		}
		if time.Now().After(deadline) {
			return "", lastErr
		}
		time.Sleep(10 * time.Millisecond)
	}
}
