package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/manifest"
	"github.com/asudbring/loki-profile-manager/internal/profile"
	"github.com/asudbring/loki-profile-manager/internal/store"
)

var importPluginSupportedRuntimes = []string{"pi", "copilot", "claude", "codex", "vscode"}

type ImportPluginRequest struct {
	StorePath    string
	SourceFolder string
	Common       bool
	Profile      string
	Bucket       string
	Name         string
	Runtimes     []string
	DryRun       bool
	Yes          bool
	Overwrite    bool
}

type ImportPluginResult struct {
	StorePath         string                    `json:"store_path"`
	SourcePath        string                    `json:"source_path"`
	SourceKind        string                    `json:"source_kind,omitempty"`
	Name              string                    `json:"name"`
	Version           string                    `json:"version,omitempty"`
	Description       string                    `json:"description,omitempty"`
	Layer             ImportSkillLayer          `json:"layer"`
	DestinationPath   string                    `json:"destination_path"`
	ManifestPath      string                    `json:"manifest_path"`
	ManifestSource    string                    `json:"manifest_source"`
	SourceHash        string                    `json:"source_hash,omitempty"`
	ExistingHash      string                    `json:"existing_hash,omitempty"`
	DryRun            bool                      `json:"dry_run"`
	Overwrite         bool                      `json:"overwrite"`
	DestinationExists bool                      `json:"destination_exists"`
	WouldCopy         bool                      `json:"would_copy"`
	WouldOverwrite    bool                      `json:"would_overwrite"`
	ManifestChanged   bool                      `json:"manifest_changed"`
	Changed           int                       `json:"changed"`
	Runtimes          []ImportPluginRuntimePlan `json:"runtimes"`
	Warnings          []string                  `json:"warnings,omitempty"`
}

type ImportPluginRuntimePlan struct {
	Runtime     string               `json:"runtime"`
	Supported   bool                 `json:"supported"`
	Actions     []ImportPluginAction `json:"actions,omitempty"`
	ManualSteps []string             `json:"manual_steps,omitempty"`
	Warnings    []string             `json:"warnings,omitempty"`
}

type ImportPluginAction struct {
	Kind        string `json:"kind"`
	Source      string `json:"source,omitempty"`
	Target      string `json:"target,omitempty"`
	Mode        string `json:"mode,omitempty"`
	Format      string `json:"format,omitempty"`
	Description string `json:"description,omitempty"`
}

type importPluginMetadata struct {
	Name             string
	Version          string
	Description      string
	PluginJSON       importPluginJSON
	PackageJSON      importPluginPackageJSON
	HasPluginJSON    bool
	HasPackageJSON   bool
	HasSkillAssets   bool
	HasPiPackageData bool
}

type importPluginJSON struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Skills      string `json:"skills"`
	Commands    string `json:"commands"`
	Hooks       string `json:"hooks"`
}

type importPluginPackageJSON struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Pi          json.RawMessage `json:"pi"`
}

type importPluginGeneratedFile struct {
	Path    string
	Content []byte
	Changed bool
}

type importPluginManifestUpdate struct {
	Path     string
	Manifest manifest.Manifest
	Changed  bool
}

