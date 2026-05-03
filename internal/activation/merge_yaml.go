package activation

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func MergeYAMLBytes(sources []string) ([]byte, error) {
	merged, err := mergeStructured("yaml", sources)
	if err != nil {
		return nil, err
	}
	content, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("marshal merged YAML: %w", err)
	}
	return content, nil
}

func WriteYAMLMerge(sources []string, target string) error {
	content, err := MergeYAMLBytes(sources)
	if err != nil {
		return err
	}
	return writeFileAtomic(target, content, 0o644)
}

func parseYAML(path string) (any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read YAML source %s: %w", path, err)
	}
	var value any
	if err := yaml.Unmarshal(content, &value); err != nil {
		return nil, fmt.Errorf("parse YAML %s: %w", path, err)
	}
	return normalizeMaps(value), nil
}
