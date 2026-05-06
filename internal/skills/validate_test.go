package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFolderValid(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "---\nname: test-skill\ndescription: Does test things\n---\nSee [more](docs/more.md).\n")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "more.md"), []byte("more"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ValidateFolder(dir)
	if !result.Valid || result.Name != "test-skill" {
		t.Fatalf("result = %+v", result)
	}
}

func TestValidateFolderMissingSkillMD(t *testing.T) {
	result := ValidateFolder(t.TempDir())
	if result.Valid || result.Issues[0].Code != "skill.missing_file" {
		t.Fatalf("result = %+v", result)
	}
}

func TestValidateFolderMissingFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "# No frontmatter\n")
	result := ValidateFolder(dir)
	if result.Valid || result.Issues[0].Code != "skill.frontmatter_invalid" {
		t.Fatalf("result = %+v", result)
	}
}

func TestValidateFolderMissingNameDescription(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "---\nname: \n---\nbody\n")
	result := ValidateFolder(dir)
	if result.Valid || len(result.Issues) != 2 {
		t.Fatalf("result = %+v", result)
	}
}

func TestValidateFolderAllowsMarkdownReferenceTitle(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "---\nname: test\ndescription: desc\n---\nSee ![diagram](images/diagram.png \"Description\").\n")
	if err := os.MkdirAll(filepath.Join(dir, "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "images", "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ValidateFolder(dir)
	if !result.Valid {
		t.Fatalf("result = %+v", result)
	}
}

func TestValidateFolderBrokenReferenceIsWarning(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "---\nname: test\ndescription: desc\n---\nSee [missing](docs/missing.md), [web](https://example.com), and [anchor](#x).\n")
	result := ValidateFolder(dir)
	if !result.Valid || result.Issues[0].Code != "skill.reference_missing" {
		t.Fatalf("result = %+v", result)
	}
}

func TestValidateFolderRejectsAbsoluteLocalReference(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "---\nname: test\ndescription: desc\n---\nSee [secret](/etc/passwd), [win](C:\\Users\\alice\\secret.txt), and [web](https://example.com).\n")
	result := ValidateFolder(dir)
	if result.Valid || !hasSkillIssue(result.Issues, "skill.reference_absolute") {
		t.Fatalf("result = %+v", result)
	}
}

func hasSkillIssue(issues []Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func writeSkill(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
