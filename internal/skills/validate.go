package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

type Result struct {
	Valid  bool    `json:"valid"`
	Name   string  `json:"name,omitempty"`
	Issues []Issue `json:"issues"`
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

var (
	inlineLinkRE = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)
	refLinkRE    = regexp.MustCompile(`(?m)^\s*\[[^\]]+\]:\s+(\S+)`)
)

func ValidateFolder(dir string) Result {
	result := Result{Valid: true, Issues: []Issue{}}
	skillPath := filepath.Join(dir, "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		return invalid("skill.missing_file", fmt.Sprintf("SKILL.md is required: %v", err), skillPath)
	}
	fm, body, err := parseFrontmatter(content)
	if err != nil {
		return invalid("skill.frontmatter_invalid", err.Error(), skillPath)
	}
	if strings.TrimSpace(fm.Name) == "" {
		result.Valid = false
		result.Issues = append(result.Issues, Issue{Code: "skill.name_missing", Message: "frontmatter name is required", Path: skillPath})
	}
	if strings.TrimSpace(fm.Description) == "" {
		result.Valid = false
		result.Issues = append(result.Issues, Issue{Code: "skill.description_missing", Message: "frontmatter description is required", Path: skillPath})
	}
	result.Name = fm.Name
	for _, ref := range references(body) {
		cleaned := cleanReferenceTarget(ref)
		if skipReference(cleaned) {
			continue
		}
		if isLocalAbsoluteReference(cleaned) {
			result.Valid = false
			result.Issues = append(result.Issues, Issue{Code: "skill.reference_absolute", Message: fmt.Sprintf("absolute local reference %q is not allowed", cleaned), Path: skillPath})
			continue
		}
		cleaned = strings.TrimSpace(strings.Split(strings.Split(cleaned, "#")[0], "?")[0])
		if cleaned == "" {
			continue
		}
		resolved := filepath.Clean(filepath.Join(dir, filepath.FromSlash(cleaned)))
		rel, err := filepath.Rel(dir, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			result.Valid = false
			result.Issues = append(result.Issues, Issue{Code: "skill.reference_invalid", Message: fmt.Sprintf("relative reference %q escapes skill folder", ref), Path: skillPath})
			continue
		}
		if _, err := os.Stat(resolved); err != nil {
			result.Valid = false
			result.Issues = append(result.Issues, Issue{Code: "skill.reference_missing", Message: fmt.Sprintf("relative reference %q does not exist", ref), Path: resolved})
		}
	}
	return result
}

func invalid(code, message, path string) Result {
	return Result{Valid: false, Issues: []Issue{{Code: code, Message: message, Path: path}}}
}

func parseFrontmatter(content []byte) (frontmatter, string, error) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return frontmatter{}, "", fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return frontmatter{}, "", fmt.Errorf("SKILL.md frontmatter closing delimiter missing")
	}
	front := text[4 : 4+end]
	body := text[4+end+5:]
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		return frontmatter{}, "", fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	return fm, body, nil
}

func references(body string) []string {
	var refs []string
	for _, match := range inlineLinkRE.FindAllStringSubmatch(body, -1) {
		refs = append(refs, match[1])
	}
	for _, match := range refLinkRE.FindAllStringSubmatch(body, -1) {
		refs = append(refs, match[1])
	}
	return refs
}

func cleanReferenceTarget(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "<") {
		if end := strings.Index(ref, ">"); end >= 0 {
			return strings.TrimSpace(ref[1:end])
		}
	}
	fields := strings.Fields(ref)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], "<>")
}

func skipReference(ref string) bool {
	ref = strings.TrimSpace(ref)
	lower := strings.ToLower(ref)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(ref, "#")
}

func isLocalAbsoluteReference(ref string) bool {
	ref = strings.TrimSpace(ref)
	return filepath.IsAbs(ref) || strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, `\`) || len(ref) >= 2 && ref[1] == ':'
}
