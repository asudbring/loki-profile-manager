package doctor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/manifest"
	"github.com/asudbring/loki-profile-manager/internal/profile"
)

// ResolvableBlocker describes a switch-blocking capture change that can be
// resolved by promoting local overrides into a store layer.
type ResolvableBlocker struct {
	Change     activation.CaptureChange `json:"change"`
	Format     string                   `json:"format"`
	TargetPath string                   `json:"target_path"`

	// AvailableLayers lists the layer names that currently contribute a source
	// for this merge target. The user picks one to own the overrides.
	AvailableLayers []LayerChoice `json:"available_layers"`
}

// LayerChoice represents a layer the user can select to receive promoted overrides.
type LayerChoice struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	SourcePath string `json:"source_path"`
	RootDir    string `json:"root_dir"`
}

// FindSwitchBlockers detects CaptureUnsupported/CaptureConflict changes that
// block switch operations. It returns blockers that can potentially be resolved
// by writing local content back to a store layer source.
func FindSwitchBlockers(ctx context.Context, database *sql.DB, storePath string, resolver config.PathResolver) ([]ResolvableBlocker, error) {
	if database == nil {
		return nil, nil
	}
	capturePlan, err := activation.BuildCapturePlan(ctx, database)
	if err != nil {
		return nil, err
	}
	if !capturePlan.HasBlocking() {
		return nil, nil
	}

	// Load all layers to find source paths for blocking targets.
	layers, err := profile.LoadAllManifests(storePath)
	if err != nil {
		return nil, err
	}

	// Build a map from target path → list of layer entries.
	type layerSource struct {
		layerName  string
		layerKind  string
		sourcePath string
		rootDir    string
		format     string
		mode       string
	}
	targetSources := map[string][]layerSource{}
	for _, layer := range layers {
		expander := manifest.Expander{Resolver: resolver, Targets: layer.Manifest.Targets}
		result := manifest.ValidateLayer(manifest.ValidationInput{LayerName: layer.Name, LayerRoot: layer.RootDir, Manifest: layer.Manifest, Expander: expander})
		for _, op := range result.Operations {
			targetSources[filepath.Clean(op.TargetPath)] = append(targetSources[filepath.Clean(op.TargetPath)], layerSource{
				layerName:  layer.Name,
				layerKind:  string(layer.Kind),
				sourcePath: op.SourcePath,
				rootDir:    layer.RootDir,
				format:     op.Entry.Format,
				mode:       op.Entry.Mode,
			})
		}
	}

	var blockers []ResolvableBlocker
	for _, change := range capturePlan.Changes {
		if change.Status != activation.CaptureUnsupported && change.Status != activation.CaptureConflict {
			continue
		}
		cleanTarget := filepath.Clean(change.TargetPath)
		sources, ok := targetSources[cleanTarget]
		if !ok || len(sources) == 0 {
			continue
		}

		// Only handle merge-mode targets for resolution (the common case).
		mergeSources := []layerSource{}
		format := ""
		for _, src := range sources {
			if src.mode == manifest.ModeMerge {
				mergeSources = append(mergeSources, src)
				format = src.format
			}
		}
		if len(mergeSources) == 0 {
			continue
		}

		choices := make([]LayerChoice, 0, len(mergeSources))
		for _, src := range mergeSources {
			choices = append(choices, LayerChoice{
				Name:       src.layerName,
				Kind:       src.layerKind,
				SourcePath: src.sourcePath,
				RootDir:    src.rootDir,
			})
		}

		blockers = append(blockers, ResolvableBlocker{
			Change:          change,
			Format:          format,
			TargetPath:      change.TargetPath,
			AvailableLayers: choices,
		})
	}
	return blockers, nil
}

// ResolveBlockerOptions controls how a single blocker is resolved.
type ResolveBlockerOptions struct {
	Blocker      ResolvableBlocker
	ChosenLayer  LayerChoice
	Database     *sql.DB
	Now          time.Time
}

// ResolveBlocker promotes the local target content into the chosen layer's
// source file and repairs the managed state record to clear the blocker.
func ResolveBlocker(ctx context.Context, opts ResolveBlockerOptions) error {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}

	// Read local target content.
	localContent, err := os.ReadFile(opts.Blocker.TargetPath)
	if err != nil {
		return fmt.Errorf("resolve blocker: read local target %s: %w", opts.Blocker.TargetPath, err)
	}

	// Validate local content is valid for the format.
	if opts.Blocker.Format == "json" {
		var probe any
		if err := json.Unmarshal(localContent, &probe); err != nil {
			return fmt.Errorf("resolve blocker: local target %s is not valid JSON: %w", opts.Blocker.TargetPath, err)
		}
		// Re-marshal with indentation for consistent formatting.
		formatted, err := json.MarshalIndent(probe, "", "  ")
		if err != nil {
			return fmt.Errorf("resolve blocker: format JSON: %w", err)
		}
		localContent = append(formatted, '\n')
	}

	// Write local content to the chosen layer's source file.
	sourceDir := filepath.Dir(opts.ChosenLayer.SourcePath)
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		return fmt.Errorf("resolve blocker: create source dir %s: %w", sourceDir, err)
	}
	if err := os.WriteFile(opts.ChosenLayer.SourcePath, localContent, 0o644); err != nil {
		return fmt.Errorf("resolve blocker: write source %s: %w", opts.ChosenLayer.SourcePath, err)
	}

	// Now repair managed state: update the content hash to match the (now-aligned) local file.
	if opts.Database == nil {
		return nil
	}
	hash, err := activation.HashPath(opts.Blocker.TargetPath)
	if err != nil {
		return fmt.Errorf("resolve blocker: hash target: %w", err)
	}

	record, found, err := activation.GetManagedTarget(ctx, opts.Database, opts.Blocker.TargetPath)
	if err != nil {
		return fmt.Errorf("resolve blocker: get managed target: %w", err)
	}
	if !found {
		return nil // No record to repair.
	}
	record.ContentHash = hash
	record.LastAppliedAt = opts.Now.UTC().Format(time.RFC3339)

	// Update metadata to note the resolution.
	metadataMap := map[string]any{}
	if strings.TrimSpace(record.MetadataJSON) != "" {
		_ = json.Unmarshal([]byte(record.MetadataJSON), &metadataMap)
	}
	metadataMap["doctor_resolve"] = true
	metadataMap["resolved_at"] = opts.Now.UTC().Format(time.RFC3339)
	metadataMap["promoted_to_layer"] = opts.ChosenLayer.Name
	metadata, err := json.Marshal(metadataMap)
	if err != nil {
		return fmt.Errorf("resolve blocker: marshal metadata: %w", err)
	}
	record.MetadataJSON = string(metadata)
	return activation.PutManagedTarget(ctx, opts.Database, record)
}
