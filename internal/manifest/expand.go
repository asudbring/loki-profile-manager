package manifest

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/asudbring/loki-profile-manager/internal/config"
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

func (e Expander) ValidateTargetPath(target string) error {
	resolver := e.Resolver.WithDefaults()
	target = config.CleanForOS(resolver.GOOS, strings.TrimSpace(target))
	if target == "" {
		return fmt.Errorf("target path is required")
	}
	if !isAbsForOS(resolver.GOOS, target) {
		return fmt.Errorf("target path %q must be absolute or use ~/${HOME}", target)
	}
	if isRootForOS(resolver.GOOS, target) {
		return fmt.Errorf("target path %q must not be a filesystem root", target)
	}
	allowedRoots := []string{}
	home := config.CleanForOS(resolver.GOOS, resolver.HomeDir)
	if home != "" {
		if isRootForOS(resolver.GOOS, home) {
			return fmt.Errorf("target policy home root %q is not allowed", home)
		}
		if samePathForOS(resolver.GOOS, target, home) {
			return fmt.Errorf("target path %q must not be the home directory itself", target)
		}
		allowedRoots = append(allowedRoots, home)
	}
	documents := config.CleanForOS(resolver.GOOS, resolver.DocumentsDir)
	if documents != "" && !samePathForOS(resolver.GOOS, documents, home) {
		if isRootForOS(resolver.GOOS, documents) {
			return fmt.Errorf("target policy documents root %q is not allowed", documents)
		}
		if samePathForOS(resolver.GOOS, target, documents) {
			return fmt.Errorf("target path %q must not be the documents directory itself", target)
		}
		allowedRoots = append(allowedRoots, documents)
	}
	if len(allowedRoots) == 0 {
		return fmt.Errorf("target path %q cannot be validated without a home or documents directory", target)
	}
	for _, root := range allowedRoots {
		if pathWithinRootForOS(resolver.GOOS, target, root) {
			return nil
		}
	}
	return fmt.Errorf("target path %q is outside allowed roots %q", target, allowedRoots)
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
		if (name == "DOCUMENTS" || name == "DOCUMENTS_DIR" || name == "USER_DOCUMENTS") && resolver.DocumentsDir != "" {
			return resolver.DocumentsDir, nil
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

func ValidateSourceWithinRoot(layerRoot, sourcePath string) error {
	root, err := filepath.Abs(layerRoot)
	if err != nil {
		return fmt.Errorf("resolve layer root %s: %w", layerRoot, err)
	}
	full, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("resolve source %s: %w", sourcePath, err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve real layer root %s: %w", layerRoot, err)
	}
	realSource, err := filepath.EvalSymlinks(full)
	if err != nil {
		return fmt.Errorf("resolve real source %s: %w", sourcePath, err)
	}
	rel, err := filepath.Rel(realRoot, realSource)
	if err != nil {
		return fmt.Errorf("compare source %s to layer root %s: %w", sourcePath, layerRoot, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("source path %q resolves outside layer root", sourcePath)
	}
	return nil
}

func isAbsAnyOS(path string) bool {
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`) {
		return true
	}
	return len(path) >= 2 && path[1] == ':'
}

func isAbsForOS(goos, value string) bool {
	if goos == "windows" {
		value = strings.ReplaceAll(value, "/", `\`)
		if strings.HasPrefix(value, `\`) {
			return true
		}
		return len(value) >= 3 && value[1] == ':' && (value[2] == '\\' || value[2] == '/')
	}
	return strings.HasPrefix(value, "/") || len(value) >= 3 && value[1] == ':' && (value[2] == '/' || value[2] == '\\')
}

func isRootForOS(goos, value string) bool {
	value = config.CleanForOS(goos, value)
	if goos == "windows" {
		value = strings.ReplaceAll(value, "/", `\`)
		return value == `\` || len(value) == 2 && value[1] == ':'
	}
	return value == "/"
}

func samePathForOS(goos, left, right string) bool {
	left = normalizeForCompare(goos, left)
	right = normalizeForCompare(goos, right)
	return left == right
}

func pathWithinRootForOS(goos, child, root string) bool {
	child = normalizeForCompare(goos, child)
	root = normalizeForCompare(goos, root)
	sep := "/"
	if goos == "windows" {
		sep = `\`
	}
	root = strings.TrimRight(root, sep)
	child = strings.TrimRight(child, sep)
	return strings.HasPrefix(child, root+sep)
}

func normalizeForCompare(goos, value string) string {
	value = config.CleanForOS(goos, value)
	if goos == "windows" {
		value = strings.ToLower(strings.ReplaceAll(value, "/", `\`))
	}
	return value
}
