package activation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	RestoreActionRestoreFile             = "restore_file"
	RestoreActionRestoreDirectory        = "restore_directory"
	RestoreActionRestoreSymlink          = "restore_symlink"
	RestoreActionRemoveCreatedTarget     = "remove_created_target"
	RestoreActionSkipMissingTargetAbsent = "skip_missing_target_already_absent"
	RestoreActionUnknown                 = "unknown"
	RestoreCurrentKindMissing            = "missing"
	RestoreCurrentKindFile               = "file"
	RestoreCurrentKindDirectory          = "directory"
	RestoreCurrentKindSymlink            = "symlink"
	RestoreCurrentKindOther              = "other"
)

type RestoreDryRunPlan struct {
	Snapshot Snapshot              `json:"snapshot"`
	Targets  []RestoreDryRunTarget `json:"targets"`
	Warnings []string              `json:"warnings,omitempty"`
}

type RestoreDryRunTarget struct {
	Entry               SnapshotEntry `json:"entry"`
	Action              string        `json:"action"`
	CurrentExists       bool          `json:"current_exists"`
	CurrentKind         string        `json:"current_kind,omitempty"`
	CurrentMode         string        `json:"current_mode,omitempty"`
	CurrentHash         string        `json:"current_hash,omitempty"`
	SensitivePath       bool          `json:"sensitive_path"`
	SensitiveLinkTarget bool          `json:"sensitive_link_target,omitempty"`
	Warnings            []string      `json:"warnings,omitempty"`
}

func BuildRestoreDryRunPlan(ctx context.Context, snapshot Snapshot) (RestoreDryRunPlan, error) {
	plan := RestoreDryRunPlan{Snapshot: snapshotWithDefaults(snapshot, snapshot.SnapshotID, snapshot.Path)}
	for _, entry := range plan.Snapshot.Targets {
		select {
		case <-ctx.Done():
			return RestoreDryRunPlan{}, ctx.Err()
		default:
		}
		target := inspectRestoreDryRunTarget(plan.Snapshot.Path, entry)
		plan.Targets = append(plan.Targets, target)
		for _, warning := range target.Warnings {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s: %s", target.Action, warning))
		}
	}
	return plan, nil
}

func inspectRestoreDryRunTarget(snapshotPath string, entry SnapshotEntry) RestoreDryRunTarget {
	target := RestoreDryRunTarget{
		Entry:               entry,
		Action:              restoreAction(entry.Kind),
		CurrentKind:         RestoreCurrentKindMissing,
		SensitivePath:       PathLooksSensitive(entry.TargetPath),
		SensitiveLinkTarget: PathLooksSensitive(entry.LinkTarget),
	}
	info, err := os.Lstat(entry.TargetPath)
	if errors.Is(err, os.ErrNotExist) {
		target.CurrentExists = false
	} else if err != nil {
		target.Action = RestoreActionUnknown
		target.CurrentKind = RestoreCurrentKindOther
		target.Warnings = append(target.Warnings, "could not inspect current target")
		return target
	} else {
		target.CurrentExists = true
		target.CurrentKind = restoreCurrentKind(info)
		target.CurrentMode = info.Mode().String()
		if target.SensitivePath {
			target.Warnings = append(target.Warnings, "sensitive-looking target path; current hash not computed")
		} else if target.CurrentKind != RestoreCurrentKindSymlink {
			if hash, hashErr := HashPath(entry.TargetPath); hashErr != nil {
				target.Warnings = append(target.Warnings, fmt.Sprintf("could not hash current target: %v", hashErr))
			} else {
				target.CurrentHash = hash
			}
		}
	}

	switch entry.Kind {
	case "missing":
		if !target.CurrentExists {
			target.Action = RestoreActionSkipMissingTargetAbsent
			return target
		}
		if entry.ExpectedHash == "" {
			target.Warnings = append(target.Warnings, "target was missing before activation and no expected hash was recorded")
		} else if target.SensitivePath {
			target.Warnings = append(target.Warnings, "expected hash check skipped for sensitive-looking target path")
		} else if target.CurrentHash != "" && target.CurrentHash != entry.ExpectedHash {
			target.Warnings = append(target.Warnings, "current hash differs from Loki-created expected hash")
		}
		if entry.ExpectedMode != "" && target.CurrentMode != "" && target.CurrentMode != entry.ExpectedMode {
			target.Warnings = append(target.Warnings, "current mode differs from Loki-created expected mode")
		}
	case "file", "directory":
		if entry.SnapshotPath == "" {
			target.Warnings = append(target.Warnings, "snapshot entry path is missing")
		} else if !snapshotEntryPathInsideSnapshot(snapshotPath, entry.SnapshotPath) {
			target.Warnings = append(target.Warnings, "snapshot entry path is outside snapshot directory")
		} else if info, statErr := os.Lstat(entry.SnapshotPath); statErr != nil {
			target.Warnings = append(target.Warnings, "snapshot entry is not readable")
		} else if info.Mode()&os.ModeSymlink != 0 {
			target.Warnings = append(target.Warnings, "snapshot entry is a symlink and was not followed")
		}
	case "symlink":
		if entry.LinkTarget == "" {
			target.Warnings = append(target.Warnings, "snapshot symlink target is missing")
		}
		if target.SensitiveLinkTarget {
			target.Warnings = append(target.Warnings, "sensitive-looking symlink target redacted")
		}
	case "":
		target.Warnings = append(target.Warnings, "snapshot entry kind is missing")
	default:
		target.Warnings = append(target.Warnings, fmt.Sprintf("unknown snapshot entry kind %q", entry.Kind))
	}
	return target
}

func restoreAction(kind string) string {
	switch kind {
	case "file":
		return RestoreActionRestoreFile
	case "directory":
		return RestoreActionRestoreDirectory
	case "symlink":
		return RestoreActionRestoreSymlink
	case "missing":
		return RestoreActionRemoveCreatedTarget
	default:
		return RestoreActionUnknown
	}
}

func snapshotEntryPathInsideSnapshot(snapshotPath, entryPath string) bool {
	if snapshotPath == "" || entryPath == "" {
		return false
	}
	snapshotAbs, snapshotErr := filepath.Abs(snapshotPath)
	entryAbs, entryErr := filepath.Abs(entryPath)
	if snapshotErr != nil || entryErr != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(snapshotAbs), filepath.Clean(entryAbs))
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func restoreCurrentKind(info os.FileInfo) string {
	if info == nil {
		return RestoreCurrentKindMissing
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return RestoreCurrentKindSymlink
	}
	if info.IsDir() {
		return RestoreCurrentKindDirectory
	}
	if info.Mode().IsRegular() {
		return RestoreCurrentKindFile
	}
	return RestoreCurrentKindOther
}

func PathLooksSensitive(value string) bool {
	if value == "" {
		return false
	}
	lower := strings.ToLower(filepath.ToSlash(value))
	for _, term := range []string{"/.ssh", ".env", "token", "credential", "credentials", "private", "private_key", "id_rsa", "id_ed25519", ".pem", ".key"} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}