func (s *Service) ImportPlugin(ctx context.Context, req ImportPluginRequest) (ImportPluginResult, error) {
	if s == nil {
		return ImportPluginResult{}, fmt.Errorf("import-plugin: service is nil")
	}
	if req.DryRun == req.Yes {
		return ImportPluginResult{}, fmt.Errorf("import-plugin: run exactly one of --dry-run or --yes")
	}
	runtimes, err := normalizeImportPluginRuntimes(req.Runtimes)
	if err != nil {
		return ImportPluginResult{}, err
	}
	storePath, err := s.effectiveStorePath(ctx, req.StorePath)
	if err != nil {
		return ImportPluginResult{}, err
	}
	if validation := store.ValidateLayout(storePath); !validation.Valid {
		return ImportPluginResult{}, fmt.Errorf("import-plugin: invalid store layout: missing %v", validation.Missing)
	}
	layer, err := resolveImportPluginLayer(storePath, req)
	if err != nil {
		return ImportPluginResult{}, err
	}
	sourcePath, err := cleanImportPluginSource(req.SourceFolder)
	if err != nil {
		return ImportPluginResult{}, err
	}
	metadata, err := readImportPluginMetadata(sourcePath)
	if err != nil {
		return ImportPluginResult{}, err
	}
	if err := validateImportPluginRuntimeMetadata(metadata, runtimes); err != nil {
		return ImportPluginResult{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = firstNonEmptyString(metadata.Name, filepath.Base(sourcePath))
	}
	if err := validateImportPluginName("plugin name", name); err != nil {
		return ImportPluginResult{}, err
	}

	result := ImportPluginResult{
		StorePath:       storePath,
		SourcePath:      sourcePath,
		SourceKind:      "directory",
		Name:            name,
		Version:         metadata.Version,
		Description:     metadata.Description,
		Layer:           importSkillLayerResult(layer),
		ManifestPath:    layer.ManifestPath,
		DryRun:          req.DryRun,
		Overwrite:       req.Overwrite,
		ManifestSource:  path.Join("plugins", name),
		DestinationPath: filepath.Join(layer.RootDir, "plugins", name),
	}
	if err := rejectImportSkillSymlinks(sourcePath); err != nil {
		return result, err
	}
	same, overlaps, err := importSkillPathRelationship(sourcePath, result.DestinationPath)
	if err != nil {
		return result, err
	}
	if same {
		result.Warnings = append(result.Warnings, "source is already the selected layer destination; copy skipped")
	}
	if overlaps && !same {
		return result, fmt.Errorf("import-plugin: source %s overlaps destination %s", sourcePath, result.DestinationPath)
	}
	if err := s.planImportPlugin(ctx, req, layer, sourcePath, metadata, runtimes, &result); err != nil {
		return result, err
	}
	if req.DryRun {
		return result, nil
	}

	err = s.withStoreOperationLock(ctx, storePath, "import-plugin", req.Yes, func(machineID string) error {
		return s.executeImportPlugin(ctx, req, layer, sourcePath, &result)
	})
	return result, err
}

func (s *Service) planImportPlugin(ctx context.Context, req ImportPluginRequest, layer importSkillLayerInfo, sourcePath string, metadata importPluginMetadata, runtimes []string, result *ImportPluginResult) error {
	_ = ctx
	planned, manifestChanged, generated, runtimePlans, extraManifests, err := s.prepareImportPluginPlan(req, layer, metadata, runtimes, result)
	if err != nil {
		return err
	}
	_ = planned
	_ = extraManifests
	result.ManifestChanged = manifestChanged
	result.Runtimes = runtimePlans
	for _, plan := range runtimePlans {
		result.Warnings = append(result.Warnings, plan.Warnings...)
	}
	if err := refreshImportPluginCopyState(sourcePath, result, req.Overwrite); err != nil {
		return err
	}
	for _, generatedFile := range generated {
		if generatedFile.Changed {
			result.ManifestChanged = true
			break
		}
	}
	result.WouldOverwrite = result.DestinationExists && result.WouldCopy && req.Overwrite
	return nil
}

func (s *Service) executeImportPlugin(ctx context.Context, req ImportPluginRequest, layer importSkillLayerInfo, sourcePath string, result *ImportPluginResult) error {
	_ = ctx
	metadata, err := readImportPluginMetadata(sourcePath)
	if err != nil {
		return err
	}
	runtimes, err := normalizeImportPluginRuntimes(req.Runtimes)
	if err != nil {
		return err
	}
	parsed, manifestChanged, generated, runtimePlans, extraManifests, err := s.prepareImportPluginPlan(req, layer, metadata, runtimes, result)
	if err != nil {
		return err
	}
	result.ManifestChanged = manifestChanged
	result.Runtimes = runtimePlans
	if err := refreshImportPluginCopyState(sourcePath, result, req.Overwrite); err != nil {
		return err
	}
	if result.DestinationExists && result.WouldCopy && !req.Overwrite {
		return fmt.Errorf("import-plugin: destination %s already exists; rerun with --overwrite to replace it", result.DestinationPath)
	}
	if err := ensureImportSkillLayerDirs(layer); err != nil {
		return err
	}
	if result.WouldCopy {
		if err := copyImportPluginBundleAtomic(sourcePath, result.DestinationPath); err != nil {
			return err
		}
	}
	for _, generatedFile := range generated {
		if !generatedFile.Changed {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(generatedFile.Path), 0o755); err != nil {
			return fmt.Errorf("import-plugin: create generated file parent %s: %w", generatedFile.Path, err)
		}
		if err := os.WriteFile(generatedFile.Path, generatedFile.Content, 0o644); err != nil {
			return fmt.Errorf("import-plugin: write generated file %s: %w", generatedFile.Path, err)
		}
	}
	for _, update := range extraManifests {
		if !update.Changed {
			continue
		}
		if err := manifest.WriteFile(update.Path, update.Manifest); err != nil {
			return err
		}
	}
	if manifestChanged {
		if err := manifest.WriteFile(layer.ManifestPath, parsed); err != nil {
			return err
		}
	}
	if result.WouldCopy || manifestChanged || anyGeneratedImportPluginFileChanged(generated) {
		result.Changed = 1
	}
	return nil
}

func (s *Service) prepareImportPluginPlan(req ImportPluginRequest, layer importSkillLayerInfo, metadata importPluginMetadata, runtimes []string, result *ImportPluginResult) (manifest.Manifest, bool, []importPluginGeneratedFile, []ImportPluginRuntimePlan, []importPluginManifestUpdate, error) {
	parsed, err := parseImportPluginManifest(layer)
	if err != nil {
		return manifest.Manifest{}, false, nil, nil, nil, err
	}
	manifestChanged := false
	if metadata.HasSkillAssets {
		changed := upsertImportPluginSkillEntry(&parsed, path.Join(result.ManifestSource, "skills"))
		manifestChanged = manifestChanged || changed
	}
	var generated []importPluginGeneratedFile
	var plans []ImportPluginRuntimePlan
	var extraManifests []importPluginManifestUpdate
	for _, runtimeName := range runtimes {
		var plan ImportPluginRuntimePlan
		switch runtimeName {
		case "pi":
			selectedChanged, selectedWarnings, err := s.normalizeImportPluginPiSettingsEntries(&parsed, layer.RootDir, layer.ManifestPath, "~/.pi/agent/settings.json")
			if err != nil {
				return manifest.Manifest{}, false, nil, nil, nil, err
			}
			manifestChanged = manifestChanged || selectedChanged
			var files []importPluginGeneratedFile
			var updates []importPluginManifestUpdate
			plan, files, updates, err = s.prepareImportPluginPiPlan(req, layer, result)
			if err != nil {
				return manifest.Manifest{}, false, nil, nil, nil, err
			}
			plan.Warnings = append(selectedWarnings, plan.Warnings...)
			generated = append(generated, files...)
			extraManifests = append(extraManifests, updates...)
			for _, update := range updates {
				manifestChanged = manifestChanged || update.Changed
			}
			for _, action := range plan.Actions {
				if action.Kind != "manifest-file" {
					continue
				}
				entry := manifest.FileEntry{ID: action.Description, Source: action.Source, Target: action.Target, Mode: action.Mode, Format: action.Format}
				if entry.ID == "" {
					entry.ID = importPluginFileID(result.Name, runtimeName, action.Target)
				}
				changed := upsertImportPluginFileEntry(&parsed, entry)
				manifestChanged = manifestChanged || changed
			}
		case "copilot":
			plan = prepareImportPluginCopilotPlan(result)
		case "claude", "codex", "vscode":
			plan = prepareImportPluginDeferredRuntimePlan(runtimeName)
		default:
			return manifest.Manifest{}, false, nil, nil, nil, fmt.Errorf("import-plugin: unsupported runtime %q", runtimeName)
		}
		plans = append(plans, plan)
	}
	return parsed, manifestChanged, generated, plans, extraManifests, nil
}

func (s *Service) prepareImportPluginPiPlan(req ImportPluginRequest, layer importSkillLayerInfo, result *ImportPluginResult) (ImportPluginRuntimePlan, []importPluginGeneratedFile, []importPluginManifestUpdate, error) {
	plan := ImportPluginRuntimePlan{Runtime: "pi", Supported: true}
	packageTarget := path.Join("~/.pi/agent/packages", result.Name)
	packageID := importPluginFileID(result.Name, "pi", "package")
	plan.Actions = append(plan.Actions, ImportPluginAction{Kind: "manifest-file", Source: result.ManifestSource, Target: packageTarget, Mode: manifest.ModeCopy, Format: manifest.FormatText, Description: packageID})

	settingsSource := path.Join("files", "dot-pi", "agent", "settings.json")
	settingsTarget := "~/.pi/agent/settings.json"
	settingsID := importPluginFileID(result.Name, "pi", "settings")
	updates, warnings, err := s.prepareImportPluginPiSettingsManifestUpdates(req, layer, settingsTarget)
	if err != nil {
		return plan, nil, nil, err
	}
	plan.Warnings = append(plan.Warnings, warnings...)
	settings, err := s.effectiveImportPluginJSON(req, settingsTarget)
	if err != nil {
		return plan, nil, nil, err
	}
	packageSpec := "./packages/" + result.Name
	changedPackages, err := appendImportPluginPackage(settings, packageSpec)
	if err != nil {
		return plan, nil, nil, err
	}
	settingsBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return plan, nil, nil, fmt.Errorf("import-plugin: marshal pi settings: %w", err)
	}
	settingsBytes = append(settingsBytes, '\n')
	settingsPath := filepath.Join(layer.RootDir, filepath.FromSlash(settingsSource))
	generatedChanged := changedPackages || !fileContentEqual(settingsPath, settingsBytes)
	generated := []importPluginGeneratedFile{{Path: settingsPath, Content: settingsBytes, Changed: generatedChanged}}
	plan.Actions = append(plan.Actions, ImportPluginAction{Kind: "manifest-file", Source: settingsSource, Target: settingsTarget, Mode: manifest.ModeMerge, Format: manifest.FormatJSON, Description: settingsID})
	return plan, generated, updates, nil
}

