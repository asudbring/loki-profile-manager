package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/allensu/loki-profile-manager/internal/activation"
	"github.com/allensu/loki-profile-manager/internal/manifest"
	"github.com/allensu/loki-profile-manager/internal/skills"
	"github.com/allensu/loki-profile-manager/internal/store"
)

type ImportSkillRequest struct {
	StorePath    string
	SourceFolder string
	Common       bool
	Profile      string
	Bucket       string
	Name         string
	DryRun       bool
	Yes          bool
	Overwrite    bool
}

type ImportSkillResult struct {
	StorePath         string           `json:"store_path"`
	SourcePath        string           `json:"source_path"`
	SourceKind        string           `json:"source_kind,omitempty"`
	Validation        skills.Result    `json:"validation"`
	Name              string           `json:"name"`
	Layer             ImportSkillLayer `json:"layer"`
	DestinationPath   string           `json:"destination_path"`
	ManifestPath      string           `json:"manifest_path"`
	ManifestSource    string           `json:"manifest_source"`
	SourceHash        string           `json:"source_hash,omitempty"`
	ExistingHash      string           `json:"existing_hash,omitempty"`
	DryRun            bool             `json:"dry_run"`
	Overwrite         bool             `json:"overwrite"`
	DestinationExists bool             `json:"destination_exists"`
	WouldCopy         bool             `json:"would_copy"`
	WouldOverwrite    bool             `json:"would_overwrite"`
	ManifestChanged   bool             `json:"manifest_changed"`
	Changed           int              `json:"changed"`
	Warnings          []string         `json:"warnings,omitempty"`
}

type ImportSkillLayer struct {
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Profile      string `json:"profile,omitempty"`
	Bucket       string `json:"bucket,omitempty"`
	RootDir      string `json:"root_dir"`
	ManifestPath string `json:"manifest_path"`
}

type importSkillLayerInfo struct {
	StorePath    string
	Kind         string
	Name         string
	Profile      string
	Bucket       string
	RootDir      string
	ManifestPath string
}

func (s *Service) ImportSkill(ctx context.Context, req ImportSkillRequest) (ImportSkillResult, error) {
	if s == nil {
		return ImportSkillResult{}, fmt.Errorf("import-skill: service is nil")
	}
	if req.DryRun == req.Yes {
		return ImportSkillResult{}, fmt.Errorf("import-skill: run exactly one of --dry-run or --yes")
	}
	storePath, err := s.effectiveStorePath(ctx, req.StorePath)
	if err != nil {
		return ImportSkillResult{}, err
	}
	if validation := store.ValidateLayout(storePath); !validation.Valid {
		return ImportSkillResult{}, fmt.Errorf("import-skill: invalid store layout: missing %v", validation.Missing)
	}
	layer, err := resolveImportSkillLayer(storePath, req)
	if err != nil {
		return ImportSkillResult{}, err
	}
	prepared, err := prepareImportSkillSource(req.SourceFolder)
	if err != nil {
		return ImportSkillResult{}, err
	}
	defer prepared.Cleanup()
	sourcePath := prepared.SkillDir
	result := ImportSkillResult{
		StorePath:      storePath,
		SourcePath:     prepared.OriginalPath,
		SourceKind:     prepared.Kind,
		Layer:          importSkillLayerResult(layer),
		ManifestPath:   layer.ManifestPath,
		DryRun:         req.DryRun,
		Overwrite:      req.Overwrite,
		ManifestSource: path.Join("skills", firstNonEmptyString(strings.TrimSpace(req.Name), prepared.DefaultName)),
	}
	if err := rejectImportSkillSymlinks(sourcePath); err != nil {
		return result, err
	}
	validation := skills.ValidateFolder(sourcePath)
	result.Validation = validation
	if !validation.Valid {
		return result, fmt.Errorf("import-skill: invalid skill folder: %s", formatSkillIssues(validation.Issues))
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = prepared.DefaultName
	}
	if err := validateImportSkillName("skill name", name); err != nil {
		return result, err
	}
	result.Name = name
	result.ManifestSource = path.Join("skills", name)
	result.DestinationPath = filepath.Join(layer.RootDir, "skills", name)

	err = s.withStoreOperationLock(ctx, storePath, "import-skill", req.Yes, func(machineID string) error {
		return s.executeImportSkill(ctx, req, layer, sourcePath, &result)
	})
	return result, err
}

