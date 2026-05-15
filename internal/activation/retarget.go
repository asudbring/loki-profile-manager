package activation

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// RetargetManagedSymlinks updates active Loki-managed symlinks that still point at oldRoot.
func RetargetManagedSymlinks(ctx context.Context, database *sql.DB, oldRoot, newRoot string) (int, error) {
	if database == nil {
		return 0, fmt.Errorf("retarget managed symlinks: database is nil")
	}
	if err := validateRebaseRoots(oldRoot, newRoot); err != nil {
		return 0, err
	}
	records, err := ListManagedTargets(ctx, database)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, record := range records {
		if record.Mode != string(OperationSymlink) {
			continue
		}
		info, err := os.Lstat(record.TargetPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return changed, fmt.Errorf("retarget managed symlink %s: %w", record.TargetPath, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		currentTarget, err := os.Readlink(record.TargetPath)
		if err != nil {
			return changed, fmt.Errorf("read managed symlink %s: %w", record.TargetPath, err)
		}
		nextTarget, ok, err := rebasePathUnderRoot(currentTarget, oldRoot, newRoot)
		if err != nil {
			return changed, err
		}
		if !ok || samePath(currentTarget, nextTarget) {
			continue
		}
		if record.SourcePath != "" && !samePath(record.SourcePath, nextTarget) {
			if rebasedRecordSource, ok, err := rebasePathUnderRoot(record.SourcePath, oldRoot, newRoot); err != nil {
				return changed, err
			} else if ok && !samePath(rebasedRecordSource, nextTarget) {
				continue
			}
		}
		if err := replaceSymlinkTarget(nextTarget, record.TargetPath); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}

func replaceSymlinkTarget(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", target, err)
	}
	if err := removeExisting(target); err != nil {
		return err
	}
	if err := os.Symlink(source, target); err != nil {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("retarget symlink %s -> %s: %w. Enable Developer Mode or run from an elevated shell", target, source, err)
		}
		return fmt.Errorf("retarget symlink %s -> %s: %w", target, source, err)
	}
	return nil
}
