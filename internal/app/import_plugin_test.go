package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/manifest"
)

func TestImportPluginRequiresRuntime(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	source := writePluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill"), "slash-goal-skill")

	_, err := svc.ImportPlugin(ctx, ImportPluginRequest{StorePath: storePath, SourceFolder: source, Profile: "dev", DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "at least one --runtime is required") {
		t.Fatalf("ImportPlugin() error = %v, want runtime requirement", err)
	}
}

func TestImportPluginDryRunDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	source := writePluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill"), "slash-goal-skill")

	result, err := svc.ImportPlugin(ctx, ImportPluginRequest{StorePath: storePath, SourceFolder: source, Profile: "dev", Runtimes: []string{"pi"}, DryRun: true})
	if err != nil {
		t.Fatalf("ImportPlugin(dry-run) error = %v", err)
	}
	if !result.DryRun || !result.WouldCopy || result.Changed != 0 || result.Name != "slash-goal-skill" {
		t.Fatalf("dry-run result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(storePath, "profiles", "dev", "core", "plugins", "slash-goal-skill")); !os.IsNotExist(err) {
		t.Fatalf("dry-run destination exists or stat err = %v", err)
	}
	parsed, err := manifest.ParseFile(filepath.Join(storePath, "profiles", "dev", "core", "manifest.yaml"))
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if len(parsed.Files) != 0 || len(parsed.Skills) != 0 {
		t.Fatalf("dry-run changed manifest: files=%+v skills=%+v", parsed.Files, parsed.Skills)
	}
}

func TestImportPluginPiCopiesBundleAndSettingsOverride(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc := newImportSkillTestService(t, ctx, home)
	defer svc.Close()
	storePath := importSkillStore(t)
	writeCommonPiSettings(t, storePath, []string{"npm:context-mode"})
	source := writePluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill"), "slash-goal-skill")

	result, err := svc.ImportPlugin(ctx, ImportPluginRequest{StorePath: storePath, SourceFolder: source, Profile: "dev", Runtimes: []string{"pi"}, Yes: true})
	if err != nil {
		t.Fatalf("ImportPlugin(pi) error = %v", err)
	}
	wantDest := filepath.Join(storePath, "profiles", "dev", "core", "plugins", "slash-goal-skill")
	if result.Changed != 1 || result.DestinationPath != wantDest || result.ManifestSource != "plugins/slash-goal-skill" || result.Layer.Kind != "core" {
		t.Fatalf("result = %+v, want dest %s", result, wantDest)
	}
	if got := readAppFile(t, filepath.Join(wantDest, "plugin.json")); !strings.Contains(got, "slash-goal-skill") {
		t.Fatalf("imported plugin.json = %q", got)
	}
	parsed, err := manifest.ParseFile(filepath.Join(storePath, "profiles", "dev", "core", "manifest.yaml"))
	if err != nil {
		t.Fatalf("ParseFile(dev core) error = %v", err)
	}
	if len(parsed.Skills) != 1 || parsed.Skills[0].Source != "plugins/slash-goal-skill/skills" {
		t.Fatalf("manifest skills = %+v", parsed.Skills)
	}
	assertManifestFileEntry(t, parsed.Files, "~/.pi/agent/packages/slash-goal-skill", "plugins/slash-goal-skill", manifest.ModeCopy, "")
	assertManifestFileEntry(t, parsed.Files, "~/.pi/agent/settings.json", "files/dot-pi/agent/settings.json", manifest.ModeMerge, manifest.FormatJSON)
	common, err := manifest.ParseFile(filepath.Join(storePath, "profiles", "common", "manifest.yaml"))
	if err != nil {
		t.Fatalf("ParseFile(common) error = %v", err)
	}
	assertManifestFileEntry(t, common.Files, "~/.pi/agent/settings.json", "files/dot-pi/agent/settings.json", manifest.ModeMerge, manifest.FormatJSON)
	plan, err := activation.BuildPlan(ctx, activation.PlanRequest{StorePath: storePath, Profile: "dev", Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("BuildPlan(dev) error = %v", err)
	}
	if !planHasOperation(plan, filepath.Join(home, ".pi", "agent", "settings.json"), activation.OperationMerge) {
		t.Fatalf("activation plan missing pi settings merge operation: %+v", plan.Operations)
	}

	settingsPath := filepath.Join(storePath, "profiles", "dev", "core", "files", "dot-pi", "agent", "settings.json")
	settings := readJSONMap(t, settingsPath)
	packages, ok := settings["packages"].([]any)
	if !ok {
		t.Fatalf("packages = %#v, want array", settings["packages"])
	}
	if !jsonArrayContains(packages, "npm:context-mode") || !jsonArrayContains(packages, "./packages/slash-goal-skill") {
		t.Fatalf("packages = %#v, want existing package plus imported plugin", packages)
	}
}

func TestImportPluginPiPreservesSelectedLayerPackages(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc := newImportSkillTestService(t, ctx, home)
	defer svc.Close()
	storePath := importSkillStore(t)
	writePiSettingsLayer(t, storePath, filepath.Join("profiles", "common"), "common-pi-settings", []string{"npm:common"}, manifest.ModeCopy)
	writePiSettingsLayer(t, storePath, filepath.Join("profiles", "dev", "core"), "dev-pi-settings", []string{"npm:dev"}, manifest.ModeMerge)
	source := writePluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill"), "slash-goal-skill")

	_, err := svc.ImportPlugin(ctx, ImportPluginRequest{StorePath: storePath, SourceFolder: source, Profile: "dev", Runtimes: []string{"pi"}, Yes: true})
	if err != nil {
		t.Fatalf("ImportPlugin(pi) error = %v", err)
	}
	settings := readJSONMap(t, filepath.Join(storePath, "profiles", "dev", "core", "files", "dot-pi", "agent", "settings.json"))
	packages, ok := settings["packages"].([]any)
	if !ok {
		t.Fatalf("packages = %#v, want array", settings["packages"])
	}
	if !jsonArrayContains(packages, "npm:dev") || !jsonArrayContains(packages, "./packages/slash-goal-skill") {
		t.Fatalf("packages = %#v, want selected layer package plus imported plugin", packages)
	}
	if jsonArrayContains(packages, "npm:common") {
		t.Fatalf("packages = %#v, common package should remain overridden by selected layer packages", packages)
	}
}

func TestImportPluginRejectsSelectedPiSettingsUnsupportedMode(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	writePiSettingsLayer(t, storePath, filepath.Join("profiles", "dev", "core"), "dev-pi-settings", []string{"npm:dev"}, manifest.ModeRender)
	source := writePluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill"), "slash-goal-skill")

	_, err := svc.ImportPlugin(ctx, ImportPluginRequest{StorePath: storePath, SourceFolder: source, Profile: "dev", Runtimes: []string{"pi"}, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "unsupported mode") {
		t.Fatalf("ImportPlugin(pi) error = %v, want unsupported mode", err)
	}
}

func TestImportPluginOverwriteExistingDestination(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	sourceV1 := writePluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill-v1"), "slash-goal-skill")
	if _, err := svc.ImportPlugin(ctx, ImportPluginRequest{StorePath: storePath, SourceFolder: sourceV1, Profile: "dev", Runtimes: []string{"copilot"}, Yes: true}); err != nil {
		t.Fatalf("ImportPlugin(v1) error = %v", err)
	}
	sourceV2 := writePluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill-v2"), "slash-goal-skill")
	writeAppFile(t, filepath.Join(sourceV2, "hooks", "goal-stop.mjs"), "console.log('v2')\n")

	result, err := svc.ImportPlugin(ctx, ImportPluginRequest{StorePath: storePath, SourceFolder: sourceV2, Profile: "dev", Runtimes: []string{"copilot"}, Overwrite: true, DryRun: true})
	if err != nil {
		t.Fatalf("ImportPlugin(v2 dry-run) error = %v", err)
	}
	if !result.DestinationExists || !result.WouldCopy || !result.WouldOverwrite {
		t.Fatalf("dry-run result = %+v, want overwrite plan", result)
	}
	_, err = svc.ImportPlugin(ctx, ImportPluginRequest{StorePath: storePath, SourceFolder: sourceV2, Profile: "dev", Runtimes: []string{"copilot"}, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "--overwrite") {
		t.Fatalf("ImportPlugin(v2 no overwrite) error = %v, want overwrite requirement", err)
	}
	result, err = svc.ImportPlugin(ctx, ImportPluginRequest{StorePath: storePath, SourceFolder: sourceV2, Profile: "dev", Runtimes: []string{"copilot"}, Overwrite: true, Yes: true})
	if err != nil {
		t.Fatalf("ImportPlugin(v2 overwrite) error = %v", err)
	}
	if got := readAppFile(t, filepath.Join(result.DestinationPath, "hooks", "goal-stop.mjs")); !strings.Contains(got, "v2") {
		t.Fatalf("overwritten hook = %q", got)
	}
}

func TestImportPluginRequiresExactlyOneExecutionMode(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	source := writePluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill"), "slash-goal-skill")

	for _, req := range []ImportPluginRequest{
		{StorePath: storePath, SourceFolder: source, Profile: "dev", Runtimes: []string{"copilot"}},
		{StorePath: storePath, SourceFolder: source, Profile: "dev", Runtimes: []string{"copilot"}, DryRun: true, Yes: true},
	} {
		_, err := svc.ImportPlugin(ctx, req)
		if err == nil || !strings.Contains(err.Error(), "exactly one") {
			t.Fatalf("ImportPlugin(%+v) error = %v, want exactly-one mode", req, err)
		}
	}
}

func TestImportPluginSkipsGitDirectory(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	source := writePluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill"), "slash-goal-skill")
	writeAppFile(t, filepath.Join(source, ".git", "config"), "[remote \"origin\"]\n")

	result, err := svc.ImportPlugin(ctx, ImportPluginRequest{StorePath: storePath, SourceFolder: source, Profile: "dev", Runtimes: []string{"copilot"}, Yes: true})
	if err != nil {
		t.Fatalf("ImportPlugin(copilot) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.DestinationPath, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git destination exists or stat err = %v", err)
	}
}

func TestImportPluginRuntimeAllPlansEveryRuntime(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	source := writePluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill"), "slash-goal-skill")

	result, err := svc.ImportPlugin(ctx, ImportPluginRequest{StorePath: storePath, SourceFolder: source, Profile: "dev", Runtimes: []string{"all"}, DryRun: true})
	if err != nil {
		t.Fatalf("ImportPlugin(all) error = %v", err)
	}
	got := map[string]bool{}
	for _, runtimePlan := range result.Runtimes {
		got[runtimePlan.Runtime] = true
	}
	for _, runtimeName := range []string{"pi", "copilot", "claude", "codex", "vscode"} {
		if !got[runtimeName] {
			t.Fatalf("runtime %s missing from plans %+v", runtimeName, result.Runtimes)
		}
	}
}

func TestImportPluginCopilotEmitsManualStepOnly(t *testing.T) {
	ctx := context.Background()
	svc := newImportSkillTestService(t, ctx, t.TempDir())
	defer svc.Close()
	storePath := importSkillStore(t)
	source := writePluginBundle(t, filepath.Join(t.TempDir(), "slash-goal-skill"), "slash-goal-skill")

	result, err := svc.ImportPlugin(ctx, ImportPluginRequest{StorePath: storePath, SourceFolder: source, Profile: "dev", Runtimes: []string{"copilot"}, DryRun: true})
	if err != nil {
		t.Fatalf("ImportPlugin(copilot) error = %v", err)
	}
	if len(result.Runtimes) != 1 || result.Runtimes[0].Runtime != "copilot" {
		t.Fatalf("runtime plans = %+v", result.Runtimes)
	}
	if len(result.Runtimes[0].Actions) != 0 || len(result.Runtimes[0].ManualSteps) != 1 {
		t.Fatalf("copilot plan = %+v, want manual step only", result.Runtimes[0])
	}
	if !strings.Contains(result.Runtimes[0].ManualSteps[0], "gh copilot -- plugin install") {
		t.Fatalf("manual step = %q", result.Runtimes[0].ManualSteps[0])
	}
}

func writePluginBundle(t *testing.T, dir, name string) string {
	t.Helper()
	writeAppFile(t, filepath.Join(dir, "plugin.json"), `{"name":"`+name+`","description":"Test plugin","version":"0.1.0","skills":"skills/","commands":"commands/","hooks":"hooks.json"}`)
	writeAppFile(t, filepath.Join(dir, "package.json"), `{"name":"`+name+`","version":"0.1.0","description":"Test plugin","pi":{"extensions":["extensions/pi-goal.ts"],"skills":["skills"]}}`)
	writeAppFile(t, filepath.Join(dir, "commands", "goal.toml"), "description = \"Set a goal\"\n")
	writeAppFile(t, filepath.Join(dir, "extensions", "pi-goal.ts"), "export default function () {}\n")
	writeAppFile(t, filepath.Join(dir, "hooks.json"), `{"version":1,"hooks":{}}`)
	writeAppFile(t, filepath.Join(dir, "hooks", "goal-stop.mjs"), "console.log('ok')\n")
	writeAppFile(t, filepath.Join(dir, "skills", "goal-keeper", "SKILL.md"), "---\nname: goal-keeper\ndescription: Test goal skill\n---\n# goal-keeper\n")
	return dir
}

func writeCommonPiSettings(t *testing.T, storePath string, packages []string) {
	t.Helper()
	writePiSettingsLayer(t, storePath, filepath.Join("profiles", "common"), "common-pi-settings", packages, manifest.ModeCopy)
}

func writePiSettingsLayer(t *testing.T, storePath, layerRel, id string, packages []string, mode string) {
	t.Helper()
	settingsPath := filepath.Join(storePath, layerRel, "files", "dot-pi", "agent", "settings.json")
	content, err := json.MarshalIndent(map[string]any{"packages": packages}, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	writeAppFile(t, settingsPath, string(append(content, '\n')))
	manifestPath := filepath.Join(storePath, layerRel, "manifest.yaml")
	parsed, err := manifest.ParseFile(manifestPath)
	if err != nil {
		t.Fatalf("ParseFile(%s) error = %v", manifestPath, err)
	}
	parsed.Files = append(parsed.Files, manifest.FileEntry{ID: id, Source: "files/dot-pi/agent/settings.json", Target: "~/.pi/agent/settings.json", Mode: mode, Format: manifest.FormatJSON})
	if err := manifest.WriteFile(manifestPath, parsed); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", manifestPath, err)
	}
}

func assertManifestFileEntry(t *testing.T, entries []manifest.FileEntry, target, source, mode, format string) {
	t.Helper()
	for _, entry := range entries {
		if entry.Target == target && entry.Source == source && entry.Mode == mode && entry.Format == format {
			return
		}
	}
	t.Fatalf("manifest files = %+v, missing target=%s source=%s mode=%s format=%s", entries, target, source, mode, format)
}

func readJSONMap(t *testing.T, filePath string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(readAppFile(t, filePath)), &out); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", filePath, err)
	}
	return out
}

func jsonArrayContains(values []any, want string) bool {
	for _, value := range values {
		if got, ok := value.(string); ok && got == want {
			return true
		}
	}
	return false
}

func planHasOperation(plan activation.Plan, target string, opType activation.OperationType) bool {
	for _, op := range plan.Operations {
		if op.TargetPath == target && op.Type == opType {
			return true
		}
	}
	return false
}
