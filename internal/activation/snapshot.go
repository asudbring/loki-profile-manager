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
	"time"

	"github.com/google/uuid"
)

const SnapshotMetadataFile = "metadata.json"

type Snapshot struct {
	SnapshotID            string          `json:"snapshot_id"`
	MachineID             string          `json:"machine_id,omitempty"`
	Path                  string          `json:"path"`
	CreatedAt             string          `json:"created_at"`
	PreviousActiveProfile string          `json:"previous_active_profile,omitempty"`
	PreviousActiveBuckets []string        `json:"previous_active_buckets"`
	Targets               []SnapshotEntry `json:"targets"`
}

type SnapshotEntry struct {
	TargetPath   string `json:"path"`
	Kind         string `json:"kind"`
	Hash         string `json:"hash,omitempty"`
	SnapshotPath string `json:"snapshot_path,omitempty"`
	LinkTarget   string `json:"link_target,omitempty"`
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
		entry, err := snapshotTarget(op.TargetPath, entriesDir, len(snapshot.Targets))
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.Targets = append(snapshot.Targets, entry)
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

func snapshotTarget(target, entriesDir string, index int) (SnapshotEntry, error) {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return SnapshotEntry{TargetPath: target, Kind: "missing"}, nil
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
	content, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal snapshot database metadata: %w", err)
	}
	buckets, err := json.Marshal(snapshot.PreviousActiveBuckets)
	if err != nil {
		return fmt.Errorf("marshal snapshot buckets: %w", err)
	}
	_, err = database.ExecContext(ctx, `INSERT INTO snapshots (snapshot_id, machine_id, path, previous_active_profile, previous_active_buckets_json, created_at, metadata_json) VALUES (?, ?, ?, ?, ?, ?, ?)`, snapshot.SnapshotID, snapshot.MachineID, snapshot.Path, snapshot.PreviousActiveProfile, string(buckets), snapshot.CreatedAt, string(content))
	if err != nil {
		return fmt.Errorf("insert snapshot record %s: %w", snapshot.SnapshotID, err)
	}
	return nil
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
