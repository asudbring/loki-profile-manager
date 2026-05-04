package activation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	Snapshot     Snapshot              `json:"snapshot"`
	TargetFilter string                `json:"target_filter,omitempty"`
	Targets      []RestoreDryRunTarget `json:"targets"`
	Warnings     []string              `json:"warnings,omitempty"`
	Blockers     []string              `json:"blockers,omitempty"`
	CanRestore   bool                  `json:"can_restore"`
	Fingerprint  string                `json:"fingerprint,omitempty"`
}

type RestorePlanOptions struct {
	Target string
}

type RestoreDryRunTarget struct {
	Entry               SnapshotEntry `json:"entry"`
	Action              string        `json:"action"`
	CurrentExists       bool          `json:"current_exists"`
	CurrentKind         string        `json:"current_kind,omitempty"`
	CurrentMode         string        `json:"current_mode,omitempty"`
	CurrentHash         string        `json:"current_hash,omitempty"`
	SnapshotHash        string        `json:"snapshot_hash,omitempty"`
	SensitivePath       bool          `json:"sensitive_path"`
	SensitiveLinkTarget bool          `json:"sensitive_link_target,omitempty"`
	Warnings            []string      `json:"warnings,omitempty"`
	Blockers            []string      `json:"blockers,omitempty"`
}

func BuildRestoreDryRunPlan(ctx context.Context, snapshot Snapshot) (RestoreDryRunPlan, error) {
	return BuildRestoreDryRunPlanWithOptions(ctx, snapshot, RestorePlanOptions{})
}

func BuildRestoreDryRunPlanWithOptions(ctx context.Context, snapshot Snapshot, opts RestorePlanOptions) (RestoreDryRunPlan, error) {
	plan := RestoreDryRunPlan{Snapshot: snapshotWithDefaults(snapshot, snapshot.SnapshotID, snapshot.Path), TargetFilter: strings.TrimSpace(opts.Target)}
	entries, err := filteredSnapshotEntries(plan.Snapshot.Targets, plan.TargetFilter)
	if err != nil {
		return RestoreDryRunPlan{}, err
	}
	for _, entry := range entries {
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
		for _, blocker := range target.Blockers {
			plan.Blockers = append(plan.Blockers, fmt.Sprintf("%s: %s", target.Action, blocker))
		}
	}
	plan.CanRestore = len(plan.Blockers) == 0
	fingerprint, err := RestorePlanFingerprint(plan)
	if err != nil {
		return RestoreDryRunPlan{}, err
	}
	plan.Fingerprint = fingerprint
	return plan, nil
}

