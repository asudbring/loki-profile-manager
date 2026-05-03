package activation

import (
	"bytes"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

func MergeTOMLBytes(sources []string) ([]byte, error) {
	merged, err := mergeStructured("toml", sources)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	encoder := toml.NewEncoder(&buf)
	if err := encoder.Encode(merged); err != nil {
		return nil, fmt.Errorf("marshal merged TOML: %w", err)
	}
	return buf.Bytes(), nil
}

func WriteTOMLMerge(sources []string, target string) error {
	content, err := MergeTOMLBytes(sources)
	if err != nil {
		return err
	}
	return writeFileAtomic(target, content, 0o644)
}

func parseTOML(path string) (any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read TOML source %s: %w", path, err)
	}
	var value map[string]any
	if err := toml.Unmarshal(content, &value); err != nil {
		return nil, fmt.Errorf("parse TOML %s: %w", path, err)
	}
	return value, nil
}