func (s *Service) prepareImportPluginPiSettingsManifestUpdates(req ImportPluginRequest, selected importSkillLayerInfo, target string) ([]importPluginManifestUpdate, []string, error) {
	storePath, err := s.effectiveStorePath(context.Background(), req.StorePath)
	if err != nil {
		return nil, nil, err
	}
	layers, err := importPluginEffectiveLayers(storePath, req)
	if err != nil {
		return nil, nil, err
	}
	var updates []importPluginManifestUpdate
	var warnings []string
	for _, layer := range layers {
		if layer.ManifestPath == selected.ManifestPath {
			continue
		}
		changed, layerWarnings, err := s.normalizeImportPluginPiSettingsEntries(&layer.Manifest, layer.RootDir, layer.ManifestPath, target)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, layerWarnings...)
		if changed {
			updates = append(updates, importPluginManifestUpdate{Path: layer.ManifestPath, Manifest: layer.Manifest, Changed: true})
		}
	}
	return updates, warnings, nil
}

func (s *Service) normalizeImportPluginPiSettingsEntries(parsed *manifest.Manifest, layerRoot, manifestPath, target string) (bool, []string, error) {
	expander := manifest.Expander{Resolver: s.resolver, Targets: parsed.Targets}
	expectedTarget, err := expander.ExpandTarget(target)
	if err != nil {
		return false, nil, err
	}
	changed := false
	var warnings []string
	for i := range parsed.Files {
		entry := &parsed.Files[i]
		expanded, err := expander.ExpandTarget(entry.Target)
		if err != nil || expanded != expectedTarget {
			continue
		}
		switch entry.Mode {
		case manifest.ModeMerge:
			if entry.Format == "" {
				entry.Format = manifest.FormatJSON
				changed = true
			} else if entry.Format != manifest.FormatJSON {
				return false, nil, fmt.Errorf("import-plugin: existing pi settings merge entry %s in %s must use json format", entry.ID, manifestPath)
			}
		case manifest.ModeCopy:
			entry.Mode = manifest.ModeMerge
			entry.Format = manifest.FormatJSON
			changed = true
			warnings = append(warnings, fmt.Sprintf("converted existing pi settings entry %s in %s from copy to merge to avoid duplicate target conflicts", firstNonEmptyString(entry.ID, entry.Target), manifestPath))
		default:
			return false, nil, fmt.Errorf("import-plugin: existing pi settings entry %s in %s uses unsupported mode %q", firstNonEmptyString(entry.ID, entry.Target), manifestPath, entry.Mode)
		}
		if entry.Source != "" {
			if resolved, err := manifest.ResolveSource(layerRoot, entry.Source); err == nil {
				if err := manifest.ValidateSourceWithinRoot(layerRoot, resolved); err != nil {
					return false, nil, err
				}
			}
		}
	}
	return changed, warnings, nil
}