func filteredSnapshotEntries(entries []SnapshotEntry, targetFilter string) ([]SnapshotEntry, error) {
	targetFilter = strings.TrimSpace(targetFilter)
	if targetFilter == "" {
		return entries, nil
	}
	matches := []SnapshotEntry{}
	for _, entry := range entries {
		if restorePathMatches(entry.TargetPath, targetFilter) {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("snapshots restore: target not found in snapshot")
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("snapshots restore: target filter matched multiple snapshot entries")
	}
	return matches, nil
}

func restorePathMatches(snapshotTarget, targetFilter string) bool {
	if snapshotTarget == "" || targetFilter == "" {
		return false
	}
	if cleanPathForRestoreMatch(snapshotTarget) == cleanPathForRestoreMatch(targetFilter) {
		return true
	}
	snapshotAbs, snapshotErr := filepath.Abs(snapshotTarget)
	filterAbs, filterErr := filepath.Abs(targetFilter)
	if snapshotErr == nil && filterErr == nil && cleanPathForRestoreMatch(snapshotAbs) == cleanPathForRestoreMatch(filterAbs) {
		return true
	}
	return false
}

func cleanPathForRestoreMatch(value string) string {
	return filepath.Clean(strings.TrimSpace(value))
}

func ValidateRestorePlan(plan RestoreDryRunPlan) error {
	if len(plan.Blockers) == 0 {
		return nil
	}
	return fmt.Errorf("snapshot restore blocked: %s", strings.Join(plan.Blockers, "; "))
}

func RestorePlanFingerprint(plan RestoreDryRunPlan) (string, error) {
	targets := make([]restorePlanFingerprintTarget, 0, len(plan.Targets))
	for _, target := range plan.Targets {
		targets = append(targets, restorePlanFingerprintTarget{
			TargetPath:          target.Entry.TargetPath,
			Kind:                target.Entry.Kind,
			Action:              target.Action,
			SnapshotPath:        target.Entry.SnapshotPath,
			Hash:                target.Entry.Hash,
			ExpectedHash:        target.Entry.ExpectedHash,
			ExpectedMode:        target.Entry.ExpectedMode,
			LinkTarget:          target.Entry.LinkTarget,
			CurrentExists:       target.CurrentExists,
			CurrentKind:         target.CurrentKind,
			CurrentMode:         target.CurrentMode,
			CurrentHash:         target.CurrentHash,
			SnapshotHash:        target.SnapshotHash,
			SensitivePath:       target.SensitivePath,
			SensitiveLinkTarget: target.SensitiveLinkTarget,
			Blockers:            cloneStrings(target.Blockers),
		})
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].TargetPath == targets[j].TargetPath {
			return targets[i].Kind < targets[j].Kind
		}
		return targets[i].TargetPath < targets[j].TargetPath
	})
	payload := restorePlanFingerprintPayload{
		Version:      2,
		SnapshotID:   plan.Snapshot.SnapshotID,
		CreatedAt:    plan.Snapshot.CreatedAt,
		Path:         plan.Snapshot.Path,
		TargetFilter: plan.TargetFilter,
		CanRestore:   plan.CanRestore,
		Blockers:     cloneStrings(plan.Blockers),
		Targets:      targets,
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal restore plan fingerprint: %w", err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}

type restorePlanFingerprintPayload struct {
	Version      int                            `json:"version"`
	SnapshotID   string                         `json:"snapshot_id"`
	CreatedAt    string                         `json:"created_at"`
	Path         string                         `json:"path"`
	TargetFilter string                         `json:"target_filter,omitempty"`
	CanRestore   bool                           `json:"can_restore"`
	Blockers     []string                       `json:"blockers,omitempty"`
	Targets      []restorePlanFingerprintTarget `json:"targets"`
}

type restorePlanFingerprintTarget struct {
	TargetPath          string   `json:"target_path"`
	Kind                string   `json:"kind"`
	Action              string   `json:"action"`
	SnapshotPath        string   `json:"snapshot_path,omitempty"`
	Hash                string   `json:"hash,omitempty"`
	ExpectedHash        string   `json:"expected_hash,omitempty"`
	ExpectedMode        string   `json:"expected_mode,omitempty"`
	LinkTarget          string   `json:"link_target,omitempty"`
	CurrentExists       bool     `json:"current_exists"`
	CurrentKind         string   `json:"current_kind,omitempty"`
	CurrentMode         string   `json:"current_mode,omitempty"`
	CurrentHash         string   `json:"current_hash,omitempty"`
	SnapshotHash        string   `json:"snapshot_hash,omitempty"`
	SensitivePath       bool     `json:"sensitive_path"`
	SensitiveLinkTarget bool     `json:"sensitive_link_target,omitempty"`
	Blockers            []string `json:"blockers,omitempty"`
}

func inspectRestoreDryRunTarget(snapshotPath string, entry SnapshotEntry) RestoreDryRunTarget {
	target := RestoreDryRunTarget{
		Entry:               entry,
		Action:              restoreAction(entry.Kind),
		CurrentKind:         RestoreCurrentKindMissing,
		SensitivePath:       PathLooksSensitive(entry.TargetPath),
		SensitiveLinkTarget: PathLooksSensitive(entry.LinkTarget),
	}
	if target.SensitivePath {
		target.addBlocker("sensitive-looking target path is blocked by default")
	}
	if target.SensitiveLinkTarget {
		target.addBlocker("sensitive-looking symlink target is blocked by default")
	}

	info, err := os.Lstat(entry.TargetPath)
	if errors.Is(err, os.ErrNotExist) {
		target.CurrentExists = false
	} else if err != nil {
		target.Action = RestoreActionUnknown
		target.CurrentKind = RestoreCurrentKindOther
		target.addBlocker("could not inspect current target")
		return target
	} else {
		target.CurrentExists = true
		target.CurrentKind = restoreCurrentKind(info)
		target.CurrentMode = info.Mode().String()
		if target.SensitivePath {
			target.Warnings = append(target.Warnings, "sensitive-looking target path; current hash not computed")
		} else if target.CurrentKind != RestoreCurrentKindSymlink {
			if hash, hashErr := HashPath(entry.TargetPath); hashErr != nil {
				target.addBlocker("could not hash current target")
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
			target.addBlocker("target was missing before activation and no expected hash was recorded")
		} else if target.SensitivePath {
			target.addBlocker("expected hash check skipped for sensitive-looking target path")
		} else if target.CurrentHash != "" && target.CurrentHash != entry.ExpectedHash {
			target.addBlocker("current hash differs from Loki-created expected hash")
		}
		if entry.ExpectedMode != "" && target.CurrentMode != "" && target.CurrentMode != entry.ExpectedMode {
			target.addBlocker("current mode differs from Loki-created expected mode")
		}
	case "file", "directory":
		if entry.SnapshotPath == "" {
			target.addBlocker("snapshot entry path is missing")
		} else if !snapshotEntryPathInsideSnapshot(snapshotPath, entry.SnapshotPath) {
			target.addBlocker("snapshot entry path is outside snapshot directory")
		} else if info, statErr := os.Lstat(entry.SnapshotPath); statErr != nil {
			target.addBlocker("snapshot entry is not readable")
		} else if info.Mode()&os.ModeSymlink != 0 {
			target.addBlocker("snapshot entry is a symlink and was not followed")
		} else if entry.Hash == "" {
			target.addBlocker("snapshot entry hash is missing")
		} else if hash, hashErr := HashPath(entry.SnapshotPath); hashErr != nil {
			target.addBlocker("could not hash snapshot entry")
		} else {
			target.SnapshotHash = hash
			if hash != entry.Hash {
				target.addBlocker("snapshot entry hash differs from metadata")
			}
		}
	case "symlink":
		if entry.LinkTarget == "" {
			target.addBlocker("snapshot symlink target is missing")
		}
	case "":
		target.addBlocker("snapshot entry kind is missing")
	default:
		target.addBlocker(fmt.Sprintf("unknown snapshot entry kind %q", entry.Kind))
	}
	return target
}

func (target *RestoreDryRunTarget) addBlocker(message string) {
	target.Blockers = append(target.Blockers, message)
	target.Warnings = append(target.Warnings, message)
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
