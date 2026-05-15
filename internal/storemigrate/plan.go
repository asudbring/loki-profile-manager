package storemigrate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/asudbring/loki-profile-manager/internal/store"
	"github.com/asudbring/loki-profile-manager/internal/storesync"
)

// PlanOptions describes a store-root migration plan request.
type PlanOptions struct {
	FromPath string
	ToPath   string
	Provider store.ProviderType
}

// Entry describes one filesystem entry copied from source store to destination store.
type Entry struct {
	RelativePath string `json:"relative_path"`
	SourcePath   string `json:"source_path"`
	DestPath     string `json:"dest_path"`
	Kind         string `json:"kind"`
	Size         int64  `json:"size"`
	Mode         string `json:"mode"`
}

// Summary is the aggregate copy estimate for a migration plan.
type Summary struct {
	FileCount    int   `json:"file_count"`
	DirCount     int   `json:"dir_count"`
	SymlinkCount int   `json:"symlink_count"`
	ByteCount    int64 `json:"byte_count"`
}

// Plan is a dry-run-safe description of a store-root migration.
type Plan struct {
	FromPath   string             `json:"from_path"`
	ToPath     string             `json:"to_path"`
	Provider   store.ProviderType `json:"provider,omitempty"`
	CanMigrate bool               `json:"can_migrate"`
	Summary    Summary            `json:"summary"`
	Entries    []Entry            `json:"entries,omitempty"`
	Warnings   []string           `json:"warnings,omitempty"`
	Blockers   []string           `json:"blockers,omitempty"`
}

// BuildPlan validates source/destination safety and builds a copy manifest.
func BuildPlan(opts PlanOptions) (Plan, error) {
	from := filepath.Clean(strings.TrimSpace(opts.FromPath))
	to := filepath.Clean(strings.TrimSpace(opts.ToPath))
	plan := Plan{FromPath: from, ToPath: to, Provider: opts.Provider, Entries: []Entry{}, Warnings: []string{}, Blockers: []string{}}
	if from == "" || from == "." {
		return plan, fmt.Errorf("store migrate: source store path is required")
	}
	if to == "" || to == "." {
		return plan, fmt.Errorf("store migrate: destination store path is required")
	}
	if same, nested, err := sameOrNestedPath(from, to); err != nil {
		return plan, err
	} else if same {
		return plan, fmt.Errorf("store migrate: source and destination must be different")
	} else if nested {
		return plan, fmt.Errorf("store migrate: destination cannot be inside source")
	}
	if _, nested, err := sameOrNestedPath(to, from); err != nil {
		return plan, err
	} else if nested {
		return plan, fmt.Errorf("store migrate: source cannot be inside destination")
	}

	sourceInspection, err := store.InspectLayout(from)
	if err != nil {
		return plan, fmt.Errorf("store migrate: inspect source: %w", err)
	}
	if !sourceInspection.Exists {
		return plan, fmt.Errorf("store migrate: source store does not exist: %s", from)
	}
	if !sourceInspection.Valid {
		return plan, fmt.Errorf("store migrate: source store layout is invalid: missing %v", sourceInspection.Missing)
	}

	destInspection, err := store.InspectLayout(to)
	if err != nil && destInspection.Exists {
		return plan, fmt.Errorf("store migrate: inspect destination: %w", err)
	}
	if destInspection.Exists && !destInspection.IsDir {
		return plan, fmt.Errorf("store migrate: destination is not a directory: %s", to)
	}
	if destInspection.Exists && !destInspection.Empty {
		return plan, fmt.Errorf("store migrate: destination must be missing or empty: %s", to)
	}

	conflicts, err := storesync.ScanConflicts(storesync.ConflictScanOptions{Root: from})
	if err != nil {
		return plan, fmt.Errorf("store migrate: scan source conflicts: %w", err)
	}
	if len(conflicts.Conflicts) > 0 {
		for _, conflict := range conflicts.Conflicts {
			plan.Blockers = append(plan.Blockers, fmt.Sprintf("source contains provider conflict copy: %s", conflict.RelativePath))
		}
		return plan, fmt.Errorf("store migrate: resolve provider conflict copies before migration")
	}

	if err := filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk source %s: %w", path, err)
		}
		if path == from {
			return nil
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ".loki-operation.lock" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat source %s: %w", path, err)
		}
		kind := entryKind(info)
		entryPlan := Entry{
			RelativePath: rel,
			SourcePath:   path,
			DestPath:     filepath.Join(to, filepath.FromSlash(rel)),
			Kind:         kind,
			Mode:         info.Mode().String(),
		}
		switch kind {
		case "directory":
			plan.Summary.DirCount++
		case "file":
			plan.Summary.FileCount++
			entryPlan.Size = info.Size()
			plan.Summary.ByteCount += info.Size()
		case "symlink":
			plan.Summary.SymlinkCount++
		default:
			plan.Blockers = append(plan.Blockers, fmt.Sprintf("unsupported filesystem entry %s: %s", rel, kind))
		}
		plan.Entries = append(plan.Entries, entryPlan)
		return nil
	}); err != nil {
		return plan, err
	}
	sort.Slice(plan.Entries, func(i, j int) bool { return plan.Entries[i].RelativePath < plan.Entries[j].RelativePath })
	if len(plan.Blockers) > 0 {
		return plan, fmt.Errorf("store migrate: source contains unsupported entries")
	}
	plan.CanMigrate = true
	return plan, nil
}

func entryKind(info fs.FileInfo) string {
	mode := info.Mode()
	if mode&os.ModeSymlink != 0 {
		return "symlink"
	}
	if info.IsDir() {
		return "directory"
	}
	if mode.IsRegular() {
		return "file"
	}
	return mode.String()
}

func sameOrNestedPath(parent, child string) (same bool, nested bool, err error) {
	parentResolved, err := resolvePathForNesting(parent)
	if err != nil {
		return false, false, fmt.Errorf("resolve path %s: %w", parent, err)
	}
	childResolved, err := resolvePathForNesting(child)
	if err != nil {
		return false, false, fmt.Errorf("resolve path %s: %w", child, err)
	}
	if parentResolved == childResolved {
		return true, false, nil
	}
	rel, err := filepath.Rel(parentResolved, childResolved)
	if err != nil {
		return false, false, err
	}
	if rel == "." {
		return true, false, nil
	}
	if rel != "" && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, true, nil
	}
	return false, false, nil
}

func resolvePathForNesting(path string) (string, error) {
	pathAbs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(pathAbs); err == nil {
		return filepath.Clean(resolved), nil
	}
	missing := []string{}
	current := pathAbs
	for {
		if _, err := os.Lstat(current); err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			parts := append([]string{filepath.Clean(resolved)}, missing...)
			return filepath.Join(parts...), nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return pathAbs, nil
		}
		missing = append([]string{filepath.Base(current)}, missing...)
		current = parent
	}
}
