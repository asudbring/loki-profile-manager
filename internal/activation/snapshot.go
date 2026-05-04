package activation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const SnapshotMetadataFile = "metadata.json"

type Snapshot struct {
	SnapshotID            string                  `json:"snapshot_id"`
	MachineID             string                  `json:"machine_id,omitempty"`
	Path                  string                  `json:"path"`
	CreatedAt             string                  `json:"created_at"`
	PreviousActiveProfile string                  `json:"previous_active_profile,omitempty"`
	PreviousActiveBuckets []string                `json:"previous_active_buckets"`
	Targets               []SnapshotEntry         `json:"targets"`
	ManagedTargets        []ManagedTargetSnapshot `json:"managed_targets,omitempty"`
}

type SnapshotEntry struct {
	TargetPath   string `json:"path"`
	Kind         string `json:"kind"`
	Hash         string `json:"hash,omitempty"`
	ExpectedHash string `json:"expected_hash,omitempty"`
	ExpectedMode string `json:"expected_mode,omitempty"`
	SnapshotPath string `json:"snapshot_path,omitempty"`
	LinkTarget   string `json:"link_target,omitempty"`
}

type ManagedTargetSnapshot struct {
	TargetPath string        `json:"target_path"`
	Found      bool          `json:"found"`
	Record     ManagedTarget `json:"record,omitempty"`
}

type SnapshotSummary struct {
	SnapshotID            string   `json:"snapshot_id"`
	MachineID             string   `json:"machine_id,omitempty"`
	Path                  string   `json:"path"`
	CreatedAt             string   `json:"created_at"`
	PreviousActiveProfile string   `json:"previous_active_profile,omitempty"`
	PreviousActiveBuckets []string `json:"previous_active_buckets"`
	TargetCount           int      `json:"target_count"`
	TargetKinds           []string `json:"target_kinds,omitempty"`
	Exists                bool     `json:"exists"`
	MetadataError         string   `json:"metadata_error,omitempty"`
}

type CreateSnapshotRequest struct {
	Database              *sql.DB
	SnapshotRoot          string
	MachineID             string
	Plan                  Plan
	PreviousActiveProfile string
	PreviousActiveBuckets []string
	Now                   func() time.Time
	Keep                  int
}

func CreateSnapshot(ctx context.Context, req CreateSnapshotRequest) (Snapshot, error) {
	if req.SnapshotRoot == "" {
		return Snapshot{}, fmt.Errorf("create snapshot: snapshot root is required")
	}
	now := time.Now
	if req.Now != nil {
		now = req.Now
	}
	created := now().UTC()
	id := created.Format("20060102T150405Z") + "-" + uuid.NewString()
	root := filepath.Join(req.SnapshotRoot, id)
	entriesDir := filepath.Join(root, "entries")
	if err := os.MkdirAll(entriesDir, 0o700); err != nil {
		return Snapshot{}, fmt.Errorf("create snapshot directory %s: %w", root, err)
	}
	snapshot := Snapshot{
		SnapshotID:            id,
		MachineID:             req.MachineID,
		Path:                  root,
		CreatedAt:             created.Format(time.RFC3339),
		PreviousActiveProfile: req.PreviousActiveProfile,
		PreviousActiveBuckets: cloneStrings(req.PreviousActiveBuckets),
		Targets:               []SnapshotEntry{},
	}
	seen := map[string]bool{}
	for _, op := range req.Plan.Operations {
		if seen[op.TargetPath] {
			continue
		}
		seen[op.TargetPath] = true
		entry, err := snapshotTarget(op, entriesDir, len(snapshot.Targets))
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.Targets = append(snapshot.Targets, entry)
		if req.Database != nil {
			record, found, err := GetManagedTarget(ctx, req.Database, op.TargetPath)
			if err != nil {
				return Snapshot{}, err
			}
			snapshot.ManagedTargets = append(snapshot.ManagedTargets, ManagedTargetSnapshot{TargetPath: op.TargetPath, Found: found, Record: record})
		}
	}
	if err := writeSnapshotMetadata(snapshot); err != nil {
		return Snapshot{}, err
	}
	if req.Database != nil {
		if err := insertSnapshotRecord(ctx, req.Database, snapshot); err != nil {
			return Snapshot{}, err
		}
	}
	keep := req.Keep
	if keep <= 0 {
		keep = 2
	}
	if err := EnforceSnapshotRetention(ctx, req.Database, req.SnapshotRoot, keep); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func snapshotTarget(op Operation, entriesDir string, index int) (SnapshotEntry, error) {
	target := op.TargetPath
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return SnapshotEntry{TargetPath: target, Kind: "missing", ExpectedHash: op.ExpectedHash}, nil
	}
	if err != nil {
		return SnapshotEntry{}, fmt.Errorf("snapshot target %s: %w", target, err)
	}
	entry := SnapshotEntry{TargetPath: target}
	if info.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(target)
		if err != nil {
			return SnapshotEntry{}, fmt.Errorf("snapshot symlink %s: %w", target, err)
		}
		entry.Kind = "symlink"
		entry.LinkTarget = linkTarget
		return entry, nil
	}
	if info.IsDir() {
		entry.Kind = "directory"
	} else {
		entry.Kind = "file"
	}
	if hash, err := HashPath(target); err == nil {
		entry.Hash = hash
	}
	snapshotPath := filepath.Join(entriesDir, fmt.Sprintf("%03d", index))
	if err := copyPathContents(target, snapshotPath, info); err != nil {
		return SnapshotEntry{}, fmt.Errorf("snapshot copy %s: %w", target, err)
	}
	entry.SnapshotPath = snapshotPath
	return entry, nil
}

