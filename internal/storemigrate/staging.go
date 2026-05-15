package storemigrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/store"
)

const stagingMarkerName = ".loki-store-migrate.json"

type stagingManifest struct {
	Tool      string `json:"tool"`
	FinalPath string `json:"final_path"`
}

// Staging describes a temporary sibling store used before final promotion.
type Staging struct {
	FinalPath  string `json:"final_path"`
	Path       string `json:"path"`
	MarkerPath string `json:"marker_path"`
}

// NewStaging returns a hidden sibling staging directory for a final destination.
func NewStaging(finalPath string, now time.Time) Staging {
	finalPath = filepath.Clean(finalPath)
	stamp := now.UTC().Format("20060102T150405Z")
	name := fmt.Sprintf(".%s.incomplete-%s-%d", filepath.Base(finalPath), stamp, now.UTC().UnixNano())
	path := filepath.Join(filepath.Dir(finalPath), name)
	return Staging{FinalPath: finalPath, Path: path, MarkerPath: filepath.Join(path, stagingMarkerName)}
}

// PrepareStaging creates the staging directory and writes the marker used for safe cleanup.
func PrepareStaging(staging Staging) error {
	if strings.TrimSpace(staging.Path) == "" || strings.TrimSpace(staging.FinalPath) == "" {
		return fmt.Errorf("store migrate: staging and final paths are required")
	}
	if err := os.MkdirAll(filepath.Dir(staging.Path), 0o755); err != nil {
		return fmt.Errorf("store migrate: create staging parent %s: %w", filepath.Dir(staging.Path), err)
	}
	if err := os.Mkdir(staging.Path, 0o755); err != nil {
		return fmt.Errorf("store migrate: create staging %s: %w", staging.Path, err)
	}
	return writeStagingMarker(staging)
}

// PromoteStaging validates staging and renames it into the final destination path.
func PromoteStaging(staging Staging) error {
	if strings.TrimSpace(staging.Path) == "" || strings.TrimSpace(staging.FinalPath) == "" {
		return fmt.Errorf("store migrate: staging and final paths are required")
	}
	if ok := stagingMarkerMatches(staging.Path, staging.FinalPath); !ok {
		return fmt.Errorf("store migrate: staging marker missing or does not match destination: %s", staging.Path)
	}
	if err := os.Remove(staging.MarkerPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("store migrate: remove staging marker %s: %w", staging.MarkerPath, err)
	}
	validation := store.ValidateLayout(staging.Path)
	if !validation.Valid {
		return fmt.Errorf("store migrate: staged destination layout is invalid: missing %v", validation.Missing)
	}
	inspection, err := store.InspectLayout(staging.FinalPath)
	if err != nil && inspection.Exists {
		return fmt.Errorf("store migrate: inspect final destination before promotion: %w", err)
	}
	if inspection.Exists {
		if !inspection.IsDir {
			return fmt.Errorf("store migrate: destination is not a directory: %s", staging.FinalPath)
		}
		if !inspection.Empty {
			return fmt.Errorf("store migrate: destination must be missing or empty: %s", staging.FinalPath)
		}
		if err := os.Remove(staging.FinalPath); err != nil {
			return fmt.Errorf("store migrate: remove empty destination before promotion %s: %w", staging.FinalPath, err)
		}
	}
	if err := os.Rename(staging.Path, staging.FinalPath); err != nil {
		return fmt.Errorf("store migrate: promote staging %s to %s: %w", staging.Path, staging.FinalPath, err)
	}
	return nil
}

// CleanupStagingPath removes one known staging path created by this process.
func CleanupStagingPath(staging Staging) error {
	if strings.TrimSpace(staging.Path) == "" {
		return nil
	}
	if _, err := os.Lstat(staging.Path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("store migrate: inspect staging %s: %w", staging.Path, err)
	}
	if !stagingMarkerMatches(staging.Path, staging.FinalPath) {
		return fmt.Errorf("store migrate: refusing to remove unmarked staging path %s", staging.Path)
	}
	if err := os.RemoveAll(staging.Path); err != nil {
		return fmt.Errorf("store migrate: remove staging %s: %w", staging.Path, err)
	}
	return nil
}

// CleanupStaging removes interrupted marked staging siblings for a final destination.
func CleanupStaging(finalPath string) ([]string, error) {
	finalPath = filepath.Clean(strings.TrimSpace(finalPath))
	if finalPath == "" || finalPath == "." {
		return nil, fmt.Errorf("store migrate: destination store path is required")
	}
	parent := filepath.Dir(finalPath)
	base := filepath.Base(finalPath)
	patterns := []string{
		filepath.Join(parent, "."+base+".incomplete-*"),
		filepath.Join(parent, base+".incomplete-*"),
		filepath.Join(parent, "."+base+".loki-migrate-staging-*"),
	}
	seen := map[string]bool{}
	removed := []string{}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return removed, fmt.Errorf("store migrate: scan staging directories: %w", err)
		}
		for _, match := range matches {
			if seen[match] {
				continue
			}
			seen[match] = true
			info, err := os.Lstat(match)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return removed, fmt.Errorf("store migrate: inspect staging %s: %w", match, err)
			}
			if !info.IsDir() || !stagingMarkerMatches(match, finalPath) {
				continue
			}
			if err := os.RemoveAll(match); err != nil {
				return removed, fmt.Errorf("store migrate: remove staging %s: %w", match, err)
			}
			removed = append(removed, match)
		}
	}
	sort.Strings(removed)
	return removed, nil
}

func writeStagingMarker(staging Staging) error {
	manifest := stagingManifest{Tool: "loki-store-migrate", FinalPath: filepath.Clean(staging.FinalPath)}
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("store migrate: marshal staging marker: %w", err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(staging.MarkerPath, content, 0o600); err != nil {
		return fmt.Errorf("store migrate: write staging marker %s: %w", staging.MarkerPath, err)
	}
	return nil
}

func stagingMarkerMatches(stagingPath, finalPath string) bool {
	content, err := os.ReadFile(filepath.Join(stagingPath, stagingMarkerName))
	if err != nil {
		return false
	}
	var manifest stagingManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return false
	}
	return manifest.Tool == "loki-store-migrate" && filepath.Clean(manifest.FinalPath) == filepath.Clean(finalPath)
}

func destinationHasOnlyStagingMarker(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 1 {
		return false
	}
	return entries[0].Name() == stagingMarkerName && !entries[0].IsDir()
}
