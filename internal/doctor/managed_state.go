package doctor

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/manifest"
	"github.com/asudbring/loki-profile-manager/internal/profile"
	"github.com/asudbring/loki-profile-manager/internal/store"
)

type managedStateManifestEntry struct {
	ID            string
	SourcePath    string
	SourcePaths   []string
	SourceEntries []activation.Source
	TargetPath    string
	Mode          string
	Format        string
	LayerKind     string
	LayerName     string
}

type managedStateCandidate struct {
	Record       activation.ManagedTarget
	Current      managedStateManifestEntry
	TargetHash   string
	Equivalent   bool
	ByteEqual    bool
	Reason       string
	Repairable   bool
	Unrepairable string
}

func addManagedStateChecks(ctx context.Context, report *Report, req Request, now time.Time) {
	if req.RepairManagedState {
		addManagedStateRepairChecks(ctx, report, req, now)
		return
	}
	candidates, scanErr := findManagedStateCandidates(ctx, req.Database, req.StorePath, req.Resolver)
	if scanErr != nil {
		report.add(Check{Severity: SeverityWarning, Code: "managed_state.scan_failed", Category: "managed_state", Message: scanErr.Error(), Remediation: "Run `loki doctor --json` for details, then inspect local state and manifests."})
		return
	}
	stale, repairable, unrepairable := splitManagedStateCandidates(candidates)
	if len(stale) == 0 {
		report.add(Check{Severity: SeverityInfo, Code: "managed_state.ok", Category: "managed_state", Message: "managed target state matches current files and manifests"})
		return
	}
	report.add(Check{Severity: SeverityWarning, Code: "managed_state.stale", Category: "managed_state", Message: fmt.Sprintf("%d managed target state record(s) are stale", len(stale)), Path: stale[0].Record.TargetPath, Remediation: "Run `loki doctor --repair-managed-state` to refresh safe local state records; add `--write-safe-files` to canonicalize safe local files.", Details: managedStateDetails(len(stale), len(repairable), len(unrepairable), 0)})
}

func addManagedStateRepairChecks(ctx context.Context, report *Report, req Request, now time.Time) {
	var stale []managedStateCandidate
	var repairable []managedStateCandidate
	var unrepairable []managedStateCandidate
	repaired := 0
	wroteFiles := 0
	repairErr := store.WithOperationLock(ctx, req.StorePath, store.OperationLockOptions{Operation: "doctor managed-state repair", Timeout: time.Second}, func() error {
		candidates, scanErr := findManagedStateCandidates(ctx, req.Database, req.StorePath, req.Resolver)
		if scanErr != nil {
			return scanErr
		}
		stale, repairable, unrepairable = splitManagedStateCandidates(candidates)
		for _, candidate := range repairable {
			wrote, err := repairManagedStateCandidate(ctx, req.Database, candidate, req.WriteSafeFiles, now)
			if err != nil {
				candidate.Unrepairable = err.Error()
				unrepairable = append(unrepairable, candidate)
				continue
			}
			repaired++
			if wrote {
				wroteFiles++
			}
		}
		return nil
	})
	if repairErr != nil {
		report.add(Check{Severity: SeverityWarning, Code: "managed_state.repair_skipped_locked", Category: "managed_state", Message: repairErr.Error(), Path: storeOperationLockPath(req.StorePath), Remediation: "Wait for the active Loki command to finish, then rerun `loki doctor --repair-managed-state`."})
		return
	}
	if len(stale) == 0 {
		report.add(Check{Severity: SeverityInfo, Code: "managed_state.ok", Category: "managed_state", Message: "managed target state matches current files and manifests"})
		return
	}
	if repaired > 0 {
		details := managedStateDetails(len(stale), len(repairable), len(unrepairable), wroteFiles)
		details["repaired"] = strconv.Itoa(repaired)
		report.add(Check{Severity: SeverityInfo, Code: "managed_state.repaired", Category: "managed_state", Message: fmt.Sprintf("repaired %d managed target state record(s)", repaired), Path: repairable[0].Record.TargetPath, Details: details})
	}
	if len(unrepairable) > 0 {
		report.add(Check{Severity: SeverityWarning, Code: "managed_state.unrepairable", Category: "managed_state", Message: fmt.Sprintf("%d stale managed target state record(s) need manual resolution", len(unrepairable)), Path: unrepairable[0].Record.TargetPath, Remediation: "Resolve the target/source difference manually, then rerun `loki switch` or `loki doctor --repair-managed-state`.", Details: managedStateDetails(len(stale), len(repairable), len(unrepairable), wroteFiles)})
	}
}

func splitManagedStateCandidates(candidates []managedStateCandidate) ([]managedStateCandidate, []managedStateCandidate, []managedStateCandidate) {
	stale := []managedStateCandidate{}
	repairable := []managedStateCandidate{}
	unrepairable := []managedStateCandidate{}
	for _, candidate := range candidates {
		stale = append(stale, candidate)
		if candidate.Repairable {
			repairable = append(repairable, candidate)
		} else {
			unrepairable = append(unrepairable, candidate)
		}
	}
	return stale, repairable, unrepairable
}