func prepareImportPluginCopilotPlan(result *ImportPluginResult) ImportPluginRuntimePlan {
	return ImportPluginRuntimePlan{
		Runtime:   "copilot",
		Supported: true,
		ManualSteps: []string{
			fmt.Sprintf("After `loki switch` activates this layer, run: gh copilot -- plugin install %q", result.DestinationPath),
		},
		Warnings: []string{"copilot plugin registration is not edited by Loki yet; direct Copilot internals are intentionally left untouched"},
	}
}

func prepareImportPluginDeferredRuntimePlan(runtimeName string) ImportPluginRuntimePlan {
	return ImportPluginRuntimePlan{
		Runtime:   runtimeName,
		Supported: true,
		Warnings:  []string{fmt.Sprintf("%s runtime adapter recognized but has no safe file actions in this MVP; bundle will be stored only", runtimeName)},
	}
}

func parseImportPluginManifest(layer importSkillLayerInfo) (manifest.Manifest, error) {
	parsed, err := manifest.ParseFile(layer.ManifestPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return manifest.Manifest{}, err
		}
		parsed = manifest.Manifest{Version: manifest.Version, Name: layer.Name, Files: []manifest.FileEntry{}, Skills: []manifest.SkillEntry{}, Ignore: []string{}, MergeRules: map[string]string{}, Targets: map[string]manifest.TargetValue{}}
	}
	return parsed, nil
}

