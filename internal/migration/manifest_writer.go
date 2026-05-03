package migration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/allensu/loki-profile-manager/internal/manifest"
	"gopkg.in/yaml.v3"
)

type manifestFileYAML struct {
	ID      string   `yaml:"id"`
	Source  string   `yaml:"source"`
	Target  string   `yaml:"target"`
	Mode    string   `yaml:"mode"`
	Format  string   `yaml:"format,omitempty"`
	Capture bool     `yaml:"capture,omitempty"`
	Secrets []string `yaml:"secrets,omitempty"`
}

type manifestSkillYAML struct {
	Source  string   `yaml:"source"`
	Targets []string `yaml:"targets,omitempty"`
}

type manifestYAML struct {
	Version    int                             `yaml:"version"`
	Name       string                          `yaml:"name"`
	Files      []manifestFileYAML              `yaml:"files"`
	Skills     []manifestSkillYAML             `yaml:"skills"`
	Ignore     []string                        `yaml:"ignore"`
	MergeRules map[string]string               `yaml:"merge_rules"`
	Targets    map[string]manifest.TargetValue `yaml:"targets"`
}

func writeManifestItems(layer layerInfo, items []Item) error {
	planned, err := prepareManifestItems(layer, items)
	if err != nil {
		return err
	}
	if err := ensureLayerDirs(layer); err != nil {
		return err
	}
	return writeManifest(layer.ManifestPath, planned)
}

func prepareManifestItems(layer layerInfo, items []Item) (manifest.Manifest, error) {
	parsed, err := loadOrCreateManifest(layer)
	if err != nil {
		return manifest.Manifest{}, err
	}
	for _, item := range items {
		if item.ManifestPath != layer.ManifestPath {
			continue
		}
		if item.Collision == CollisionConflict {
			return manifest.Manifest{}, fmt.Errorf("manifest %s: item %s has conflicting store destination", layer.ManifestPath, item.ID)
		}
		fileEntry := manifest.FileEntry{ID: item.ID, Source: item.ManifestSource, Target: item.Target, Mode: item.Mode, Format: item.Format, Secrets: cloneStrings(item.Secrets)}
		if err := upsertFileEntry(&parsed, fileEntry); err != nil {
			return manifest.Manifest{}, err
		}
		if item.IsSkill {
			upsertSkillEntry(&parsed, manifest.SkillEntry{Source: item.SkillSource})
		}
	}
	return parsed, nil
}

func loadOrCreateManifest(layer layerInfo) (manifest.Manifest, error) {
	parsed, err := manifest.ParseFile(layer.ManifestPath)
	if err == nil {
		return parsed, nil
	}
	if !errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "no such file") {
		return manifest.Manifest{}, err
	}
	return manifest.Manifest{Version: manifest.Version, Name: layer.Name, Files: []manifest.FileEntry{}, Skills: []manifest.SkillEntry{}, Ignore: []string{}, MergeRules: map[string]string{}, Targets: map[string]manifest.TargetValue{}}, nil
}

func upsertFileEntry(m *manifest.Manifest, entry manifest.FileEntry) error {
	for i, existing := range m.Files {
		if existing.ID == entry.ID {
			if existing.Target == entry.Target && existing.Source == entry.Source {
				entry.Capture = existing.Capture
				m.Files[i] = entry
				return nil
			}
			return fmt.Errorf("manifest %s: file id %s already exists for a different source or target", m.Name, entry.ID)
		}
		if existing.Target == entry.Target && existing.Source == entry.Source {
			entry.Capture = existing.Capture
			m.Files[i] = entry
			return nil
		}
	}
	for _, existing := range m.Files {
		if existing.Target != entry.Target {
			continue
		}
		if existing.Mode == manifest.ModeMerge && entry.Mode == manifest.ModeMerge && existing.Format == entry.Format {
			continue
		}
		return fmt.Errorf("manifest %s: target %s already exists with non-merge entry", m.Name, entry.Target)
	}
	m.Files = append(m.Files, entry)
	return nil
}

func upsertSkillEntry(m *manifest.Manifest, entry manifest.SkillEntry) {
	for i, existing := range m.Skills {
		if existing.Source == entry.Source {
			if len(entry.Targets) == 0 {
				entry.Targets = existing.Targets
			}
			m.Skills[i] = entry
			return
		}
	}
	m.Skills = append(m.Skills, entry)
}

func writeManifest(path string, m manifest.Manifest) error {
	doc := manifestYAML{Version: m.Version, Name: m.Name, Files: make([]manifestFileYAML, 0, len(m.Files)), Skills: make([]manifestSkillYAML, 0, len(m.Skills)), Ignore: m.Ignore, MergeRules: m.MergeRules, Targets: m.Targets}
	if doc.Ignore == nil {
		doc.Ignore = []string{}
	}
	if doc.MergeRules == nil {
		doc.MergeRules = map[string]string{}
	}
	if doc.Targets == nil {
		doc.Targets = map[string]manifest.TargetValue{}
	}
	for _, file := range m.Files {
		doc.Files = append(doc.Files, manifestFileYAML{ID: file.ID, Source: file.Source, Target: file.Target, Mode: file.Mode, Format: omitTextFormat(file.Format), Capture: file.Capture, Secrets: file.Secrets})
	}
	for _, skill := range m.Skills {
		doc.Skills = append(doc.Skills, manifestSkillYAML{Source: skill.Source, Targets: skill.Targets})
	}
	content, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal manifest %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create manifest parent %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".manifest-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp manifest %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp manifest %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp manifest %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace manifest %s: %w", path, err)
	}
	return nil
}

func omitTextFormat(format string) string {
	if format == manifest.FormatText {
		return ""
	}
	return format
}