func (s *Service) executeImportSkill(ctx context.Context, req ImportSkillRequest, layer importSkillLayerInfo, sourcePath string, result *ImportSkillResult) error {
	if err := rejectImportSkillSymlinks(sourcePath); err != nil {
		return err
	}
	validation := skills.ValidateFolder(sourcePath)
	result.Validation = validation
	if !validation.Valid {
		return fmt.Errorf("import-skill: invalid skill folder: %s", formatSkillIssues(validation.Issues))
	}
	parsed, manifestChanged, err := prepareImportSkillManifest(layer, result.ManifestSource)
	if err != nil {
		return err
	}
	result.ManifestChanged = manifestChanged

	samePath, overlaps, err := importSkillPathRelationship(sourcePath, result.DestinationPath)
	if err != nil {
		return err
	}
	if overlaps {
		return fmt.Errorf("import-skill: source %s and destination %s overlap; choose a source outside the destination store path", sourcePath, result.DestinationPath)
	}
	if samePath {
		result.DestinationExists = true
		if req.DryRun {
			return nil
		}
		if result.ManifestChanged {
			if err := ensureImportSkillLayerDirs(layer); err != nil {
				return err
			}
			if err := manifest.WriteFile(layer.ManifestPath, parsed); err != nil {
				return err
			}
			result.Changed = 1
		}
		return nil
	}

	sourceHash, err := activation.HashPath(sourcePath)
	if err != nil {
		return err
	}
	result.SourceHash = sourceHash
	if _, err := os.Lstat(result.DestinationPath); err == nil {
		result.DestinationExists = true
		existingHash, hashErr := activation.HashPath(result.DestinationPath)
		if hashErr != nil {
			return hashErr
		}
		result.ExistingHash = existingHash
		result.WouldCopy = existingHash != sourceHash
	} else if errors.Is(err, os.ErrNotExist) {
		result.WouldCopy = true
	} else {
		return fmt.Errorf("import-skill: stat destination %s: %w", result.DestinationPath, err)
	}
	result.WouldOverwrite = result.DestinationExists && result.WouldCopy && req.Overwrite
	if req.DryRun {
		return nil
	}
	if result.DestinationExists && result.WouldCopy && !req.Overwrite {
		return fmt.Errorf("import-skill: destination %s already exists; rerun with --overwrite to replace it", result.DestinationPath)
	}
	if err := ensureImportSkillLayerDirs(layer); err != nil {
		return err
	}
	if result.WouldCopy {
		if err := activation.CopyPath(sourcePath, result.DestinationPath); err != nil {
			return err
		}
	}
	if result.ManifestChanged {
		if err := manifest.WriteFile(layer.ManifestPath, parsed); err != nil {
			return err
		}
	}
	if result.WouldCopy || result.ManifestChanged {
		result.Changed = 1
	}
	return nil
}

func cleanImportSkillSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("import-skill: source folder is required")
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		return "", fmt.Errorf("import-skill: resolve source folder %s: %w", source, err)
	}
	abs = filepath.Clean(abs)
	info, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("import-skill: stat source folder %s: %w", abs, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("import-skill: source folder %s is a symlink; symlinks are not supported in skill imports", abs)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("import-skill: source folder %s is not a directory", abs)
	}
	return abs, nil
}

func resolveImportSkillLayer(storePath string, req ImportSkillRequest) (importSkillLayerInfo, error) {
	profile := strings.TrimSpace(req.Profile)
	bucket := strings.TrimSpace(req.Bucket)
	if req.Common {
		if profile != "" || bucket != "" {
			return importSkillLayerInfo{}, fmt.Errorf("import-skill: --common cannot be combined with --profile or --bucket")
		}
		root := filepath.Join(storePath, "profiles", "common")
		return importSkillLayerInfo{StorePath: storePath, Kind: "common", Name: "common", RootDir: root, ManifestPath: filepath.Join(root, "manifest.yaml")}, nil
	}
	if profile == "" {
		return importSkillLayerInfo{}, fmt.Errorf("import-skill: choose --common or --profile <profile>")
	}
	if err := validateImportSkillName("profile", profile); err != nil {
		return importSkillLayerInfo{}, err
	}
	if bucket == "" {
		root := filepath.Join(storePath, "profiles", profile, "core")
		return importSkillLayerInfo{StorePath: storePath, Kind: "core", Name: profile + "-core", Profile: profile, RootDir: root, ManifestPath: filepath.Join(root, "manifest.yaml")}, nil
	}
	if err := validateImportSkillName("bucket", bucket); err != nil {
		return importSkillLayerInfo{}, err
	}
	coreManifest := filepath.Join(storePath, "profiles", profile, "core", "manifest.yaml")
	if info, err := os.Stat(coreManifest); err != nil || info.IsDir() {
		if err == nil {
			err = fmt.Errorf("manifest is a directory")
		}
		return importSkillLayerInfo{}, fmt.Errorf("import-skill: parent profile %q is not initialized: %w", profile, err)
	}
	root := filepath.Join(storePath, "profiles", profile, "buckets", bucket)
	return importSkillLayerInfo{StorePath: storePath, Kind: "bucket", Name: bucket, Profile: profile, Bucket: bucket, RootDir: root, ManifestPath: filepath.Join(root, "manifest.yaml")}, nil
}