func resolveImportPluginLayer(storePath string, req ImportPluginRequest) (importSkillLayerInfo, error) {
	profileName := strings.TrimSpace(req.Profile)
	bucket := strings.TrimSpace(req.Bucket)
	if req.Common {
		if profileName != "" || bucket != "" {
			return importSkillLayerInfo{}, fmt.Errorf("import-plugin: --common cannot be combined with --profile or --bucket")
		}
		root := filepath.Join(storePath, "profiles", "common")
		return importSkillLayerInfo{StorePath: storePath, Kind: "common", Name: "common", RootDir: root, ManifestPath: filepath.Join(root, "manifest.yaml")}, nil
	}
	if profileName == "" {
		return importSkillLayerInfo{}, fmt.Errorf("import-plugin: choose --common or --profile <profile>")
	}
	if err := validateImportPluginName("profile", profileName); err != nil {
		return importSkillLayerInfo{}, err
	}
	if bucket == "" {
		root := filepath.Join(storePath, "profiles", profileName, "core")
		return importSkillLayerInfo{StorePath: storePath, Kind: "core", Name: profileName + "-core", Profile: profileName, RootDir: root, ManifestPath: filepath.Join(root, "manifest.yaml")}, nil
	}
	if err := validateImportPluginName("bucket", bucket); err != nil {
		return importSkillLayerInfo{}, err
	}
	coreManifest := filepath.Join(storePath, "profiles", profileName, "core", "manifest.yaml")
	if info, err := os.Stat(coreManifest); err != nil || info.IsDir() {
		if err == nil {
			err = fmt.Errorf("manifest is a directory")
		}
		return importSkillLayerInfo{}, fmt.Errorf("import-plugin: profile %q does not exist: %w", profileName, err)
	}
	root := filepath.Join(storePath, "profiles", profileName, "buckets", bucket)
	return importSkillLayerInfo{StorePath: storePath, Kind: "bucket", Name: bucket, Profile: profileName, Bucket: bucket, RootDir: root, ManifestPath: filepath.Join(root, "manifest.yaml")}, nil
}

func cleanImportPluginSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("import-plugin: source is required")
	}
	cleaned, err := filepath.Abs(source)
	if err != nil {
		return "", fmt.Errorf("import-plugin: resolve source %s: %w", source, err)
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("import-plugin: source %s: %w", cleaned, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("import-plugin: source %s must be a directory", cleaned)
	}
	return cleaned, nil
}

func copyImportPluginBundle(sourcePath, destinationPath string) error {
	return filepath.WalkDir(sourcePath, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Name() == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(sourcePath, current)
		if err != nil {
			return fmt.Errorf("import-plugin: resolve relative source path %s: %w", current, err)
		}
		if relative == "." {
			return os.MkdirAll(destinationPath, 0o755)
		}
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("import-plugin: stat source %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("import-plugin: source contains symlink %s; symlinks are not supported in plugin imports", current)
		}
		target := filepath.Join(destinationPath, relative)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		content, err := os.ReadFile(current)
		if err != nil {
			return fmt.Errorf("import-plugin: read source %s: %w", current, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("import-plugin: create parent directory for %s: %w", target, err)
		}
		if err := os.WriteFile(target, content, info.Mode().Perm()); err != nil {
			return fmt.Errorf("import-plugin: write staged source %s: %w", target, err)
		}
		return nil
	})
}

