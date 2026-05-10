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
)

type CleanupStatus string

const (
	CleanupMissing   CleanupStatus = "missing"
	CleanupRemovable CleanupStatus = "removable"
	CleanupCovered   CleanupStatus = "covered_by_current_target"
	CleanupBlocked   CleanupStatus = "blocked"
)

type CleanupChange struct {
	TargetPath   string        `json:"target_path"`
	Mode         string        `json:"mode"`
	LayerKind    string        `json:"layer_kind,omitempty"`
	LayerName    string        `json:"layer_name,omitempty"`
	PreviousHash string        `json:"previous_hash,omitempty"`
	CurrentHash  string        `json:"current_hash,omitempty"`
	Status       CleanupStatus `json:"status"`
	Message      string        `json:"message,omitempty"`
}

type CleanupPlan struct {
	Changes []CleanupChange `json:"changes"`
}

func (p CleanupPlan) HasChanges() bool {
	return len(p.Changes) > 0
}

func (p CleanupPlan) HasBlocking() bool {
	for _, change := range p.Changes {
		if change.Status == CleanupBlocked {
			return true
		}
	}
	return false
}

func (p CleanupPlan) BlockingMessages() []string {
	var messages []string
	for _, change := range p.Changes {
		if change.Status != CleanupBlocked {
			continue
		}
		message := change.Message
		if message == "" {
			message = string(change.Status)
		}
		messages = append(messages, fmt.Sprintf("%s: %s", change.TargetPath, message))
	}
	return messages
}

func BuildCleanupPlanForPlan(ctx context.Context, database *sql.DB, plan Plan) (CleanupPlan, error) {
	keepTargets := map[string]bool{}
	for _, op := range plan.Operations {
		if strings.TrimSpace(op.TargetPath) != "" {
			keepTargets[filepath.Clean(op.TargetPath)] = true
		}
	}
	return BuildCleanupPlanForTargets(ctx, database, keepTargets, plan.StorePath)
}

func BuildCleanupPlanForTargets(ctx context.Context, database *sql.DB, keepTargets map[string]bool, storeRoot string) (CleanupPlan, error) {
	records, err := ListManagedTargets(ctx, database)
	if err != nil {
		return CleanupPlan{}, err
	}
	changes := make([]CleanupChange, 0)
	for _, record := range records {
		if !recordBelongsToStore(record, storeRoot) {
			continue
		}
		change, changed, err := classifyCleanupChange(record, keepTargets)
		if err != nil {
			return CleanupPlan{}, err
		}
		if changed {
			changes = append(changes, change)
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].TargetPath < changes[j].TargetPath })
	return CleanupPlan{Changes: changes}, nil
}

