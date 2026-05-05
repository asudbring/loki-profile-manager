package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type writeFileEntryYAML struct {
	ID      string   `yaml:"id"`
	Source  string   `yaml:"source"`
	Target  string   `yaml:"target"`
	Mode    string   `yaml:"mode"`
	Format  string   `yaml:"format,omitempty"`
	Capture bool     `yaml:"capture,omitempty"`
	Secrets []string `yaml:"secrets,omitempty"`
}

type writeSkillEntryYAML struct {
	Source  string   `yaml:"source"`
	Targets []string `yaml:"targets,omitempty"`
}

type writeManifestYAML struct {
	Version    int                    `yaml:"version"`
	Name       string                 `yaml:"name"`
	Files      []writeFileEntryYAML   `yaml:"files"`
	Skills     []writeSkillEntryYAML  `yaml:"skills"`
	Ignore     []string               `yaml:"ignore"`
	MergeRules map[string]string      `yaml:"merge_rules"`
	Targets    map[string]TargetValue `yaml:"targets"`
}

// Marshal serializes a manifest using Loki's stable YAML shape.
func Marshal(m Manifest) ([]byte, error) {
	if m.Version == 0 {
		m.Version = Version
	}
	normalize(&m)
	doc := writeManifestYAML{
		Version:    m.Version,
		Name:       m.Name,
		Files:      make([]writeFileEntryYAML, 0, len(m.Files)),
		Skills:     make([]writeSkillEntryYAML, 0, len(m.Skills)),
		Ignore:     m.Ignore,
		MergeRules: m.MergeRules,
		Targets:    m.Targets,
	}
	for _, file := range m.Files {
		doc.Files = append(doc.Files, writeFileEntryYAML{ID: file.ID, Source: file.Source, Target: file.Target, Mode: file.Mode, Format: omitTextFormat(file.Format), Capture: file.Capture, Secrets: file.Secrets})
	}
	for _, skill := range m.Skills {
		doc.Skills = append(doc.Skills, writeSkillEntryYAML{Source: skill.Source, Targets: skill.Targets})
	}
	content, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	return content, nil
}

// WriteFile atomically writes a manifest file.
func WriteFile(path string, m Manifest) error {
	content, err := Marshal(m)
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
	if format == FormatText {
		return ""
	}
	return format
}
