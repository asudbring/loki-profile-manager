package activation

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeFileAtomic(target string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", target, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".loki-write-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", target, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file for %s: %w", target, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp file for %s: %w", target, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", target, err)
	}
	if err := replacePath(tmpPath, target); err != nil {
		return err
	}
	return nil
}
