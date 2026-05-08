package activation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"
)

type CaptureStatus string

const (
	CaptureUnchanged   CaptureStatus = "unchanged"
	CaptureCapturable  CaptureStatus = "capturable"
	CaptureConflict    CaptureStatus = "conflict"
	CaptureUnsupported CaptureStatus = "unsupported"
)

type CaptureChange struct {
	TargetPath   string        `json:"target_path"`
	SourcePath   string        `json:"source_path,omitempty"`
	Mode         string        `json:"mode"`
	LayerKind    string        `json:"layer_kind,omitempty"`
	LayerName    string        `json:"layer_name,omitempty"`
	PreviousHash string        `json:"previous_hash,omitempty"`
	TargetHash   string        `json:"target_hash,omitempty"`
	SourceHash   string        `json:"source_hash,omitempty"`
	Status       CaptureStatus `json:"status"`
	Message      string        `json:"message,omitempty"`
}

type CapturePlan struct {
	Changes []CaptureChange `json:"changes"`
}

func (p CapturePlan) HasChanges() bool {
	return len(p.Changes) > 0
}

func (p CapturePlan) HasCapturable() bool {
	for _, change := range p.Changes {
		if change.Status == CaptureCapturable {
			return true
		}
	}
	return false
}

func (p CapturePlan) HasBlocking() bool {
	for _, change := range p.Changes {
		if change.Status == CaptureConflict || change.Status == CaptureUnsupported {
			return true
		}
	}
	return false
}

func BuildCapturePlan(ctx context.Context, database *sql.DB) (CapturePlan, error) {
	return BuildCapturePlanForTargets(ctx, database, nil)
}

func BuildCapturePlanForTargets(ctx context.Context, database *sql.DB, targetPaths map[string]bool) (CapturePlan, error) {
	records, err := ListManagedTargets(ctx, database)
	if err != nil {
		return CapturePlan{}, err
	}
	changes := make([]CaptureChange, 0)
	for _, record := range records {
		if len(targetPaths) > 0 && !targetPaths[record.TargetPath] {
			continue
		}
		change, changed, err := classifyCaptureChange(record)
		if err != nil {
			return CapturePlan{}, err
		}
		if changed {
			changes = append(changes, change)
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].TargetPath < changes[j].TargetPath })
	return CapturePlan{Changes: changes}, nil
}

func classifyCaptureChange(record ManagedTarget) (CaptureChange, bool, error) {
	if record.TargetPath == "" || record.ContentHash == "" {
		return CaptureChange{}, false, nil
	}
	if record.Mode == string(OperationSymlink) {
		return CaptureChange{}, false, nil
	}
	info, infoErr := os.Lstat(record.TargetPath)
	if errors.Is(infoErr, os.ErrNotExist) {
		return CaptureChange{TargetPath: record.TargetPath, SourcePath: record.SourcePath, Mode: record.Mode, LayerKind: record.LayerKind, LayerName: record.LayerName, PreviousHash: record.ContentHash, Status: CaptureConflict, Message: "managed target is missing; capture delete is not supported"}, true, nil
	}
	if infoErr != nil {
		return CaptureChange{}, false, infoErr
	}
	targetHash, err := HashPath(record.TargetPath)
	if err != nil {
		return CaptureChange{}, false, err
	}
	if targetHash == record.ContentHash {
		return CaptureChange{}, false, nil
	}
	change := CaptureChange{TargetPath: record.TargetPath, SourcePath: record.SourcePath, Mode: record.Mode, LayerKind: record.LayerKind, LayerName: record.LayerName, PreviousHash: record.ContentHash, TargetHash: targetHash}
	if info.Mode()&os.ModeSymlink != 0 {
		change.Status = CaptureUnsupported
		change.Message = "capture for copy-managed symlink targets is not supported"
		return change, true, nil
	}
	if record.Mode != string(OperationCopy) {
		change.Status = CaptureUnsupported
		change.Message = fmt.Sprintf("capture for %s mode is not supported", record.Mode)
		return change, true, nil
	}
	if record.SourcePath == "" {
		change.Status = CaptureConflict
		change.Message = "managed target has no store source path"
		return change, true, nil
	}
	sourceHash, err := HashPath(record.SourcePath)
	if errors.Is(err, os.ErrNotExist) {
		change.Status = CaptureConflict
		change.Message = "store source is missing"
		return change, true, nil
	}
	if err != nil {
		return CaptureChange{}, false, err
	}
	change.SourceHash = sourceHash
	if sourceHash != record.ContentHash {
		change.Status = CaptureConflict
		change.Message = "local target and store source both changed since last apply"
		return change, true, nil
	}
	change.Status = CaptureCapturable
	change.Message = "local target changed; can write back to store source"
	return change, true, nil
}

func ApplyCaptures(ctx context.Context, database *sql.DB, plan CapturePlan, now time.Time) (int, error) {
	changed := 0
	if now.IsZero() {
		now = time.Now()
	}
	for _, change := range plan.Changes {
		if change.Status != CaptureCapturable {
			continue
		}
		record, found, err := GetManagedTarget(ctx, database, change.TargetPath)
		if err != nil {
			return changed, err
		}
		if !found {
			return changed, fmt.Errorf("capture %s: managed target record disappeared", change.TargetPath)
		}
		latest, changedNow, err := classifyCaptureChange(record)
		if err != nil {
			return changed, err
		}
		if !changedNow {
			continue
		}
		if latest.Status != CaptureCapturable {
			return changed, fmt.Errorf("capture %s: %s", latest.TargetPath, latest.Message)
		}
		if err := CopyPath(latest.TargetPath, latest.SourcePath); err != nil {
			return changed, fmt.Errorf("capture %s to %s: %w", latest.TargetPath, latest.SourcePath, err)
		}
		hash, err := HashPath(latest.TargetPath)
		if err != nil {
			return changed, err
		}
		record.ContentHash = hash
		record.LastAppliedAt = now.UTC().Format(time.RFC3339)
		if err := PutManagedTarget(ctx, database, record); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}
