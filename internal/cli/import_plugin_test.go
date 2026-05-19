package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asudbring/loki-profile-manager/internal/app"
)

func TestImportPluginDryRunCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliImportSkillStore(t)
	source := cliPluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill"), "slash-goal-skill")

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "import-plugin", source, "--profile", "dev", "--runtime", "pi", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("import-plugin dry-run error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"Loki plugin import dry-run", "plugins/slash-goal-skill", "Runtime plans:", "~/.pi/agent/packages/slash-goal-skill", "Would copy: true"} {
		if !strings.Contains(got, want) {
			t.Fatalf("import-plugin output missing %q:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(storePath, "profiles", "dev", "core", "plugins", "slash-goal-skill")); !os.IsNotExist(err) {
		t.Fatalf("dry-run destination exists or stat err = %v", err)
	}
}

func TestImportPluginYesJSON(t *testing.T) {
	home := t.TempDir()
	storePath := cliImportSkillStore(t)
	source := cliPluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill"), "slash-goal-skill")

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "import-plugin", source, "--profile", "work", "--runtime", "pi", "--yes", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("import-plugin yes error = %v", err)
	}
	var result app.ImportPluginResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("import-plugin JSON invalid: %v\n%s", err, out.String())
	}
	if result.Changed != 1 || result.ManifestSource != "plugins/slash-goal-skill" || result.Layer.Kind != "core" || len(result.Runtimes) != 1 || result.Runtimes[0].Runtime != "pi" {
		t.Fatalf("result = %+v", result)
	}
	if got := string(mustRead(t, filepath.Join(storePath, "profiles", "work", "core", "plugins", "slash-goal-skill", "plugin.json"))); !strings.Contains(got, "slash-goal-skill") {
		t.Fatalf("imported plugin.json = %q", got)
	}
}

func TestImportPluginRequiresRuntimeAndMode(t *testing.T) {
	home := t.TempDir()
	storePath := cliImportSkillStore(t)
	source := cliPluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill"), "slash-goal-skill")

	cmd, _, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "import-plugin", source, "--profile", "dev", "--dry-run"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "at least one --runtime is required") {
		t.Fatalf("import-plugin missing runtime error = %v", err)
	}

	cmd, _, _ = switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "import-plugin", source, "--runtime", "pi", "--dry-run"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "choose --common or --profile") {
		t.Fatalf("import-plugin missing mode error = %v", err)
	}

	cmd, _, _ = switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "import-plugin", source, "--profile", "dev", "--runtime", "pi"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("import-plugin missing execution mode error = %v", err)
	}

	cmd, _, _ = switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "import-plugin", source, "--profile", "dev", "--runtime", "pi", "--dry-run", "--yes"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("import-plugin conflicting execution mode error = %v", err)
	}
}

func TestImportPluginAllRuntimeCLIShowsWarningsAndCopilotStep(t *testing.T) {
	home := t.TempDir()
	storePath := cliImportSkillStore(t)
	source := cliPluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill"), "slash-goal-skill")

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "import-plugin", source, "--profile", "dev", "--runtime", "all", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("import-plugin all dry-run error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"- pi", "- copilot", "gh copilot -- plugin install", "- claude", "- codex", "- vscode", "recognized but has no safe file actions"} {
		if !strings.Contains(got, want) {
			t.Fatalf("import-plugin all output missing %q:\n%s", want, got)
		}
	}
}

func cliPluginBundle(t *testing.T, dir, name string) string {
	t.Helper()
	cliWrite(t, filepath.Join(dir, "plugin.json"), `{"name":"`+name+`","description":"Test plugin","version":"0.1.0","skills":"skills/","commands":"commands/","hooks":"hooks.json"}`)
	cliWrite(t, filepath.Join(dir, "package.json"), `{"name":"`+name+`","version":"0.1.0","description":"Test plugin","pi":{"extensions":["extensions/pi-goal.ts"],"skills":["skills"]}}`)
	cliWrite(t, filepath.Join(dir, "commands", "goal.toml"), "description = \"Set a goal\"\n")
	cliWrite(t, filepath.Join(dir, "extensions", "pi-goal.ts"), "export default function () {}\n")
	cliWrite(t, filepath.Join(dir, "hooks.json"), `{"version":1,"hooks":{}}`)
	cliWrite(t, filepath.Join(dir, "hooks", "goal-stop.mjs"), "console.log('ok')\n")
	cliWrite(t, filepath.Join(dir, "skills", "goal-keeper", "SKILL.md"), "---\nname: goal-keeper\ndescription: Test goal skill\n---\n# goal-keeper\n")
	return dir
}
