package infisical

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var placeholderRE = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}|\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func RequiredSecrets(template []byte, declared []string) []string {
	seen := map[string]bool{}
	for _, name := range declared {
		name = strings.TrimSpace(name)
		if name != "" {
			seen[name] = true
		}
	}
	for _, match := range placeholderRE.FindAllSubmatch(template, -1) {
		for _, group := range match[1:] {
			if len(group) > 0 {
				seen[string(group)] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func RenderTemplate(template []byte, secrets map[string]string, required []string) ([]byte, error) {
	required = RequiredSecrets(template, required)
	if err := ValidateSecretNames(required); err != nil {
		return nil, err
	}
	missing := missingSecrets(secrets, required)
	if len(missing) > 0 {
		return nil, MissingSecretError{Names: missing}
	}
	result := placeholderRE.ReplaceAllStringFunc(string(template), func(match string) string {
		groups := placeholderRE.FindStringSubmatch(match)
		name := ""
		if len(groups) > 1 && groups[1] != "" {
			name = groups[1]
		} else if len(groups) > 2 {
			name = groups[2]
		}
		return secrets[name]
	})
	return []byte(result), nil
}

func missingSecrets(secrets map[string]string, required []string) []string {
	var missing []string
	for _, name := range required {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := secrets[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func RedactedSecretSummary(names []string) string {
	cleaned, err := normalizeNames(names)
	if err != nil {
		return fmt.Sprintf("%d secret(s)", len(names))
	}
	return strings.Join(cleaned, ", ")
}
