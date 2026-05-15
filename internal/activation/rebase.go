package activation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// RebaseManagedTargetSourcePaths rewrites local managed target store-source paths from oldRoot to newRoot.
func RebaseManagedTargetSourcePaths(ctx context.Context, database *sql.DB, oldRoot, newRoot string) (int, error) {
	if database == nil {
		return 0, fmt.Errorf("rebase managed target source paths: database is nil")
	}
	if err := validateRebaseRoots(oldRoot, newRoot); err != nil {
		return 0, err
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin source path rebase: %w", err)
	}
	defer tx.Rollback()
	changed, err := RebaseManagedTargetSourcePathsTx(ctx, tx, oldRoot, newRoot)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit source path rebase: %w", err)
	}
	return changed, nil
}

// RebaseManagedTargetSourcePathsTx rewrites managed target source paths inside an existing transaction.
func RebaseManagedTargetSourcePathsTx(ctx context.Context, tx *sql.Tx, oldRoot, newRoot string) (int, error) {
	if tx == nil {
		return 0, fmt.Errorf("rebase managed target source paths: transaction is nil")
	}
	if err := validateRebaseRoots(oldRoot, newRoot); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, managedTargetSelectSQL+` ORDER BY target_path`)
	if err != nil {
		return 0, fmt.Errorf("query managed targets for rebase: %w", err)
	}
	defer rows.Close()
	records := []ManagedTarget{}
	for rows.Next() {
		var record ManagedTarget
		if err := rows.Scan(&record.TargetPath, &record.SourcePath, &record.Mode, &record.ContentHash, &record.LayerKind, &record.LayerName, &record.LastAppliedAt, &record.MetadataJSON); err != nil {
			return 0, fmt.Errorf("scan managed target for rebase: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate managed targets for rebase: %w", err)
	}
	changed := 0
	for _, record := range records {
		recordChanged := false
		if next, ok, err := rebasePathUnderRoot(record.SourcePath, oldRoot, newRoot); err != nil {
			return changed, err
		} else if ok {
			record.SourcePath = next
			recordChanged = true
		}
		if nextMetadata, ok, err := rebaseMetadataSources(record.MetadataJSON, oldRoot, newRoot); err != nil {
			return changed, err
		} else if ok {
			record.MetadataJSON = nextMetadata
			recordChanged = true
		}
		if !recordChanged {
			continue
		}
		if err := putManagedTargetTx(ctx, tx, record); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}

func validateRebaseRoots(oldRoot, newRoot string) error {
	oldRoot = strings.TrimSpace(oldRoot)
	newRoot = strings.TrimSpace(newRoot)
	if oldRoot == "" || newRoot == "" {
		return fmt.Errorf("rebase managed target source paths: old and new store roots are required")
	}
	oldAbs, err := filepath.Abs(filepath.Clean(oldRoot))
	if err != nil {
		return fmt.Errorf("resolve old store root %s: %w", oldRoot, err)
	}
	newAbs, err := filepath.Abs(filepath.Clean(newRoot))
	if err != nil {
		return fmt.Errorf("resolve new store root %s: %w", newRoot, err)
	}
	if oldAbs == newAbs {
		return fmt.Errorf("rebase managed target source paths: old and new store roots must differ")
	}
	if pathWithinRoot(oldAbs, newAbs) || pathWithinRoot(newAbs, oldAbs) {
		return fmt.Errorf("rebase managed target source paths: store roots must not be nested")
	}
	return nil
}

func rebaseMetadataSources(metadataJSON, oldRoot, newRoot string) (string, bool, error) {
	if strings.TrimSpace(metadataJSON) == "" {
		return metadataJSON, false, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(metadataJSON), &raw); err != nil {
		return "", false, fmt.Errorf("parse managed target metadata for rebase: %w", err)
	}
	sourcesRaw, ok := raw["sources"]
	if !ok || len(sourcesRaw) == 0 || string(sourcesRaw) == "null" {
		return metadataJSON, false, nil
	}
	var sources []Source
	if err := json.Unmarshal(sourcesRaw, &sources); err != nil {
		return "", false, fmt.Errorf("parse managed target metadata sources for rebase: %w", err)
	}
	changed := false
	for i := range sources {
		if next, ok, err := rebasePathUnderRoot(sources[i].Path, oldRoot, newRoot); err != nil {
			return "", false, err
		} else if ok {
			sources[i].Path = next
			changed = true
		}
	}
	if !changed {
		return metadataJSON, false, nil
	}
	nextSources, err := json.Marshal(sources)
	if err != nil {
		return "", false, fmt.Errorf("marshal rebased metadata sources: %w", err)
	}
	raw["sources"] = nextSources
	next, err := json.Marshal(raw)
	if err != nil {
		return "", false, fmt.Errorf("marshal rebased managed target metadata: %w", err)
	}
	return string(next), true, nil
}

func rebasePathUnderRoot(pathValue, oldRoot, newRoot string) (string, bool, error) {
	pathValue = strings.TrimSpace(pathValue)
	if pathValue == "" {
		return pathValue, false, nil
	}
	oldAbs, err := filepath.Abs(filepath.Clean(oldRoot))
	if err != nil {
		return "", false, err
	}
	pathAbs, err := filepath.Abs(filepath.Clean(pathValue))
	if err != nil {
		return "", false, err
	}
	oldComparable := comparableActivationPath(oldAbs)
	pathComparable := comparableActivationPath(pathAbs)
	if pathComparable != oldComparable && !pathWithinRoot(oldAbs, pathAbs) {
		return pathValue, false, nil
	}
	rel, err := relativePathUnderRootPreserveCase(oldAbs, pathAbs)
	if err != nil {
		return "", false, err
	}
	if rel == "." {
		return filepath.Clean(newRoot), true, nil
	}
	return filepath.Join(filepath.Clean(newRoot), rel), true, nil
}

func pathWithinRoot(rootAbs, childAbs string) bool {
	rel, err := filepath.Rel(comparableActivationPath(rootAbs), comparableActivationPath(childAbs))
	if err != nil || rel == "." || rel == "" {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func comparableActivationPath(path string) string {
	return strings.ToLower(filepath.Clean(path))
}

func relativePathUnderRootPreserveCase(rootAbs, childAbs string) (string, error) {
	rootClean := filepath.Clean(rootAbs)
	childClean := filepath.Clean(childAbs)
	if comparableActivationPath(rootClean) == comparableActivationPath(childClean) {
		return ".", nil
	}
	rootWithSeparator := rootClean
	if !strings.HasSuffix(rootWithSeparator, string(filepath.Separator)) {
		rootWithSeparator += string(filepath.Separator)
	}
	if strings.HasPrefix(comparableActivationPath(childClean), comparableActivationPath(rootWithSeparator)) && len(childClean) >= len(rootWithSeparator) {
		return childClean[len(rootWithSeparator):], nil
	}
	return filepath.Rel(rootClean, childClean)
}
