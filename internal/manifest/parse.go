package manifest

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func ParseFile(path string) (Manifest, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %s: %w", path, err)
	}
	return ParseBytes(path, content)
}

func ParseBytes(path string, content []byte) (Manifest, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if manifest.Version == 0 {
		return Manifest{}, fmt.Errorf("parse manifest %s: missing version", path)
	}
	if manifest.Version != Version {
		return Manifest{}, fmt.Errorf("parse manifest %s: unsupported version %d", path, manifest.Version)
	}
	normalize(&manifest)
	return manifest, nil
}

func normalize(manifest *Manifest) {
	if manifest.Files == nil {
		manifest.Files = []FileEntry{}
	}
	if manifest.Skills == nil {
		manifest.Skills = []SkillEntry{}
	}
	if manifest.Ignore == nil {
		manifest.Ignore = []string{}
	}
	if manifest.MergeRules == nil {
		manifest.MergeRules = map[string]string{}
	}
	if manifest.Targets == nil {
		manifest.Targets = map[string]TargetValue{}
	}
	for i := range manifest.Files {
		if manifest.Files[i].Secrets == nil {
			manifest.Files[i].Secrets = []string{}
		}
	}
	for i := range manifest.Skills {
		if manifest.Skills[i].Targets == nil {
			manifest.Skills[i].Targets = []string{}
		}
	}
}
