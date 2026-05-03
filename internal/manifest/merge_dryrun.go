package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

func MergeDryRun(operations []FileOperation) []Problem {
	groups := map[string][]FileOperation{}
	for _, op := range operations {
		groups[op.TargetPath] = append(groups[op.TargetPath], op)
	}
	var problems []Problem
	for target, group := range groups {
		if len(group) <= 1 {
			continue
		}
		if mergeGroup(group) {
			problems = append(problems, validateStructuredMerge(target, group)...)
			continue
		}
		if identicalSources(group) {
			continue
		}
		problems = append(problems, Problem{Severity: SeverityBlocking, Code: "merge.duplicate_target", Message: "duplicate target is not a compatible structured merge and source contents differ", Target: target})
	}
	return problems
}

func mergeGroup(group []FileOperation) bool {
	if len(group) == 0 || group[0].Entry.Mode != ModeMerge || !structuredFormat(group[0].Entry.Format) {
		return false
	}
	format := group[0].Entry.Format
	for _, op := range group[1:] {
		if op.Entry.Mode != ModeMerge || op.Entry.Format != format {
			return false
		}
	}
	return true
}

func validateStructuredMerge(target string, group []FileOperation) []Problem {
	var problems []Problem
	var merged any
	for i, op := range group {
		value, err := parseStructured(op.Entry.Format, op.SourcePath)
		if err != nil {
			problems = append(problems, Problem{Severity: SeverityBlocking, Code: "merge.parse_failed", Message: err.Error(), Path: op.SourcePath, Target: target, Layer: op.LayerName})
			continue
		}
		if !isMap(value) {
			problems = append(problems, Problem{Severity: SeverityBlocking, Code: "merge.root_not_object", Message: "structured merge root must be an object/table", Path: op.SourcePath, Target: target, Layer: op.LayerName})
			continue
		}
		if i == 0 {
			merged = value
			continue
		}
		if err := compatibleTypes(merged, value, "$"); err != nil {
			problems = append(problems, Problem{Severity: SeverityBlocking, Code: "merge.type_conflict", Message: err.Error(), Path: op.SourcePath, Target: target, Layer: op.LayerName})
			continue
		}
		merged = overlay(merged, value)
	}
	return problems
}

func parseStructured(format, path string) (any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read structured source %s: %w", path, err)
	}
	switch format {
	case FormatJSON:
		var value any
		if err := json.Unmarshal(content, &value); err != nil {
			return nil, fmt.Errorf("parse JSON %s: %w", path, err)
		}
		return value, nil
	case FormatYAML:
		var value any
		if err := yaml.Unmarshal(content, &value); err != nil {
			return nil, fmt.Errorf("parse YAML %s: %w", path, err)
		}
		return value, nil
	case FormatTOML:
		var value map[string]any
		if err := toml.Unmarshal(content, &value); err != nil {
			return nil, fmt.Errorf("parse TOML %s: %w", path, err)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported structured format %q", format)
	}
}

func compatibleTypes(left, right any, path string) error {
	if isMap(left) && isMap(right) {
		leftMap := asMap(left)
		rightMap := asMap(right)
		for key, rightValue := range rightMap {
			if leftValue, ok := leftMap[key]; ok {
				if err := compatibleTypes(leftValue, rightValue, path+"."+key); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if isMap(left) != isMap(right) {
		return fmt.Errorf("type conflict at %s: object/table cannot merge with %s", path, typeName(nonMap(left, right)))
	}
	return nil
}

func overlay(left, right any) any {
	if !isMap(left) || !isMap(right) {
		return right
	}
	out := map[string]any{}
	for key, value := range asMap(left) {
		out[key] = value
	}
	for key, rightValue := range asMap(right) {
		if leftValue, ok := out[key]; ok && isMap(leftValue) && isMap(rightValue) {
			out[key] = overlay(leftValue, rightValue)
			continue
		}
		out[key] = rightValue
	}
	return out
}

func isMap(value any) bool {
	_, ok := asMapOK(value)
	return ok
}

func asMap(value any) map[string]any {
	out, _ := asMapOK(value)
	return out
}

func asMapOK(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[interface{}]interface{}:
		out := map[string]any{}
		for key, value := range typed {
			out[fmt.Sprint(key)] = value
		}
		return out, true
	default:
		return nil, false
	}
}

func nonMap(left, right any) any {
	if isMap(left) {
		return right
	}
	return left
}

func typeName(value any) string {
	if value == nil {
		return "null"
	}
	return reflect.TypeOf(value).String()
}

func identicalSources(group []FileOperation) bool {
	if len(group) < 2 {
		return true
	}
	first, err := os.ReadFile(group[0].SourcePath)
	if err != nil {
		return false
	}
	for _, op := range group[1:] {
		content, err := os.ReadFile(op.SourcePath)
		if err != nil || !bytes.Equal(first, content) {
			return false
		}
	}
	return true
}
