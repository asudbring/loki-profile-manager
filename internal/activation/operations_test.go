package activation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"gopkg.in/yaml.v3"
)

type fakeSecrets struct {
	values map[string]string
}

func (f fakeSecrets) GetSecrets(ctx context.Context, names []string) (map[string]string, error) {
	out := map[string]string{}
	var missing []string
	for _, name := range names {
		value, ok := f.values[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		out[name] = value
	}
	if len(missing) > 0 {
		return nil, fakeMissing{names: missing}
	}
	return out, nil
}

type fakeMissing struct{ names []string }

func (e fakeMissing) Error() string { return "missing: " + strings.Join(e.names, ",") }

func TestCopyPathCopiesFilesAndDirectories(t *testing.T) {
	root := t.TempDir()
	sourceFile := filepath.Join(root, "source.txt")
	writeFile(t, sourceFile, "hello")
	targetFile := filepath.Join(root, "out", "target.txt")
	if err := CopyPath(sourceFile, targetFile); err != nil {
		t.Fatalf("CopyPath(file) error = %v", err)
	}
	if got := readFile(t, targetFile); got != "hello" {
		t.Fatalf("target file = %q", got)
	}

	sourceDir := filepath.Join(root, "dir")
	writeFile(t, filepath.Join(sourceDir, "nested", "a.txt"), "a")
	writeFile(t, filepath.Join(sourceDir, "b.txt"), "b")
	targetDir := filepath.Join(root, "copied")
	if err := CopyPath(sourceDir, targetDir); err != nil {
		t.Fatalf("CopyPath(dir) error = %v", err)
	}
	if got := readFile(t, filepath.Join(targetDir, "nested", "a.txt")); got != "a" {
		t.Fatalf("nested file = %q", got)
	}
}

func TestApplySymlinkCreatesLink(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	target := filepath.Join(root, "target.txt")
	writeFile(t, source, "hello")
	if err := ApplySymlink(source, target); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	link, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if link != source {
		t.Fatalf("link = %q, want %q", link, source)
	}
}

func TestMergeWritersDeepMergeAndReplaceArrays(t *testing.T) {
	root := t.TempDir()
	jsonA := filepath.Join(root, "a.json")
	jsonB := filepath.Join(root, "b.json")
	writeFile(t, jsonA, `{"editor":{"fontSize":12},"list":[1]}`)
	writeFile(t, jsonB, `{"editor":{"lineNumbers":"on"},"list":[2]}`)
	mergedJSON, err := MergeJSONBytes([]string{jsonA, jsonB})
	if err != nil {
		t.Fatalf("MergeJSONBytes() error = %v", err)
	}
	var gotJSON map[string]any
	if err := json.Unmarshal(mergedJSON, &gotJSON); err != nil {
		t.Fatalf("merged JSON invalid: %v\n%s", err, mergedJSON)
	}
	if gotJSON["list"].([]any)[0].(float64) != 2 || gotJSON["editor"].(map[string]any)["fontSize"].(float64) != 12 {
		t.Fatalf("merged JSON = %#v", gotJSON)
	}

	yamlA := filepath.Join(root, "a.yaml")
	yamlB := filepath.Join(root, "b.yaml")
	writeFile(t, yamlA, "editor:\n  fontSize: 12\nlist:\n  - 1\n")
	writeFile(t, yamlB, "editor:\n  lineNumbers: on\nlist:\n  - 2\n")
	mergedYAML, err := MergeYAMLBytes([]string{yamlA, yamlB})
	if err != nil {
		t.Fatalf("MergeYAMLBytes() error = %v", err)
	}
	var gotYAML map[string]any
	if err := yaml.Unmarshal(mergedYAML, &gotYAML); err != nil {
		t.Fatalf("merged YAML invalid: %v\n%s", err, mergedYAML)
	}
	if gotYAML["editor"].(map[string]any)["fontSize"].(int) != 12 {
		t.Fatalf("merged YAML = %#v", gotYAML)
	}

	tomlA := filepath.Join(root, "a.toml")
	tomlB := filepath.Join(root, "b.toml")
	writeFile(t, tomlA, "list = [1]\n[editor]\nfontSize = 12\n")
	writeFile(t, tomlB, "list = [2]\n[editor]\nlineNumbers = \"on\"\n")
	mergedTOML, err := MergeTOMLBytes([]string{tomlA, tomlB})
	if err != nil {
		t.Fatalf("MergeTOMLBytes() error = %v", err)
	}
	var gotTOML map[string]any
	if err := toml.Unmarshal(mergedTOML, &gotTOML); err != nil {
		t.Fatalf("merged TOML invalid: %v\n%s", err, mergedTOML)
	}
	if gotTOML["editor"].(map[string]any)["fontSize"].(int64) != 12 {
		t.Fatalf("merged TOML = %#v", gotTOML)
	}
}

func TestMergeTypeConflictFails(t *testing.T) {
	root := t.TempDir()
	left := filepath.Join(root, "left.json")
	right := filepath.Join(root, "right.json")
	writeFile(t, left, `{"editor":{"fontSize":12}}`)
	writeFile(t, right, `{"editor":"bad"}`)
	if _, err := MergeJSONBytes([]string{left, right}); err == nil {
		t.Fatal("MergeJSONBytes() error = nil, want type conflict")
	}
}

func TestRenderToFileUsesSecretProvider(t *testing.T) {
	root := t.TempDir()
	template := filepath.Join(root, "config.tmpl")
	target := filepath.Join(root, "config.txt")
	writeFile(t, template, "token={{ TOKEN }}\nproject=${PROJECT}\n")
	provider := fakeSecrets{values: map[string]string{"TOKEN": "secret-token", "PROJECT": "demo"}}
	if err := RenderToFile(context.Background(), provider, template, target, []string{"TOKEN"}); err != nil {
		t.Fatalf("RenderToFile() error = %v", err)
	}
	if got := readFile(t, target); got != "token=secret-token\nproject=demo\n" {
		t.Fatalf("rendered = %q", got)
	}
	if err := RenderToFile(context.Background(), fakeSecrets{values: map[string]string{}}, template, filepath.Join(root, "bad"), []string{"TOKEN"}); err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("missing secret error = %v", err)
	}
}

