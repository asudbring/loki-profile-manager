package migration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/allensu/loki-profile-manager/internal/activation"
	"github.com/allensu/loki-profile-manager/internal/config"
)

type ExecuteRequest struct {
	Database *sql.DB
	Resolver config.PathResolver
	Plan     Plan
	DryRun   bool
	Yes      bool
	Now      func() time.Time
}

type ExecuteResult struct {
	Plan    Plan `json:"plan"`
	DryRun  bool `json:"dry_run"`
	Changed int  `json:"changed"`
}

func Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	result := ExecuteResult{Plan: req.Plan, DryRun: req.DryRun}
	if req.DryRun {
		return result, nil
	}
	if !req.Yes {
		return result, fmt.Errorf("migration requires --yes or --dry-run")
	}
	if req.Database == nil {
		return result, fmt.Errorf("migration execute: database is nil")
	}
	layer := layerInfo{StorePath: req.Plan.StorePath, Profile: req.Plan.Profile, Bucket: req.Plan.Bucket, Root: req.Plan.LayerRoot, ManifestPath: manifestPath(req.Plan), Kind: req.Plan.LayerKind, Name: req.Plan.LayerName}
	plannedManifest, err := prepareManifestItems(layer, req.Plan.Items)
	if err != nil {
		return result, err
	}
	if err := ensureLayerDirs(layer); err != nil {
		return result, err
	}
	for _, item := range req.Plan.Items {
		if item.Collision == CollisionConflict {
			return result, fmt.Errorf("migration item %s has conflicting destination %s", item.ID, item.StorePath)
		}
		if err := validateSkillItem(item); err != nil {
			return result, err
		}
		if err := copyIntoStore(item); err != nil {
			return result, err
		}
	}
	if err := writeManifest(layer.ManifestPath, plannedManifest); err != nil {
		return result, err
	}
	now := req.Now
	if now == nil {
		now = time.Now
	}
	for _, item := range req.Plan.Items {
		if !item.WillAdoptRecord {
			continue
		}
		if err := putManagedRecord(ctx, req.Database, item, now()); err != nil {
			return result, err
		}
	}
	result.Changed = len(req.Plan.Items)
	return result, nil
}

func manifestPath(plan Plan) string {
	for _, item := range plan.Items {
		if item.ManifestPath != "" {
			return item.ManifestPath
		}
	}
	return plan.LayerRoot + string(os.PathSeparator) + "manifest.yaml"
}

func copyIntoStore(item Item) error {
	if item.StorePath == "" || item.SourcePath == "" {
		return fmt.Errorf("migration item %s missing source or destination", item.ID)
	}
	if item.Collision == CollisionIdentical {
		return nil
	}
	return activation.CopyPath(item.SourcePath, item.StorePath)
}

func putManagedRecord(ctx context.Context, database *sql.DB, item Item, now time.Time) error {
	hash, err := activation.HashPath(item.TargetPath)
	if err != nil {
		return fmt.Errorf("hash adopted target %s: %w", item.TargetPath, err)
	}
	storeHash, err := activation.HashPath(item.StorePath)
	if err != nil {
		return fmt.Errorf("hash migrated store copy %s: %w", item.StorePath, err)
	}
	if item.AdoptedTargetHash != "" {
		if hash != item.AdoptedTargetHash {
			return fmt.Errorf("adopted target %s changed before managed-state write; rerun migration/adoption", item.TargetPath)
		}
	} else if hash != storeHash {
		return fmt.Errorf("adopted target %s changed before managed-state write; rerun migration/adoption", item.TargetPath)
	}
	metadata := map[string]any{
		"source_kind":     item.SourceKind,
		"manifest_path":   item.ManifestPath,
		"manifest_source": item.ManifestSource,
		"is_skill":        item.IsSkill,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return activation.PutManagedTarget(ctx, database, activation.ManagedTarget{
		TargetPath:    item.TargetPath,
		SourcePath:    item.StorePath,
		Mode:          item.Mode,
		ContentHash:   hash,
		LayerKind:     item.LayerKind,
		LayerName:     item.LayerName,
		LastAppliedAt: now.UTC().Format(time.RFC3339),
		MetadataJSON:  string(metadataJSON),
	})
}