func writeSnapshotMetadata(snapshot Snapshot) error {
	content, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot metadata: %w", err)
	}
	return writeFileAtomic(filepath.Join(snapshot.Path, SnapshotMetadataFile), append(content, '\n'), 0o600)
}

func insertSnapshotRecord(ctx context.Context, database *sql.DB, snapshot Snapshot) error {
	content, buckets, err := marshalSnapshotRecord(snapshot)
	if err != nil {
		return err
	}
	_, err = database.ExecContext(ctx, `INSERT INTO snapshots (snapshot_id, machine_id, path, previous_active_profile, previous_active_buckets_json, created_at, metadata_json) VALUES (?, ?, ?, ?, ?, ?, ?)`, snapshot.SnapshotID, snapshot.MachineID, snapshot.Path, snapshot.PreviousActiveProfile, string(buckets), snapshot.CreatedAt, string(content))
	if err != nil {
		return fmt.Errorf("insert snapshot record %s: %w", snapshot.SnapshotID, err)
	}
	return nil
}

func PersistSnapshot(ctx context.Context, database *sql.DB, snapshot Snapshot) error {
	if snapshot.SnapshotID == "" {
		return fmt.Errorf("persist snapshot: snapshot id is required")
	}
	if snapshot.Path == "" {
		return fmt.Errorf("persist snapshot %s: snapshot path is required", snapshot.SnapshotID)
	}
	if err := writeSnapshotMetadata(snapshot); err != nil {
		return err
	}
	if database == nil {
		return nil
	}
	content, buckets, err := marshalSnapshotRecord(snapshot)
	if err != nil {
		return err
	}
	_, err = database.ExecContext(ctx, `UPDATE snapshots SET machine_id = ?, path = ?, previous_active_profile = ?, previous_active_buckets_json = ?, created_at = ?, metadata_json = ? WHERE snapshot_id = ?`, snapshot.MachineID, snapshot.Path, snapshot.PreviousActiveProfile, string(buckets), snapshot.CreatedAt, string(content), snapshot.SnapshotID)
	if err != nil {
		return fmt.Errorf("update snapshot record %s: %w", snapshot.SnapshotID, err)
	}
	return nil
}

func marshalSnapshotRecord(snapshot Snapshot) ([]byte, []byte, error) {
	content, err := json.Marshal(snapshot)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal snapshot database metadata: %w", err)
	}
	buckets, err := json.Marshal(snapshot.PreviousActiveBuckets)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal snapshot buckets: %w", err)
	}
	return content, buckets, nil
}

