package storemigrate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/asudbring/loki-profile-manager/internal/store"
)

// CopyResult summarizes a completed store-root copy.
type CopyResult struct {
	CopiedFiles    int      `json:"copied_files"`
	CopiedDirs     int      `json:"copied_dirs"`
	CopiedSymlinks int      `json:"copied_symlinks"`
	CopiedBytes    int64    `json:"copied_bytes"`
	Valid          bool     `json:"valid"`
	Missing        []string `json:"missing,omitempty"`
}

// CopyPlan copies the planned store entries and validates the destination layout.
func CopyPlan(plan Plan) (CopyResult, error) {
	if !plan.CanMigrate {
		return CopyResult{}, fmt.Errorf("store migrate: plan is not migratable")
	}
	if plan.FromPath == "" || plan.ToPath == "" {
		return CopyResult{}, fmt.Errorf("store migrate: plan source and destination are required")
	}
	if same, nested, err := sameOrNestedPath(plan.FromPath, plan.ToPath); err != nil {
		return CopyResult{}, err
	} else if same {
		return CopyResult{}, fmt.Errorf("store migrate: source and destination must be different")
	} else if nested {
		return CopyResult{}, fmt.Errorf("store migrate: destination cannot be inside source")
	}
	if _, nested, err := sameOrNestedPath(plan.ToPath, plan.FromPath); err != nil {
		return CopyResult{}, err
	} else if nested {
		return CopyResult{}, fmt.Errorf("store migrate: source cannot be inside destination")
	}
	inspection, err := store.InspectLayout(plan.ToPath)
	if err != nil && inspection.Exists {
		return CopyResult{}, fmt.Errorf("store migrate: inspect destination before copy: %w", err)
	}
	if inspection.Exists && !inspection.IsDir {
		return CopyResult{}, fmt.Errorf("store migrate: destination is not a directory: %s", plan.ToPath)
	}
	if inspection.Exists && !inspection.Empty {
		return CopyResult{}, fmt.Errorf("store migrate: destination must be missing or empty: %s", plan.ToPath)
	}
	if err := os.MkdirAll(plan.ToPath, 0o755); err != nil {
		return CopyResult{}, fmt.Errorf("create destination store %s: %w", plan.ToPath, err)
	}
	var result CopyResult
	for _, entry := range plan.Entries {
		switch entry.Kind {
		case "directory":
			if err := copyDirectoryEntry(entry); err != nil {
				return result, err
			}
			result.CopiedDirs++
		case "file":
			bytes, err := copyFileEntry(entry)
			if err != nil {
				return result, err
			}
			result.CopiedFiles++
			result.CopiedBytes += bytes
		case "symlink":
			if err := copySymlinkEntry(entry); err != nil {
				return result, err
			}
			result.CopiedSymlinks++
		default:
			return result, fmt.Errorf("copy %s: unsupported entry kind %s", entry.RelativePath, entry.Kind)
		}
	}
	validation := store.ValidateLayout(plan.ToPath)
	result.Valid = validation.Valid
	result.Missing = validation.Missing
	if !validation.Valid {
		return result, fmt.Errorf("store migrate: copied destination layout is invalid: missing %v", validation.Missing)
	}
	return result, nil
}

func copyDirectoryEntry(entry Entry) error {
	info, err := os.Lstat(entry.SourcePath)
	if err != nil {
		return fmt.Errorf("stat directory %s: %w", entry.SourcePath, err)
	}
	if err := os.MkdirAll(entry.DestPath, info.Mode().Perm()); err != nil {
		return fmt.Errorf("create directory %s: %w", entry.DestPath, err)
	}
	return nil
}

func copyFileEntry(entry Entry) (int64, error) {
	info, err := os.Lstat(entry.SourcePath)
	if err != nil {
		return 0, fmt.Errorf("stat file %s: %w", entry.SourcePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(entry.DestPath), 0o755); err != nil {
		return 0, fmt.Errorf("create parent for %s: %w", entry.DestPath, err)
	}
	source, err := os.Open(entry.SourcePath)
	if err != nil {
		return 0, fmt.Errorf("open source file %s: %w", entry.SourcePath, err)
	}
	defer source.Close()
	dest, err := os.OpenFile(entry.DestPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return 0, fmt.Errorf("create destination file %s: %w", entry.DestPath, err)
	}
	written, copyErr := io.Copy(dest, source)
	closeErr := dest.Close()
	if copyErr != nil {
		return written, fmt.Errorf("copy file %s to %s: %w", entry.SourcePath, entry.DestPath, copyErr)
	}
	if closeErr != nil {
		return written, fmt.Errorf("close destination file %s: %w", entry.DestPath, closeErr)
	}
	return written, nil
}

func copySymlinkEntry(entry Entry) error {
	linkTarget, err := os.Readlink(entry.SourcePath)
	if err != nil {
		return fmt.Errorf("read symlink %s: %w", entry.SourcePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(entry.DestPath), 0o755); err != nil {
		return fmt.Errorf("create parent for %s: %w", entry.DestPath, err)
	}
	if err := os.Symlink(linkTarget, entry.DestPath); err != nil {
		return fmt.Errorf("create symlink %s: %w", entry.DestPath, err)
	}
	return nil
}
