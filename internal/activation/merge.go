package activation

import (
	"fmt"
	"reflect"
)

func MergeBytes(format string, sources []string) ([]byte, error) {
	switch format {
	case "json":
		return MergeJSONBytes(sources)
	case "yaml":
		return MergeYAMLBytes(sources)
	case "toml":
		return MergeTOMLBytes(sources)
	default:
		return nil, fmt.Errorf("unsupported merge format %q", format)
	}
}

func WriteMerge(format string, sources []string, target string) error {
	switch format {
	case "json":
		return WriteJSONMerge(sources, target)
	case "yaml":
		return WriteYAMLMerge(sources, target)
	case "toml":
		return WriteTOMLMerge(sources, target)
	default:
		return fmt.Errorf("unsupported merge format %q", format)
	}
}

func mergeStructured(format string, sources []string) (any, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("merge %s: at least one source is required", format)
	}
	var merged any
	for i, source := range sources {
		value, err := parseStructured(format, source)
		if err != nil {
			return nil, err
		}
		if !isMap(value) {
			return nil, fmt.Errorf("merge %s: root of %s must be an object/table", format, source)
		}
		if i == 0 {
			merged = value
			continue
		}
		if err := compatibleTypes(merged, value, "$"); err != nil {
			return nil, fmt.Errorf("merge %s source %s: %w", format, source, err)
		}
		merged = overlay(merged, value)
	}
	return merged, nil
}

func parseStructured(format, path string) (any, error) {
	switch format {
	case "json":
		return parseJSON(path)
	case "yaml":
		return parseYAML(path)
	case "toml":
		return parseTOML(path)
	default:
		return nil, fmt.Errorf("unsupported structured format %q", format)
	}
}

func compatibleTypes(left, right any, path string) error {
	left = normalizeMaps(left)
	right = normalizeMaps(right)
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
	left = normalizeMaps(left)
	right = normalizeMaps(right)
	if !isMap(left) || !isMap(right) {
		return right
	}
	out := map[string]any{}
	for key, value := range asMap(left) {
		out[key] = normalizeMaps(value)
	}
	for key, rightValue := range asMap(right) {
		if leftValue, ok := out[key]; ok && isMap(leftValue) && isMap(rightValue) {
			out[key] = overlay(leftValue, rightValue)
			continue
		}
		out[key] = normalizeMaps(rightValue)
	}
	return out
}

func normalizeMaps(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, value := range typed {
			out[key] = normalizeMaps(value)
		}
		return out
	case map[interface{}]interface{}:
		out := map[string]any{}
		for key, value := range typed {
			out[fmt.Sprint(key)] = normalizeMaps(value)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, value := range typed {
			out[i] = normalizeMaps(value)
		}
		return out
	default:
		return value
	}
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