func ListSnapshots(ctx context.Context, database *sql.DB, snapshotRoot string) ([]SnapshotSummary, error) {
	summaries := map[string]SnapshotSummary{}
	if database != nil {
		rows, err := database.QueryContext(ctx, `SELECT snapshot_id, COALESCE(machine_id, ''), path, COALESCE(previous_active_profile, ''), COALESCE(previous_active_buckets_json, '[]'), created_at, COALESCE(metadata_json, '') FROM snapshots`)
		if err != nil {
			return nil, fmt.Errorf("list snapshot records: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var summary SnapshotSummary
			var bucketsJSON string
			var metadataJSON string
			if err := rows.Scan(&summary.SnapshotID, &summary.MachineID, &summary.Path, &summary.PreviousActiveProfile, &bucketsJSON, &summary.CreatedAt, &metadataJSON); err != nil {
				return nil, fmt.Errorf("scan snapshot record: %w", err)
			}
			summary.PreviousActiveBuckets = []string{}
			if bucketsJSON != "" {
				if err := json.Unmarshal([]byte(bucketsJSON), &summary.PreviousActiveBuckets); err != nil {
					summary.MetadataError = appendMetadataError(summary.MetadataError, fmt.Sprintf("parse previous buckets: %v", err))
				}
			}
			summary.Exists = snapshotDirExists(summary.Path)
			if metadataJSON != "" {
				var snapshot Snapshot
				if err := json.Unmarshal([]byte(metadataJSON), &snapshot); err != nil {
					summary.MetadataError = appendMetadataError(summary.MetadataError, fmt.Sprintf("parse database metadata: %v", err))
				} else {
					applySnapshotToSummary(&summary, snapshot)
				}
			} else if summary.Exists {
				if snapshot, err := readSnapshotMetadata(summary.Path); err != nil {
					summary.MetadataError = appendMetadataError(summary.MetadataError, err.Error())
				} else {
					applySnapshotToSummary(&summary, snapshot)
				}
			}
			summary.Exists = snapshotDirExists(summary.Path)
			summaries[summary.SnapshotID] = summary
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate snapshot records: %w", err)
		}
	}

	filesystemSummaries, err := listFilesystemSnapshotSummaries(snapshotRoot)
	if err != nil {
		return nil, err
	}
	for _, summary := range filesystemSummaries {
		if summary.SnapshotID == "" {
			continue
		}
		if _, exists := summaries[summary.SnapshotID]; exists {
			continue
		}
		summaries[summary.SnapshotID] = summary
	}

	out := make([]SnapshotSummary, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt == out[j].CreatedAt {
			return out[i].SnapshotID > out[j].SnapshotID
		}
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

func LoadSnapshot(ctx context.Context, database *sql.DB, snapshotRoot, snapshotID string) (Snapshot, error) {
	snapshotID = strings.TrimSpace(snapshotID)
	if err := validateSnapshotID(snapshotID); err != nil {
		return Snapshot{}, err
	}
	if database != nil {
		var path string
		var metadataJSON string
		err := database.QueryRowContext(ctx, `SELECT path, COALESCE(metadata_json, '') FROM snapshots WHERE snapshot_id = ?`, snapshotID).Scan(&path, &metadataJSON)
		if err == nil {
			if metadataJSON != "" {
				var snapshot Snapshot
				if err := json.Unmarshal([]byte(metadataJSON), &snapshot); err == nil {
					return snapshotWithDefaults(snapshot, snapshotID, path), nil
				}
			}
			if path != "" {
				snapshot, err := readSnapshotMetadata(path)
				if err == nil {
					return snapshotWithDefaults(snapshot, snapshotID, path), nil
				}
				return Snapshot{}, fmt.Errorf("load snapshot %s metadata: %w", snapshotID, err)
			}
			return Snapshot{}, fmt.Errorf("load snapshot %s: database metadata is empty", snapshotID)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, fmt.Errorf("load snapshot record %s: %w", snapshotID, err)
		}
	}
	if snapshotRoot != "" {
		directPath := filepath.Join(snapshotRoot, snapshotID)
		if snapshot, err := readSnapshotMetadata(directPath); err == nil {
			return snapshotWithDefaults(snapshot, snapshotID, directPath), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return Snapshot{}, fmt.Errorf("load snapshot %s metadata: %w", snapshotID, err)
		}
		summaries, err := listFilesystemSnapshotSummaries(snapshotRoot)
		if err != nil {
			return Snapshot{}, err
		}
		for _, summary := range summaries {
			if summary.SnapshotID != snapshotID || summary.MetadataError != "" {
				continue
			}
			snapshot, err := readSnapshotMetadata(summary.Path)
			if err != nil {
				return Snapshot{}, fmt.Errorf("load snapshot %s metadata: %w", snapshotID, err)
			}
			return snapshotWithDefaults(snapshot, snapshotID, summary.Path), nil
		}
	}
	return Snapshot{}, fmt.Errorf("snapshot %s not found", snapshotID)
}

func readSnapshotMetadata(snapshotPath string) (Snapshot, error) {
	if snapshotPath == "" {
		return Snapshot{}, fmt.Errorf("snapshot path is required")
	}
	content, err := os.ReadFile(filepath.Join(snapshotPath, SnapshotMetadataFile))
	if err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal(content, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("parse %s: %w", filepath.Join(snapshotPath, SnapshotMetadataFile), err)
	}
	return snapshotWithDefaults(snapshot, filepath.Base(snapshotPath), snapshotPath), nil
}

func listFilesystemSnapshotSummaries(snapshotRoot string) ([]SnapshotSummary, error) {
	if snapshotRoot == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(snapshotRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read snapshot root %s: %w", snapshotRoot, err)
	}
	summaries := []SnapshotSummary{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(snapshotRoot, entry.Name())
		summary := SnapshotSummary{SnapshotID: entry.Name(), Path: path, PreviousActiveBuckets: []string{}, Exists: true}
		snapshot, err := readSnapshotMetadata(path)
		if err != nil {
			summary.MetadataError = err.Error()
		} else {
			applySnapshotToSummary(&summary, snapshot)
		}
		summary.Exists = snapshotDirExists(path)
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func applySnapshotToSummary(summary *SnapshotSummary, snapshot Snapshot) {
	if snapshot.SnapshotID != "" {
		summary.SnapshotID = snapshot.SnapshotID
	}
	if snapshot.MachineID != "" {
		summary.MachineID = snapshot.MachineID
	}
	if snapshot.Path != "" {
		summary.Path = snapshot.Path
	}
	if snapshot.CreatedAt != "" {
		summary.CreatedAt = snapshot.CreatedAt
	}
	if snapshot.PreviousActiveProfile != "" {
		summary.PreviousActiveProfile = snapshot.PreviousActiveProfile
	}
	summary.PreviousActiveBuckets = cloneStrings(snapshot.PreviousActiveBuckets)
	summary.TargetCount = len(snapshot.Targets)
	summary.TargetKinds = snapshotTargetKinds(snapshot.Targets)
}

func snapshotWithDefaults(snapshot Snapshot, snapshotID, snapshotPath string) Snapshot {
	if snapshot.SnapshotID == "" {
		snapshot.SnapshotID = snapshotID
	}
	if snapshot.Path == "" {
		snapshot.Path = snapshotPath
	}
	if snapshot.PreviousActiveBuckets == nil {
		snapshot.PreviousActiveBuckets = []string{}
	}
	if snapshot.Targets == nil {
		snapshot.Targets = []SnapshotEntry{}
	}
	if snapshot.ManagedTargets == nil {
		snapshot.ManagedTargets = []ManagedTargetSnapshot{}
	}
	return snapshot
}

func snapshotTargetKinds(targets []SnapshotEntry) []string {
	seen := map[string]bool{}
	for _, target := range targets {
		if target.Kind == "" || seen[target.Kind] {
			continue
		}
		seen[target.Kind] = true
	}
	kinds := make([]string, 0, len(seen))
	for kind := range seen {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

func snapshotDirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func validateSnapshotID(snapshotID string) error {
	if snapshotID == "" {
		return fmt.Errorf("snapshot id is required")
	}
	if snapshotID == "." || snapshotID == ".." || strings.ContainsAny(snapshotID, `/\\`) {
		return fmt.Errorf("invalid snapshot id %q", snapshotID)
	}
	return nil
}

func appendMetadataError(current, next string) string {
	if current == "" {
		return next
	}
	if next == "" {
		return current
	}
	return current + "; " + next
}

func EnforceSnapshotRetention(ctx context.Context, database *sql.DB, snapshotRoot string, keep int) error {
	if keep <= 0 {
		return nil
	}
	entries, err := os.ReadDir(snapshotRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read snapshot root %s: %w", snapshotRoot, err)
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	if len(dirs) <= keep {
		return nil
	}
	for _, name := range dirs[keep:] {
		path := filepath.Join(snapshotRoot, name)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove old snapshot %s: %w", path, err)
		}
		if database != nil {
			_, _ = database.ExecContext(ctx, `DELETE FROM snapshots WHERE snapshot_id = ? OR path = ?`, name, path)
		}
	}
	return nil
}
