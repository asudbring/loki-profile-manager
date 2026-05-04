package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/allensu/loki-profile-manager/internal/machine"
	"github.com/allensu/loki-profile-manager/internal/store"
	"github.com/allensu/loki-profile-manager/internal/storesync"
)

type SyncRequest struct {
	StorePath string
	DryRun    bool
	Yes       bool
}

type SyncResult struct {
	StorePath        string                   `json:"store_path"`
	DryRun           bool                     `json:"dry_run"`
	WouldDeleteCount int                      `json:"would_delete_count"`
	DeletedCount     int                      `json:"deleted_count"`
	SkippedCount     int                      `json:"skipped_count"`
	Conflicts        []storesync.ConflictCopy `json:"conflicts"`
	Truncated        bool                     `json:"truncated,omitempty"`
	HeartbeatUpdated bool                     `json:"heartbeat_updated"`
	MachineID        string                   `json:"machine_id,omitempty"`
	ActiveProfile    string                   `json:"active_profile,omitempty"`
	ActiveBuckets    []string                 `json:"active_buckets,omitempty"`
	Warnings         []string                 `json:"warnings,omitempty"`
}

func (s *Service) Sync(ctx context.Context, req SyncRequest) (SyncResult, error) {
	if s == nil {
		return SyncResult{}, fmt.Errorf("sync: service is nil")
	}
	if req.DryRun == req.Yes {
		return SyncResult{}, fmt.Errorf("sync: run exactly one of --dry-run or --yes")
	}
	storePath, err := s.effectiveStorePath(ctx, req.StorePath)
	if err != nil {
		return SyncResult{}, err
	}
	if validation := store.ValidateLayout(storePath); !validation.Valid {
		return SyncResult{}, fmt.Errorf("sync: invalid store layout: missing %v", validation.Missing)
	}

	result := SyncResult{StorePath: storePath, DryRun: req.DryRun, ActiveBuckets: []string{}}
	err = s.withStoreOperationLock(ctx, storePath, "sync", req.Yes, func(machineID string) error {
		result.MachineID = machineID
		scan, err := storesync.ScanConflicts(storesync.ConflictScanOptions{Root: storePath, Limit: storesync.DefaultConflictScanLimit})
		if err != nil {
			return err
		}
		result.Conflicts = scan.Conflicts
		result.Truncated = scan.Truncated
		result.WouldDeleteCount, result.SkippedCount = countSyncActions(scan.Conflicts)
		if req.DryRun {
			return nil
		}

		record, registered, err := machine.GetMachine(storePath, machineID)
		if err != nil {
			return err
		}
		if !registered {
			return fmt.Errorf("sync: machine %s is not registered; run `loki machine register --allow-profile <profile>` before deleting conflict copies", machineID)
		}
		profile, buckets := s.previousActiveState(ctx, record, registered)
		result.ActiveProfile = profile
		result.ActiveBuckets = cloneStrings(buckets)

		for _, conflict := range scan.Conflicts {
			if conflict.Action != storesync.ConflictActionDelete {
				continue
			}
			if err := deleteConflictCopy(storePath, conflict); err != nil {
				return err
			}
			result.DeletedCount++
		}
		if _, err := s.WriteHeartbeat(ctx, HeartbeatRequest{StorePath: storePath, ActiveProfile: profile, ActiveBuckets: buckets}); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("heartbeat update failed: %v", err))
		} else {
			result.HeartbeatUpdated = true
		}
		return nil
	})
	return result, err
}

func countSyncActions(conflicts []storesync.ConflictCopy) (deleteCount, skipCount int) {
	for _, conflict := range conflicts {
		switch conflict.Action {
		case storesync.ConflictActionDelete:
			deleteCount++
		default:
			skipCount++
		}
	}
	return deleteCount, skipCount
}

func deleteConflictCopy(storePath string, conflict storesync.ConflictCopy) error {
	path := filepath.Clean(conflict.Path)
	storeRoot := filepath.Clean(storePath)
	if rel, err := filepath.Rel(storeRoot, path); err != nil || rel == "." || rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator) {
		return fmt.Errorf("sync: refusing to delete conflict outside store: %s", conflict.Path)
	}
	if !storesync.IsDeletableConflictCopyName(filepath.Base(path), storesync.ConflictScanOptions{}) {
		return fmt.Errorf("sync: refusing to delete non-deletable conflict path: %s", conflict.Path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("sync: stat conflict copy %s: %w", conflict.Path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("sync: refusing to delete conflict directory %s", conflict.Path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("sync: delete conflict copy %s: %w", conflict.Path, err)
	}
	return nil
}
