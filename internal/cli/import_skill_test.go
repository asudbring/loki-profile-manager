package cli

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/store"
)

func TestImportSkillDryRunCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliImportSkillStore(t)
	source := cliSkillFolder(t, filepath.Join(t.TempDir(), "sample-skill"), "sample-skill", "v1")

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "import-skill", source, "--common", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("import-skill dry-run error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Loki skill import dry-run") || !strings.Contains(got, "skills/sample-skill") || !strings.Contains(got, "Would copy: true") {
		t.Fatalf("import-skill output = %s", got)
	}
	if _, err := os.Stat(filepath.Join(storePath, "profiles", "common", "skills", "sample-skill")); !os.IsNotExist(err) {
		t.Fatalf("dry-run destination exists or stat err = %v", err)
	}
}

func TestImportSkillYesJSON(t *testing.T) {
	home := t.TempDir()
	storePath := cliImportSkillStore(t)
	source := cliSkillFolder(t, filepath.Join(t.TempDir(), "sample-skill"), "sample-skill", "v1")

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "import-skill", source, "--profile", "work", "--name", "team-skill", "--yes", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("import-skill yes error = %v", err)
	}
	var result app.ImportSkillResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("import-skill JSON invalid: %v\n%s", err, out.String())
	}
	if result.Changed != 1 || result.ManifestSource != "skills/team-skill" || result.Layer.Kind != "core" {
		t.Fatalf("result = %+v", result)
	}
	if got := string(mustRead(t, filepath.Join(storePath, "profiles", "work", "core", "skills", "team-skill", "notes.md"))); got != "v1" {
		t.Fatalf("imported notes = %q", got)
	}
}

func TestImportSkillZipDryRunCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliImportSkillStore(t)
	source := cliSkillZip(t, filepath.Join(t.TempDir(), "sample-skill.zip"), map[string]string{
		"sample-skill/SKILL.md": "---\nname: sample-skill\ndescription: Test skill\n---\n# sample-skill\n",
		"sample-skill/notes.md": "v1",
	})

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "import-skill", source, "--common", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("import-skill zip dry-run error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Loki skill import dry-run") || !strings.Contains(got, "Source kind: zip") || !strings.Contains(got, "skills/sample-skill") || !strings.Contains(got, "Would copy: true") {
		t.Fatalf("import-skill zip output = %s", got)
	}
}

func TestImportSkillZipYesJSON(t *testing.T) {
	home := t.TempDir()
	storePath := cliImportSkillStore(t)
	source := cliSkillZip(t, filepath.Join(t.TempDir(), "sample-skill.zip"), map[string]string{
		"sample-skill/SKILL.md": "---\nname: sample-skill\ndescription: Test skill\n---\n# sample-skill\n",
		"sample-skill/notes.md": "v1",
	})

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "import-skill", source, "--profile", "work", "--name", "team-skill", "--yes", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("import-skill zip yes error = %v", err)
	}
	var result app.ImportSkillResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("import-skill zip JSON invalid: %v\n%s", err, out.String())
	}
	if result.SourceKind != "zip" || result.Changed != 1 || result.ManifestSource != "skills/team-skill" || result.Layer.Kind != "core" {
		t.Fatalf("zip result = %+v", result)
	}
	if got := string(mustRead(t, filepath.Join(storePath, "profiles", "work", "core", "skills", "team-skill", "notes.md"))); got != "v1" {
		t.Fatalf("imported zip notes = %q", got)
	}
}

func TestImportSkillRequiresModeAndDestination(t *testing.T) {
	storePath := cliImportSkillStore(t)
	source := cliSkillFolder(t, filepath.Join(t.TempDir(), "sample-skill"), "sample-skill", "v1")

	cmd, _, _ := testCommand(t)
	cmd.SetArgs([]string{"--store", storePath, "import-skill", source, "--common"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("import-skill no-mode error = %v", err)
	}

	cmd, _, _ = testCommand(t)
	cmd.SetArgs([]string{"--store", storePath, "import-skill", source, "--common", "--profile", "work", "--dry-run"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--common cannot be combined") {
		t.Fatalf("import-skill bad-destination error = %v", err)
	}
}

func cliImportSkillStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	return root
}

func cliSkillFolder(t *testing.T, dir, name, notes string) string {
	t.Helper()
	cliWrite(t, filepath.Join(dir, "SKILL.md"), "---\nname: "+name+"\ndescription: Test skill\n---\n# "+name+"\n")
	cliWrite(t, filepath.Join(dir, "notes.md"), notes)
	return dir
}

func cliSkillZip(t *testing.T, zipPath string, entries map[string]string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(zip parent) error = %v", err)
	}
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Create(zip) error = %v", err)
	}
	writer := zip.NewWriter(file)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("Create(%s) error = %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%s) error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip Close() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file Close() error = %v", err)
	}
	return zipPath
}
