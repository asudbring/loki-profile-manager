package activation

import (
	"encoding/json"
	"fmt"
	"os"
)

func MergeJSONBytes(sources []string) ([]byte, error) {
	merged, err := mergeStructured("json", sources)
	if err != nil {
		return nil, err
	}
	content, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal merged JSON: %w", err)
	}
	return append(content, '\n'), nil
}

func WriteJSONMerge(sources []string, target string) error {
	content, err := MergeJSONBytes(sources)
	if err != nil {
		return err
	}
	return writeFileAtomic(target, content, 0o644)
}

func parseJSON(path string) (any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read JSON source %s: %w", path, err)
	}
	var value any
	if err := json.Unmarshal(content, &value); err != nil {
		return nil, fmt.Errorf("parse JSON %s: %w", path, err)
	}
	return value, nil
}