func classifyCleanupChange(record ManagedTarget, keepTargets map[string]bool) (CleanupChange, bool, error) {
	if strings.TrimSpace(record.TargetPath) == "" {
		return CleanupChange{}, false, nil
	}
	target := filepath.Clean(record.TargetPath)
	if keepTargets[target] {
		return CleanupChange{}, false, nil
	}
	change := CleanupChange{TargetPath: record.TargetPath, Mode: record.Mode, LayerKind: record.LayerKind, LayerName: record.LayerName, PreviousHash: record.ContentHash}
	if coveredByCurrentTarget(target, keepTargets) {
		change.Status = CleanupCovered
		change.Message = "obsolete managed target is covered by a current directory target; dropping stale state record"
		return change, true, nil
	}
	if protectsCurrentTarget(target, keepTargets) {
		change.Status = CleanupBlocked
		change.Message = "obsolete managed target is an ancestor of a current target; remove or migrate it manually"
		return change, true, nil
	}
	info, err := os.Lstat(record.TargetPath)
	if errors.Is(err, os.ErrNotExist) {
		change.Status = CleanupMissing
		change.Message = "obsolete managed target is already missing; dropping stale state record"
		return change, true, nil
	}
	if err != nil {
		return CleanupChange{}, false, fmt.Errorf("classify cleanup target %s: %w", record.TargetPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 && record.Mode == string(OperationSymlink) && record.SourcePath != "" {
		linkTarget, err := os.Readlink(record.TargetPath)
		if err != nil {
			return CleanupChange{}, false, fmt.Errorf("read cleanup symlink %s: %w", record.TargetPath, err)
		}
		if samePath(linkTarget, record.SourcePath) {
			change.Status = CleanupRemovable
			change.Message = "obsolete managed symlink still points at Loki source"
			return change, true, nil
		}
		change.Status = CleanupBlocked
		change.Message = "obsolete managed symlink points somewhere else; remove or adopt it manually"
		return change, true, nil
	}
	if record.ContentHash == "" {
		change.Status = CleanupBlocked
		change.Message = "obsolete managed target has no recorded hash; remove or adopt it manually"
		return change, true, nil
	}
	hash, err := HashPath(record.TargetPath)
	if err != nil {
		return CleanupChange{}, false, err
	}
	change.CurrentHash = hash
	if hash != record.ContentHash {
		change.Status = CleanupBlocked
		change.Message = "obsolete managed target changed since Loki last applied it; capture, remove, or adopt it manually"
		return change, true, nil
	}
	change.Status = CleanupRemovable
	change.Message = "obsolete managed target still matches Loki state"
	return change, true, nil
}

func ApplyCleanup(ctx context.Context, database *sql.DB, plan CleanupPlan, keepTargets map[string]bool) (int, error) {
	cleaned := 0
	for _, planned := range plan.Changes {
		record, found, err := GetManagedTarget(ctx, database, planned.TargetPath)
		if err != nil {
			return cleaned, err
		}
		if !found {
			continue
		}
		latest, changed, err := classifyCleanupChange(record, keepTargets)
		if err != nil {
			return cleaned, err
		}
		if !changed {
			continue
		}
		switch latest.Status {
		case CleanupMissing, CleanupCovered:
			if err := DeleteManagedTarget(ctx, database, latest.TargetPath); err != nil {
				return cleaned, err
			}
			cleaned++
		case CleanupRemovable:
			if err := os.RemoveAll(latest.TargetPath); err != nil {
				return cleaned, fmt.Errorf("remove obsolete managed target %s: %w", latest.TargetPath, err)
			}
			if err := DeleteManagedTarget(ctx, database, latest.TargetPath); err != nil {
				return cleaned, err
			}
			cleaned++
		case CleanupBlocked:
			return cleaned, fmt.Errorf("cleanup %s: %s", latest.TargetPath, latest.Message)
		default:
			return cleaned, fmt.Errorf("cleanup %s: unsupported cleanup status %q", latest.TargetPath, latest.Status)
		}
	}
	return cleaned, nil
}

func CleanupKeepTargets(plan Plan) map[string]bool {
	keepTargets := map[string]bool{}
	for _, op := range plan.Operations {
		if strings.TrimSpace(op.TargetPath) != "" {
			keepTargets[filepath.Clean(op.TargetPath)] = true
		}
	}
	return keepTargets
}

func coveredByCurrentTarget(target string, keepTargets map[string]bool) bool {
	for keep := range keepTargets {
		if pathWithin(keep, target) {
			return true
		}
	}
	return false
}

func protectsCurrentTarget(target string, keepTargets map[string]bool) bool {
	for keep := range keepTargets {
		if pathWithin(target, keep) {
			return true
		}
	}
	return false
}

func recordBelongsToStore(record ManagedTarget, storeRoot string) bool {
	storeRoot = filepath.Clean(strings.TrimSpace(storeRoot))
	if storeRoot == "." || storeRoot == "" {
		return false
	}
	if pathSameOrWithin(storeRoot, record.SourcePath) {
		return true
	}
	if record.MetadataJSON != "" {
		var metadata struct {
			Sources []Source `json:"sources"`
		}
		if json.Unmarshal([]byte(record.MetadataJSON), &metadata) == nil {
			for _, source := range metadata.Sources {
				if pathSameOrWithin(storeRoot, source.Path) {
					return true
				}
			}
		}
	}
	return false
}

func pathSameOrWithin(parent, child string) bool {
	parent = strings.TrimSpace(parent)
	child = strings.TrimSpace(child)
	if parent == "" || child == "" {
		return false
	}
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if parent == child {
		return true
	}
	return pathWithin(parent, child)
}

func pathWithin(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if parent == child {
		return false
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