func copyImportPluginBundleAtomic(sourcePath, destinationPath string) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return fmt.Errorf("import-plugin: create destination parent %s: %w", filepath.Dir(destinationPath), err)
	}
	tmp, err := os.MkdirTemp(filepath.Dir(destinationPath), ".loki-import-plugin-*")
	if err != nil {
		return fmt.Errorf("import-plugin: create staging directory in store: %w", err)
	}
	defer os.RemoveAll(tmp)
	payload := filepath.Join(tmp, "payload")
	if err := copyImportPluginBundle(sourcePath, payload); err != nil {
		return err
	}
	return activation.CopyPath(payload, destinationPath)
}

func refreshImportPluginCopyState(sourcePath string, result *ImportPluginResult, overwrite bool) error {
	sourceHash, err := hashImportPluginBundle(sourcePath)
	if err != nil {
		return err
	}
	result.SourceHash = sourceHash
	result.ExistingHash = ""
	result.DestinationExists = false
	result.WouldCopy = false
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
		return fmt.Errorf("import-plugin: stat destination %s: %w", result.DestinationPath, err)
	}
	result.WouldOverwrite = result.DestinationExists && result.WouldCopy && overwrite
	return nil
}

func hashImportPluginBundle(sourcePath string) (string, error) {
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return "", fmt.Errorf("hash plugin bundle %s: %w", sourcePath, err)
	}
	h := sha256.New()
	if !info.IsDir() {
		return hashImportPluginFile(sourcePath)
	}
	err = filepath.WalkDir(sourcePath, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == sourcePath {
			return nil
		}
		if entry.Name() == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(sourcePath, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("import-plugin: source contains symlink %s; symlinks are not supported in plugin imports", current)
		}
		if entry.IsDir() {
			_, _ = io.WriteString(h, "dir\x00"+rel+"\n")
			return nil
		}
		fileHash, err := hashImportPluginFile(current)
		if err != nil {
			return err
		}
		_, _ = io.WriteString(h, strings.Join([]string{"file", rel, info.Mode().Perm().String(), fileHash}, "\x00")+"\n")
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("hash plugin bundle %s: %w", sourcePath, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashImportPluginFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readImportPluginMetadata(sourcePath string) (importPluginMetadata, error) {
	var metadata importPluginMetadata
	pluginPath := filepath.Join(sourcePath, "plugin.json")
	if content, err := os.ReadFile(pluginPath); err == nil {
		if err := json.Unmarshal(content, &metadata.PluginJSON); err != nil {
			return metadata, fmt.Errorf("import-plugin: parse %s: %w", pluginPath, err)
		}
		metadata.HasPluginJSON = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return metadata, fmt.Errorf("import-plugin: read %s: %w", pluginPath, err)
	}
	packagePath := filepath.Join(sourcePath, "package.json")
	if content, err := os.ReadFile(packagePath); err == nil {
		if err := json.Unmarshal(content, &metadata.PackageJSON); err != nil {
			return metadata, fmt.Errorf("import-plugin: parse %s: %w", packagePath, err)
		}
		metadata.HasPackageJSON = true
		metadata.HasPiPackageData = len(bytes.TrimSpace(metadata.PackageJSON.Pi)) > 0
	} else if !errors.Is(err, os.ErrNotExist) {
		return metadata, fmt.Errorf("import-plugin: read %s: %w", packagePath, err)
	}
	if !metadata.HasPluginJSON && !metadata.HasPackageJSON {
		return metadata, fmt.Errorf("import-plugin: source %s must contain plugin.json or package.json", sourcePath)
	}
	metadata.Name = firstNonEmptyString(strings.TrimSpace(metadata.PluginJSON.Name), strings.TrimSpace(metadata.PackageJSON.Name))
	metadata.Version = firstNonEmptyString(strings.TrimSpace(metadata.PluginJSON.Version), strings.TrimSpace(metadata.PackageJSON.Version))
	metadata.Description = firstNonEmptyString(strings.TrimSpace(metadata.PluginJSON.Description), strings.TrimSpace(metadata.PackageJSON.Description))
	metadata.HasSkillAssets = importPluginHasSkillAssets(sourcePath, metadata.PluginJSON.Skills)
	return metadata, nil
}

func validateImportPluginRuntimeMetadata(metadata importPluginMetadata, runtimes []string) error {
	for _, runtimeName := range runtimes {
		switch runtimeName {
		case "pi":
			if !metadata.HasPackageJSON {
				return fmt.Errorf("import-plugin: --runtime pi requires package.json")
			}
		case "copilot":
			if !metadata.HasPluginJSON {
				return fmt.Errorf("import-plugin: --runtime copilot requires plugin.json")
			}
		}
	}
	return nil
}

func importPluginHasSkillAssets(sourcePath, pluginSkills string) bool {
	candidates := []string{}
	if strings.TrimSpace(pluginSkills) != "" {
		candidates = append(candidates, strings.TrimSpace(pluginSkills))
	}
	candidates = append(candidates, "skills")
	for _, candidate := range candidates {
		root := filepath.Join(sourcePath, filepath.FromSlash(candidate))
		if hasSkillMarkdown(root) {
			return true
		}
	}
	return false
}

func hasSkillMarkdown(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "SKILL.md")); err == nil {
		return true
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), "SKILL.md")); err == nil {
			return true
		}
	}
	return false
}

