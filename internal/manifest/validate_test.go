package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/config"
)

func TestValidateLayerValid(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "files", "a.json"), []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Manifest{Name: "test", Files: []FileEntry{{ID: "a", Source: "files/a.json", Target: "~/a.json", Mode: ModeMerge, Format: FormatJSON}}, Ignore: []string{"**/Cache/**"}}
	result := ValidateLayer(ValidationInput{LayerName: "test", LayerRoot: root, Manifest: m, Expander: testExpander()})
	if len(result.Problems) != 0 || len(result.Operations) != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestValidateLayerMissingSource(t *testing.T) {
	m := Manifest{Name: "test", Files: []FileEntry{{ID: "a", Source: "files/missing", Target: "~/a", Mode: ModeCopy}}}
	result := ValidateLayer(ValidationInput{LayerName: "test", LayerRoot: t.TempDir(), Manifest: m, Expander: testExpander()})
	if !hasProblem(result.Problems, "manifest.source_missing") {
		t.Fatalf("missing source problem not found: %+v", result.Problems)
	}
}

func TestValidateLayerBadModeFormatIgnore(t *testing.T) {
	m := Manifest{Name: "test", Files: []FileEntry{{ID: "a", Source: "files/a", Target: "~/a", Mode: "bad", Format: "bad"}}, Ignore: []string{"["}}
	result := ValidateLayer(ValidationInput{LayerName: "test", LayerRoot: t.TempDir(), Manifest: m, Expander: testExpander()})
	for _, code := range []string{"manifest.mode_invalid", "manifest.format_invalid", "manifest.ignore_invalid"} {
		if !hasProblem(result.Problems, code) {
			t.Fatalf("problem %s not found: %+v", code, result.Problems)
		}
	}
}

func TestValidateLayerTargetAndSecretPolicy(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "files", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Manifest{Name: "test", Files: []FileEntry{{ID: "a", Source: "files/a.txt", Target: "/etc/hosts", Mode: ModeRender, Secrets: []string{"BAD-NAME"}}}}
	result := ValidateLayer(ValidationInput{LayerName: "test", LayerRoot: root, Manifest: m, Expander: testExpander()})
	for _, code := range []string{"manifest.target_invalid", "manifest.secret_invalid"} {
		if !hasProblem(result.Problems, code) {
			t.Fatalf("problem %s not found: %+v", code, result.Problems)
		}
	}
}

func testExpander() Expander {
	return Expander{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: "/Users/alice"}}
}

func hasProblem(problems []Problem, code string) bool {
	for _, problem := range problems {
		if problem.Code == code {
			return true
		}
	}
	return false
}
