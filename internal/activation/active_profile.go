package activation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/asudbring/loki-profile-manager/internal/config"
)

func ActiveProfileValue(profile string, buckets []string) string {
	profile = strings.TrimSpace(profile)
	cleaned := make([]string, 0, len(buckets))
	seen := map[string]bool{}
	for _, bucket := range buckets {
		bucket = strings.TrimSpace(bucket)
		if bucket == "" || seen[bucket] {
			continue
		}
		seen[bucket] = true
		cleaned = append(cleaned, bucket)
	}
	return profile + ":" + strings.Join(cleaned, ",")
}

func WriteActiveProfileMarker(paths config.LocalPaths, profile string, buckets []string) error {
	markerPath := strings.TrimSpace(paths.ActiveProfilePath)
	if markerPath == "" {
		markerPath = activeProfilePathFromStateDir(paths.StateDir)
	}
	if markerPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o700); err != nil {
		return fmt.Errorf("create active profile marker parent %s: %w", filepath.Dir(markerPath), err)
	}
	content := ActiveProfileValue(profile, buckets) + "\n"
	if err := os.WriteFile(markerPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write active profile marker %s: %w", markerPath, err)
	}
	return nil
}

func activeProfilePathFromStateDir(stateDir string) string {
	stateDir = strings.TrimSpace(stateDir)
	if stateDir == "" {
		return ""
	}
	if strings.Contains(stateDir, `\`) {
		return strings.TrimRight(stateDir, `\`) + `\active_profile.txt`
	}
	return filepath.Join(stateDir, "active_profile.txt")
}