func findManagedStateCandidates(ctx context.Context, database *sql.DB, storePath string, resolver config.PathResolver) ([]managedStateCandidate, error) {
	records, err := activation.ListManagedTargets(ctx, database)
	if err != nil {
		return nil, err
	}
	entries, err := currentManagedStateManifestEntries(storePath, resolver)
	if err != nil {
		return nil, err
	}
	byTargetSource := map[string][]managedStateManifestEntry{}
	byTarget := map[string][]managedStateManifestEntry{}
	for _, entry := range entries {
		key := managedStateKey(entry.TargetPath, entry.SourcePath)
		byTargetSource[key] = append(byTargetSource[key], entry)
		byTarget[filepath.Clean(entry.TargetPath)] = append(byTarget[filepath.Clean(entry.TargetPath)], entry)
	}
	candidates := []managedStateCandidate{}
	for _, record := range records {
		if strings.TrimSpace(record.TargetPath) == "" {
			continue
		}
		current, ok := currentManagedStateEntryForRecord(record, byTargetSource, byTarget)
		if !ok {
			continue
		}
		targetHash, err := activation.HashPath(record.TargetPath)
		if err != nil {
			continue
		}
		staleHash := record.ContentHash != targetHash
		staleMode := record.Mode != current.Mode
		staleLayer := record.LayerKind != current.LayerKind || record.LayerName != current.LayerName
		if !staleHash && !staleMode && !staleLayer {
			continue
		}
		equivalent, byteEqual, reason := managedStateEquivalent(current, record.TargetPath)
		candidate := managedStateCandidate{Record: record, Current: current, TargetHash: targetHash, Equivalent: equivalent, ByteEqual: byteEqual, Reason: reason}
		if equivalent && current.Mode == manifest.ModeMerge && len(byTarget[filepath.Clean(record.TargetPath)]) > 1 && len(current.SourcePaths) == 0 {
			candidate.Unrepairable = "merge target has multiple manifest sources"
		} else if equivalent && managedStateRepairModeSupported(current.Mode) {
			candidate.Repairable = true
		} else if !equivalent {
			candidate.Unrepairable = "source and target differ"
		} else {
			candidate.Unrepairable = fmt.Sprintf("mode %q is not safely repairable", current.Mode)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func currentManagedStateEntryForRecord(record activation.ManagedTarget, byTargetSource map[string][]managedStateManifestEntry, byTarget map[string][]managedStateManifestEntry) (managedStateManifestEntry, bool) {
	if strings.TrimSpace(record.SourcePath) != "" {
		currentEntries := byTargetSource[managedStateKey(record.TargetPath, record.SourcePath)]
		if len(currentEntries) != 1 {
			return managedStateManifestEntry{}, false
		}
		return currentEntries[0], true
	}
	return currentManagedStateMergeEntryForRecord(record, byTarget[filepath.Clean(record.TargetPath)])
}

func currentManagedStateMergeEntryForRecord(record activation.ManagedTarget, entries []managedStateManifestEntry) (managedStateManifestEntry, bool) {
	if len(entries) < 2 || strings.TrimSpace(record.MetadataJSON) == "" {
		return managedStateManifestEntry{}, false
	}
	var metadata struct {
		Sources []activation.Source `json:"sources"`
	}
	if err := json.Unmarshal([]byte(record.MetadataJSON), &metadata); err != nil || len(metadata.Sources) != len(entries) {
		return managedStateManifestEntry{}, false
	}
	format := entries[0].Format
	sourcePaths := make([]string, 0, len(entries))
	sourceEntries := make([]activation.Source, 0, len(entries))
	for i, entry := range entries {
		if entry.Mode != manifest.ModeMerge || entry.Format != format || filepath.Clean(metadata.Sources[i].Path) != filepath.Clean(entry.SourcePath) {
			return managedStateManifestEntry{}, false
		}
		sourcePaths = append(sourcePaths, entry.SourcePath)
		sourceEntries = append(sourceEntries, activation.Source{Path: entry.SourcePath, LayerName: entry.LayerName, LayerKind: entry.LayerKind, FileID: entry.ID, Order: i})
	}
	return managedStateManifestEntry{ID: entries[0].ID, SourcePaths: sourcePaths, SourceEntries: sourceEntries, TargetPath: record.TargetPath, Mode: manifest.ModeMerge, Format: format, LayerKind: "merged", LayerName: "merged"}, true
}

func currentManagedStateManifestEntries(storePath string, resolver config.PathResolver) ([]managedStateManifestEntry, error) {
	layers, err := profile.LoadAllManifests(storePath)
	if err != nil {
		return nil, err
	}
	entries := []managedStateManifestEntry{}
	for _, layer := range layers {
		expander := manifest.Expander{Resolver: resolver, Targets: layer.Manifest.Targets}
		result := manifest.ValidateLayer(manifest.ValidationInput{LayerName: layer.Name, LayerRoot: layer.RootDir, Manifest: layer.Manifest, Expander: expander})
		for _, op := range result.Operations {
			entries = append(entries, managedStateManifestEntry{ID: op.Entry.ID, SourcePath: op.SourcePath, TargetPath: op.TargetPath, Mode: op.Entry.Mode, Format: op.Entry.Format, LayerKind: string(layer.Kind), LayerName: layer.Name})
		}
	}
	return entries, nil
}

func managedStateKey(targetPath, sourcePath string) string {
	return filepath.Clean(targetPath) + "\x00" + filepath.Clean(sourcePath)
}

func managedStateEquivalent(current managedStateManifestEntry, targetPath string) (bool, bool, string) {
	targetHash, targetErr := activation.HashPath(targetPath)
	if targetErr != nil {
		return false, false, "target_unreadable"
	}
	if len(current.SourcePaths) > 0 {
		content, err := activation.MergeBytes(current.Format, current.SourcePaths)
		if err != nil {
			return false, false, "merge_failed"
		}
		if activation.HashBytes(content) == targetHash {
			return true, true, "hash_equal"
		}
		if current.Format == manifest.FormatJSON && jsonSemanticEqualBytes(content, targetPath) {
			return true, false, "json_semantic_equal"
		}
		return false, false, "different"
	}
	sourceHash, sourceErr := activation.HashPath(current.SourcePath)
	if sourceErr == nil && sourceHash == targetHash {
		return true, true, "hash_equal"
	}
	if current.Format == manifest.FormatJSON && jsonSemanticEqual(current.SourcePath, targetPath) {
		return true, false, "json_semantic_equal"
	}
	return false, false, "different"
}

func jsonSemanticEqual(leftPath, rightPath string) bool {
	leftContent, err := os.ReadFile(leftPath)
	if err != nil {
		return false
	}
	return jsonSemanticEqualBytes(leftContent, rightPath)
}

func jsonSemanticEqualBytes(leftContent []byte, rightPath string) bool {
	rightContent, err := os.ReadFile(rightPath)
	if err != nil {
		return false
	}
	if bytes.Equal(leftContent, rightContent) {
		return true
	}
	left, err := parseJSONWithNumbers(leftContent)
	if err != nil {
		return false
	}
	right, err := parseJSONWithNumbers(rightContent)
	if err != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}

func parseJSONWithNumbers(content []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func managedStateRepairModeSupported(mode string) bool {
	switch mode {
	case manifest.ModeCopy, manifest.ModeMerge:
		return true
	default:
		return false
	}
}

func repairManagedStateCandidate(ctx context.Context, database *sql.DB, candidate managedStateCandidate, writeSafeFiles bool, now time.Time) (bool, error) {
	wrote := false
	if writeSafeFiles && !candidate.ByteEqual {
		switch candidate.Current.Mode {
		case manifest.ModeCopy:
			if err := activation.CopyPath(candidate.Current.SourcePath, candidate.Record.TargetPath); err != nil {
				return false, err
			}
			wrote = true
		case manifest.ModeMerge:
			sourcePaths := candidate.Current.SourcePaths
			if len(sourcePaths) == 0 {
				sourcePaths = []string{candidate.Current.SourcePath}
			}
			if err := activation.WriteMerge(candidate.Current.Format, sourcePaths, candidate.Record.TargetPath); err != nil {
				return false, err
			}
			wrote = true
		default:
			return false, fmt.Errorf("mode %q cannot be written safely", candidate.Current.Mode)
		}
	}
	hash, err := activation.HashPath(candidate.Record.TargetPath)
	if err != nil {
		return wrote, err
	}
	metadataMap := map[string]any{
		"doctor_repair":   true,
		"previous_mode":   candidate.Record.Mode,
		"previous_hash":   candidate.Record.ContentHash,
		"repair_reason":   candidate.Reason,
		"operation_id":    candidate.Current.ID,
		"write_safe_file": wrote,
	}
	if len(candidate.Current.SourceEntries) > 0 {
		metadataMap["sources"] = candidate.Current.SourceEntries
	} else if strings.TrimSpace(candidate.Record.MetadataJSON) != "" {
		var existing map[string]any
		if err := json.Unmarshal([]byte(candidate.Record.MetadataJSON), &existing); err == nil {
			if sources, ok := existing["sources"]; ok {
				metadataMap["sources"] = sources
			}
		}
	}
	metadata, err := json.Marshal(metadataMap)
	if err != nil {
		return wrote, err
	}
	record := candidate.Record
	record.SourcePath = candidate.Current.SourcePath
	record.Mode = candidate.Current.Mode
	record.ContentHash = hash
	record.LayerKind = candidate.Current.LayerKind
	record.LayerName = candidate.Current.LayerName
	record.LastAppliedAt = now.UTC().Format(time.RFC3339)
	record.MetadataJSON = string(metadata)
	return wrote, activation.PutManagedTarget(ctx, database, record)
}

func managedStateDetails(stale, repairable, unrepairable, wroteFiles int) map[string]string {
	return map[string]string{
		"stale":        strconv.Itoa(stale),
		"repairable":   strconv.Itoa(repairable),
		"unrepairable": strconv.Itoa(unrepairable),
		"wrote_files":  strconv.Itoa(wroteFiles),
	}
}

func storeOperationLockPath(storePath string) string {
	return store.OperationLockPath(storePath)
}
