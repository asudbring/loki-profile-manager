package machine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

func EnsureID(machineIDPath string) (string, error) {
	if strings.TrimSpace(machineIDPath) == "" {
		return "", fmt.Errorf("ensure machine id: path is required")
	}
	if id, ok, err := ReadID(machineIDPath); err != nil || ok {
		return id, err
	}

	id := uuid.NewString()
	if err := os.MkdirAll(filepath.Dir(machineIDPath), 0o700); err != nil {
		return "", fmt.Errorf("create machine id directory: %w", err)
	}
	if err := os.WriteFile(machineIDPath, []byte(id+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write machine id %s: %w", machineIDPath, err)
	}
	return id, nil
}