func TestExecuteDryRunAndRollbackOnFailure(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	target := filepath.Join(root, "target.txt")
	writeFile(t, source, "new")
	plan := Plan{Profile: "work", Operations: []Operation{{ID: "copy", Type: OperationCopy, SourcePath: source, TargetPath: target, LayerName: "work", LayerKind: "core"}}}
	result, err := Execute(ctx, ExecuteRequest{Database: database, LocalPaths: config.LocalPaths{SnapshotDir: filepath.Join(root, "snapshots")}, Plan: plan, DryRun: true})
	if err != nil {
		t.Fatalf("Execute(dry-run) error = %v", err)
	}
	if result.Changed != 0 {
		t.Fatalf("dry-run changed = %d", result.Changed)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("dry-run target exists or stat err = %v", err)
	}

	writeFile(t, target, "old")
	oldHash, err := HashPath(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertManagedTarget(ctx, database, plan.Operations[0], oldHash, time.Now()); err != nil {
		t.Fatalf("UpsertManagedTarget() error = %v", err)
	}
	_, err = Execute(ctx, ExecuteRequest{Database: database, LocalPaths: config.LocalPaths{SnapshotDir: filepath.Join(root, "snapshots")}, Plan: plan, PreviousActiveProfile: "dev", PreviousActiveBuckets: []string{"old"}, FailAfter: 1})
	if err == nil || !strings.Contains(err.Error(), "rollback completed") {
		t.Fatalf("Execute(fail) error = %v", err)
	}
	if got := readFile(t, target); got != "old" {
		t.Fatalf("rollback target = %q", got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(content)
}
