package storesync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const DefaultConflictScanLimit = 100

const (
	ConflictActionDelete = "delete"
	ConflictActionSkip   = "skip"
)

type ConflictScanOptions struct {
	Root          string
	Limit         int
	Hostname      string
	DisplayName   string
	ExtraPatterns []string
}

type ConflictCopy struct {
	Path         string `json:"path"`
	RelativePath string `json:"relative_path"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Action       string `json:"action"`
	Reason       string `json:"reason,omitempty"`
}

type ConflictScanResult struct {
	Conflicts []ConflictCopy `json:"conflicts"`
	Truncated bool           `json:"truncated"`
}

func ScanConflicts(opts ConflictScanOptions) (ConflictScanResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		return ConflictScanResult{}, fmt.Errorf("scan conflicts: root is required")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultConflictScanLimit
	}
	root = filepath.Clean(root)
	result := ConflictScanResult{Conflicts: []ConflictCopy{}}
	appendConflict := func(conflict ConflictCopy) error {
		result.Conflicts = append(result.Conflicts, conflict)
		if len(result.Conflicts) >= limit {
			result.Truncated = true
			return fs.SkipAll
		}
		return nil
	}
	var scanErr error
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if scanErr == nil {
				scanErr = fmt.Errorf("scan %s: %w", path, err)
			}
			return nil
		}
		name := entry.Name()
		if entry.IsDir() && shouldSkipDir(name) && path != root {
			return filepath.SkipDir
		}
		if !IsConflictCopyName(name, opts) {
			return nil
		}
		conflict := ConflictCopy{
			Path:         filepath.Clean(path),
			RelativePath: relativePath(root, path),
			Name:         name,
		}
		if entry.IsDir() {
			conflict.Kind = "directory"
			conflict.Action = ConflictActionSkip
			conflict.Reason = "directory conflict copies are not deleted by sync MVP"
			if err := appendConflict(conflict); err != nil {
				return err
			}
			return filepath.SkipDir
		}
		info, statErr := os.Lstat(path)
		if statErr != nil {
			if scanErr == nil {
				scanErr = fmt.Errorf("stat conflict copy %s: %w", path, statErr)
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			conflict.Kind = "symlink"
		} else if info.Mode().IsRegular() {
			conflict.Kind = "file"
		} else {
			conflict.Kind = info.Mode().String()
			conflict.Action = ConflictActionSkip
			conflict.Reason = "non-regular conflict copy is not deleted by sync MVP"
			return appendConflict(conflict)
		}
		if IsDeletableConflictCopyName(name, opts) {
			conflict.Action = ConflictActionDelete
		} else {
			conflict.Action = ConflictActionSkip
			conflict.Reason = "conflict-copy name needs manual review"
		}
		return appendConflict(conflict)
	})
	if errors.Is(err, fs.SkipAll) {
		err = nil
	}
	if err == nil {
		err = scanErr
	}
	sort.Slice(result.Conflicts, func(i, j int) bool {
		return result.Conflicts[i].RelativePath < result.Conflicts[j].RelativePath
	})
	return result, err
}

func IsConflictCopyName(name string, opts ConflictScanOptions) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	if IsDeletableConflictCopyName(name, opts) {
		return true
	}
	return strings.Contains(lower, "case conflict")
}

func IsDeletableConflictCopyName(name string, opts ConflictScanOptions) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	patterns := []string{"conflicted copy", "conflict copy", "sync conflict"}
	patterns = append(patterns, opts.ExtraPatterns...)
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern != "" && strings.Contains(lower, pattern) {
			return true
		}
	}
	if strings.Contains(lower, "conflict") && (strings.Contains(lower, "onedrive") || strings.Contains(lower, "dropbox")) {
		return true
	}
	for _, owner := range []string{opts.Hostname, opts.DisplayName} {
		owner = strings.ToLower(strings.TrimSpace(owner))
		if owner != "" && strings.Contains(lower, owner) && strings.Contains(lower, "conflict") {
			return true
		}
	}
	return false
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", ".DS_Store":
		return true
	default:
		return false
	}
}

func relativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