func prepareImportSkillManifest(layer importSkillLayerInfo, source string) (manifest.Manifest, bool, error) {
	parsed, err := manifest.ParseFile(layer.ManifestPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return manifest.Manifest{}, false, err
		}
		parsed = manifest.Manifest{Version: manifest.Version, Name: layer.Name, Files: []manifest.FileEntry{}, Skills: []manifest.SkillEntry{}, Ignore: []string{}, MergeRules: map[string]string{}, Targets: map[string]manifest.TargetValue{}}
	}
	entry := manifest.SkillEntry{Source: source}
	for i, existing := range parsed.Skills {
		if existing.Source != source {
			continue
		}
		if len(entry.Targets) == 0 {
			entry.Targets = existing.Targets
		}
		parsed.Skills[i] = entry
		return parsed, false, nil
	}
	parsed.Skills = append(parsed.Skills, entry)
	return parsed, true, nil
}

func ensureImportSkillLayerDirs(layer importSkillLayerInfo) error {
	for _, dir := range []string{"files", "skills", "templates"} {
		full := filepath.Join(layer.RootDir, dir)
		if err := os.MkdirAll(full, 0o755); err != nil {
			return fmt.Errorf("create layer directory %s: %w", full, err)
		}
	}
	return nil
}

func importSkillPathRelationship(sourcePath, destinationPath string) (same bool, overlaps bool, err error) {
	sourceAbs, err := filepath.Abs(sourcePath)
	if err != nil {
		return false, false, fmt.Errorf("import-skill: resolve source %s: %w", sourcePath, err)
	}
	destAbs, err := filepath.Abs(destinationPath)
	if err != nil {
		return false, false, fmt.Errorf("import-skill: resolve destination %s: %w", destinationPath, err)
	}
	sourceAbs = filepath.Clean(sourceAbs)
	destAbs = filepath.Clean(destAbs)
	if sourceAbs == destAbs {
		return true, false, nil
	}
	return false, pathWithinImportSkillRoot(destAbs, sourceAbs) || pathWithinImportSkillRoot(sourceAbs, destAbs), nil
}

func pathWithinImportSkillRoot(child, root string) bool {
	rel, err := filepath.Rel(root, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func rejectImportSkillSymlinks(root string) error {
	return filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("import-skill: stat source %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("import-skill: source contains symlink %s; symlinks are not supported in skill imports", current)
		}
		return nil
	})
}

func validateImportSkillName(kind, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("import-skill: %s is required", kind)
	}
	if value == "." || value == ".." || filepath.IsAbs(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\`) || len(value) >= 2 && value[1] == ':' {
		return fmt.Errorf("import-skill: %s %q must be a simple name", kind, value)
	}
	if strings.ContainsAny(value, `/\`) || filepath.Clean(value) != value {
		return fmt.Errorf("import-skill: %s %q must be a clean path component", kind, value)
	}
	return nil
}

func importSkillLayerResult(layer importSkillLayerInfo) ImportSkillLayer {
	return ImportSkillLayer{Kind: layer.Kind, Name: layer.Name, Profile: layer.Profile, Bucket: layer.Bucket, RootDir: layer.RootDir, ManifestPath: layer.ManifestPath}
}

func formatSkillIssues(issues []skills.Issue) string {
	if len(issues) == 0 {
		return "unknown validation failure"
	}
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		message := issue.Code
		if issue.Message != "" {
			message += ": " + issue.Message
		}
		if issue.Path != "" {
			message += " (" + issue.Path + ")"
		}
		parts = append(parts, message)
	}
	return strings.Join(parts, "; ")
}