func normalizeImportPluginRuntimes(values []string) ([]string, error) {
	var raw []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.ToLower(strings.TrimSpace(part))
			if part != "" {
				raw = append(raw, part)
			}
		}
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("import-plugin: at least one --runtime is required")
	}
	hasAll := false
	for _, value := range raw {
		if value == "all" {
			hasAll = true
			break
		}
	}
	if hasAll && len(raw) > 1 {
		return nil, fmt.Errorf("import-plugin: --runtime all cannot be combined with other runtimes")
	}
	seen := map[string]bool{}
	var out []string
	for _, value := range raw {
		if value == "all" {
			for _, runtimeName := range importPluginSupportedRuntimes {
				if !seen[runtimeName] {
					seen[runtimeName] = true
					out = append(out, runtimeName)
				}
			}
			continue
		}
		if !importPluginRuntimeSupported(value) {
			return nil, fmt.Errorf("import-plugin: unsupported runtime %q (supported: %s)", value, strings.Join(importPluginSupportedRuntimes, ", "))
		}
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out, nil
}

func importPluginRuntimeSupported(value string) bool {
	for _, runtimeName := range importPluginSupportedRuntimes {
		if value == runtimeName {
			return true
		}
	}
	return false
}

func upsertImportPluginSkillEntry(parsed *manifest.Manifest, source string) bool {
	entry := manifest.SkillEntry{Source: source}
	for i, existing := range parsed.Skills {
		if existing.Source != source {
			continue
		}
		if len(entry.Targets) == 0 {
			entry.Targets = existing.Targets
		}
		if importPluginSkillEntryEqual(existing, entry) {
			return false
		}
		parsed.Skills[i] = entry
		return true
	}
	parsed.Skills = append(parsed.Skills, entry)
	return true
}

func upsertImportPluginFileEntry(parsed *manifest.Manifest, entry manifest.FileEntry) bool {
	for i, existing := range parsed.Files {
		if existing.ID != entry.ID && existing.Target != entry.Target {
			continue
		}
		if existing.ID != "" && existing.Target == entry.Target {
			entry.ID = existing.ID
		}
		if existing.Capture {
			entry.Capture = existing.Capture
		}
		if len(entry.Secrets) == 0 {
			entry.Secrets = existing.Secrets
		}
		if importPluginFileEntryEqual(existing, entry) {
			return false
		}
		parsed.Files[i] = entry
		return true
	}
	parsed.Files = append(parsed.Files, entry)
	return true
}

func importPluginSkillEntryEqual(left, right manifest.SkillEntry) bool {
	if left.Source != right.Source || len(left.Targets) != len(right.Targets) {
		return false
	}
	for i := range left.Targets {
		if left.Targets[i] != right.Targets[i] {
			return false
		}
	}
	return true
}

func importPluginFileEntryEqual(left, right manifest.FileEntry) bool {
	if left.ID != right.ID || left.Source != right.Source || left.Target != right.Target || left.Mode != right.Mode || left.Format != right.Format || left.Capture != right.Capture {
		return false
	}
	if len(left.Secrets) != len(right.Secrets) {
		return false
	}
	for i := range left.Secrets {
		if left.Secrets[i] != right.Secrets[i] {
			return false
		}
	}
	return true
}

