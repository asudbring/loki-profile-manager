package activation

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func ApplySymlink(source, target string) error {
	if _, err := os.Lstat(source); err != nil {
		return fmt.Errorf("create symlink %s -> %s: source invalid: %w", target, source, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", target, err)
	}
	if err := removeExisting(target); err != nil {
		return err
	}
	if err := os.Symlink(source, target); err != nil {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("create symlink %s -> %s: %w. Enable Developer Mode or run from an elevated shell; Loki does not fall back to copy unless a future manifest flag permits it", target, source, err)
		}
		return fmt.Errorf("create symlink %s -> %s: %w", target, source, err)
	}
	return nil
}
