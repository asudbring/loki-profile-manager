package manifest

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/allensu/loki-profile-manager/internal/config"
)

type Expander struct {
	Resolver config.PathResolver
	Targets  map[string]TargetValue
}

var (
	bracedVarRE  = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	dollarVarRE  = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
	percentVarRE = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_]*)%`)
)

func (e Expander) ExpandTarget(value string) (string, error) {
	resolver := e.Resolver.WithDefaults()
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("expand target: target path is required")
	}
	expanded, err := e.expand(value, map[string]bool{}, 0)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(expanded, "~") {
		if resolver.HomeDir == "" {
			return "", fmt.Errorf("expand target %q: home directory is required for ~", value)
		}
		if expanded == "~" {
			expanded = resolver.HomeDir
		} else if strings.HasPrefix(expanded, "~/") || strings.HasPrefix(expanded, `~\`) {
			expanded = config.JoinForOS(resolver.GOOS, resolver.HomeDir, expanded[2:])
		}
	}
	return config.CleanForOS(resolver.GOOS, expanded), nil
}

func (e Expander) expand(value string, seen map[string]bool, depth int) (string, error) {
	if depth > 20 {
		return "", fmt.Errorf("expand target %q: variable expansion depth exceeded", value)
	}
	resolver := e.Resolver.WithDefaults()
	expandOne := func(name string) (string, error) {
		if target, ok := e.Targets[name]; ok {
			if seen[name] {
				return "", fmt.Errorf("expand target %q: target variable cycle at %s", value, name)
			}
			raw, ok := target.ForOS(resolver.GOOS)
			if !ok {
				return "", fmt.Errorf("expand target %q: target variable %s has no value for %s", value, name, resolver.GOOS)
			}
			seen[name] = true
			expanded, err := e.expand(raw, seen, depth+1)
			delete(seen, name)
			return expanded, err
		}
		if name == "HOME" && resolver.HomeDir != "" {
			return resolver.HomeDir, nil
		}
		if name == "USERPROFILE" && resolver.HomeDir != "" {
			return resolver.HomeDir, nil
		}
		if name == "LOCALAPPDATA" && resolver.LocalAppData != "" {
			return resolver.LocalAppData, nil
		}
		if resolver.Env != nil {
			if got := resolver.Env(name); got != "" {
				return got, nil
			}
		}
		return "", fmt.Errorf("expand target %q: unknown variable %s", value, name)
	}

	var firstErr error
	replace := func(re *regexp.Regexp, input string) string {
		return re.ReplaceAllStringFunc(input, func(match string) string {
			if firstErr != nil {
				return match
			}
			groups := re.FindStringSubmatch(match)
			if len(groups) != 2 {
				return match
			}
			replacement, err := expandOne(groups[1])
			if err != nil {
				firstErr = err
				return match
			}
			return replacement
		})
	}
	value = replace(bracedVarRE, value)
	value = replace(percentVarRE, value)
	value = replace(dollarVarRE, value)
	if firstErr != nil {
		return "", firstErr
	}
	return value, nil
}

func ResolveSource(layerRoot, source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("source path is required")
	}
	if isAbsAnyOS(source) {
		return "", fmt.Errorf("source path %q must be relative", source)
	}
	normalized := strings.ReplaceAll(source, `\`, "/")
	root, err := filepath.Abs(layerRoot)
	if err != nil {
		return "", fmt.Errorf("resolve layer root %s: %w", layerRoot, err)
	}
	full := filepath.Clean(filepath.Join(root, filepath.FromSlash(normalized)))
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return "", fmt.Errorf("resolve source %s: %w", source, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source path %q escapes layer root", source)
	}
	return full, nil
}

func isAbsAnyOS(path string) bool {
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`) {
		return true
	}
	return len(path) >= 2 && path[1] == ':'
}