func (s *Service) effectiveImportPluginJSON(req ImportPluginRequest, target string) (map[string]any, error) {
	content, found, err := s.effectiveImportPluginTargetContent(req, target)
	if err != nil {
		return nil, err
	}
	if !found {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(content, &out); err != nil {
		return nil, fmt.Errorf("import-plugin: parse effective JSON for %s: %w", target, err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func (s *Service) effectiveImportPluginTargetContent(req ImportPluginRequest, target string) ([]byte, bool, error) {
	storePath, err := s.effectiveStorePath(context.Background(), req.StorePath)
	if err != nil {
		return nil, false, err
	}
	layers, err := importPluginEffectiveLayers(storePath, req)
	if err != nil {
		return nil, false, err
	}
	expectedTarget, err := (manifest.Expander{Resolver: s.resolver}).ExpandTarget(target)
	if err != nil {
		return nil, false, err
	}
	var sources []string
	for _, layer := range layers {
		expander := manifest.Expander{Resolver: s.resolver, Targets: layer.Manifest.Targets}
		for _, entry := range layer.Manifest.Files {
			expanded, err := expander.ExpandTarget(entry.Target)
			if err != nil || expanded != expectedTarget {
				continue
			}
			sourcePath, err := manifest.ResolveSource(layer.RootDir, entry.Source)
			if err != nil {
				return nil, false, err
			}
			sources = append(sources, sourcePath)
		}
	}
	if len(sources) == 1 {
		content, err := os.ReadFile(sources[0])
		if err != nil {
			return nil, false, fmt.Errorf("import-plugin: read effective source %s: %w", sources[0], err)
		}
		return content, true, nil
	}
	if len(sources) > 1 {
		content, err := activation.MergeBytes(manifest.FormatJSON, sources)
		if err != nil {
			return nil, false, err
		}
		return content, true, nil
	}
	if content, err := os.ReadFile(expectedTarget); err == nil {
		return content, true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("import-plugin: read current target %s: %w", expectedTarget, err)
	}
	return nil, false, nil
}

func importPluginEffectiveLayers(storePath string, req ImportPluginRequest) ([]profile.Layer, error) {
	if req.Common {
		parsed, err := manifest.ParseFile(filepath.Join(storePath, "profiles", "common", "manifest.yaml"))
		if err != nil {
			return nil, err
		}
		return []profile.Layer{{Name: "common", Kind: profile.LayerCommon, RootDir: filepath.Join(storePath, "profiles", "common"), ManifestPath: filepath.Join(storePath, "profiles", "common", "manifest.yaml"), Manifest: parsed}}, nil
	}
	buckets := []string{}
	if strings.TrimSpace(req.Bucket) != "" {
		buckets = append(buckets, strings.TrimSpace(req.Bucket))
	}
	return profile.Resolve(storePath, req.Profile, buckets)
}

func appendImportPluginPackage(settings map[string]any, packageSpec string) (bool, error) {
	value, ok := settings["packages"]
	if !ok || value == nil {
		settings["packages"] = []any{packageSpec}
		return true, nil
	}
	items, ok := value.([]any)
	if !ok {
		return false, fmt.Errorf("import-plugin: pi settings packages must be an array")
	}
	for _, item := range items {
		if existing, ok := item.(string); ok && existing == packageSpec {
			return false, nil
		}
	}
	settings["packages"] = append(items, packageSpec)
	return true, nil
}

func anyGeneratedImportPluginFileChanged(files []importPluginGeneratedFile) bool {
	for _, file := range files {
		if file.Changed {
			return true
		}
	}
	return false
}

func fileContentEqual(path string, want []byte) bool {
	got, err := os.ReadFile(path)
	return err == nil && bytes.Equal(got, want)
}

func importPluginFileID(name, runtimeName, suffix string) string {
	return "plugin-" + sanitizeImportPluginID(name) + "-" + sanitizeImportPluginID(runtimeName) + "-" + sanitizeImportPluginID(suffix)
}

func sanitizeImportPluginID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(builder.String(), "-")
	if out == "" {
		return "plugin"
	}
	return out
}

func validateImportPluginName(kind, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("import-plugin: %s is required", kind)
	}
	if value == "." || value == ".." || filepath.IsAbs(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\`) || len(value) >= 2 && value[1] == ':' {
		return fmt.Errorf("import-plugin: %s %q must be a simple name", kind, value)
	}
	if strings.ContainsAny(value, `/\`) || filepath.Clean(value) != value {
		return fmt.Errorf("import-plugin: %s %q must be a clean path component", kind, value)
	}
	return nil
}
