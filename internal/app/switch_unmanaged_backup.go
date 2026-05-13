package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/asudbring/loki-profile-manager/internal/activation"
)

type unmanagedBackupManifest struct {
	CreatedAt string            `json:"created_at"`
	Profile   string            `json:"profile"`
	Buckets   []string          `json:"buckets"`
	Backups   []UnmanagedBackup `json:"backups"`
}

func (s *Service) backupUnmanagedTargetsIfRequested(ctx context.Context, plan activation.Plan, req SwitchRequest) (activation.Plan, string, []UnmanagedBackup, error) {
	err := activation.ValidateSafety(ctx, s.database, &plan)
	if err == nil {
		return plan, "", nil, nil
	}
	if !req.BackupUnmanaged || req.DryRun {
		return plan, "", nil, err
	}
	backupOps, blocked := unmanagedBackupCandidates(plan)
	if len(blocked) > 0 || len(backupOps) == 0 {
		return plan, "", nil, err
	}
	backupRoot, backups, backupErr := s.backupUnmanagedTargets(plan, backupOps)
	if backupErr != nil {
		return plan, backupRoot, backups, backupErr
	}
	if err := activation.ValidateSafety(ctx, s.database, &plan); err != nil {
		return plan, backupRoot, backups, err
	}
	return plan, backupRoot, backups, nil
}

func unmanagedBackupCandidates(plan activation.Plan) ([]activation.Operation, []activation.Operation) {
	seen := map[string]bool{}
	var backupOps []activation.Operation
	var blocked []activation.Operation
	for _, op := range plan.Operations {
		if op.Safety.Safe {
			continue
		}
		switch op.Safety.Class {
		case activation.SafetyUnmanagedFile, activation.SafetyUnmanagedDirectory:
			if !seen[op.TargetPath] {
				seen[op.TargetPath] = true
				backupOps = append(backupOps, op)
			}
		default:
			blocked = append(blocked, op)
		}
	}
	return backupOps, blocked
}

func (s *Service) backupUnmanagedTargets(plan activation.Plan, ops []activation.Operation) (string, []UnmanagedBackup, error) {
	if len(ops) == 0 {
		return "", nil, nil
	}
	if strings.TrimSpace(s.paths.StateDir) == "" {
		return "", nil, fmt.Errorf("backup unmanaged targets: local state directory is required")
	}
	now := time.Now().UTC()
	backupBase := filepath.Join(s.paths.StateDir, "unmanaged-backups")
	for _, op := range ops {
		if pathContainsOrEqual(op.TargetPath, s.paths.StateDir, s.resolver.GOOS) {
			return "", nil, fmt.Errorf("backup unmanaged target %s: target contains Loki local state directory %s; move or adopt it manually", op.TargetPath, s.paths.StateDir)
		}
		if pathContainsOrEqual(s.paths.StateDir, op.TargetPath, s.resolver.GOOS) {
			return "", nil, fmt.Errorf("backup unmanaged target %s: target is inside Loki local state directory %s; move or adopt it manually", op.TargetPath, s.paths.StateDir)
		}
	}
	if err := os.MkdirAll(backupBase, 0o700); err != nil {
		return backupBase, nil, fmt.Errorf("create unmanaged backup base %s: %w", backupBase, err)
	}
	backupRoot, err := os.MkdirTemp(backupBase, now.Format("20060102T150405Z")+"-")
	if err != nil {
		return backupBase, nil, fmt.Errorf("create unmanaged backup root under %s: %w", backupBase, err)
	}
	backups := make([]UnmanagedBackup, 0, len(ops))
	for i, op := range ops {
		backupPath := filepath.Join(backupRoot, backupNameForTarget(i+1, op.TargetPath))
		if err := activation.CopyPath(op.TargetPath, backupPath); err != nil {
			return backupRoot, backups, fmt.Errorf("backup unmanaged target %s: %w", op.TargetPath, err)
		}
		if err := os.RemoveAll(op.TargetPath); err != nil {
			return backupRoot, backups, fmt.Errorf("remove unmanaged target after backup %s: %w; backup preserved at %s", op.TargetPath, err, backupPath)
		}
		backups = append(backups, UnmanagedBackup{TargetPath: op.TargetPath, BackupPath: backupPath, SafetyClass: op.Safety.Class})
	}
	manifest := unmanagedBackupManifest{CreatedAt: now.Format(time.RFC3339), Profile: plan.Profile, Buckets: cloneStrings(plan.Buckets), Backups: backups}
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return backupRoot, backups, fmt.Errorf("marshal unmanaged backup manifest: %w", err)
	}
	manifestPath := filepath.Join(backupRoot, "manifest.json")
	if err := os.WriteFile(manifestPath, append(content, '\n'), 0o600); err != nil {
		return backupRoot, backups, fmt.Errorf("write unmanaged backup manifest %s: %w", manifestPath, err)
	}
	return backupRoot, backups, nil
}

func pathContainsOrEqual(parent, child, goos string) bool {
	parent = strings.TrimSpace(parent)
	child = strings.TrimSpace(child)
	if parent == "" || child == "" {
		return false
	}
	parentAbs, err := filepath.Abs(parent)
	if err != nil {
		parentAbs = filepath.Clean(parent)
	}
	childAbs, err := filepath.Abs(child)
	if err != nil {
		childAbs = filepath.Clean(child)
	}
	parentAbs = normalizeContainmentPath(filepath.Clean(parentAbs), goos)
	childAbs = normalizeContainmentPath(filepath.Clean(childAbs), goos)
	if parentAbs == childAbs {
		return true
	}
	rel, err := filepath.Rel(parentAbs, childAbs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func normalizeContainmentPath(path, goos string) string {
	if goos == "windows" || goos == "darwin" {
		return strings.ToLower(path)
	}
	return path
}

func backupNameForTarget(index int, target string) string {
	sum := sha256.Sum256([]byte(target))
	name := sanitizeBackupName(target)
	if len(name) > 80 {
		name = name[len(name)-80:]
	}
	return fmt.Sprintf("%03d-%s-%s", index, name, hex.EncodeToString(sum[:6]))
}

func sanitizeBackupName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "target"
	}
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range value {
		keep := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_'
		if keep {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(builder.String(), "_")
	if out == "" {
		return "target"
	}
	return out
}
