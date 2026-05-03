package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeDryRunJSONCompatible(t *testing.T) {
	root := t.TempDir()
	left := writeTemp(t, root, "left.json", `{"editor":{"fontSize":12},"a":1}`)
	right := writeTemp(t, root, "right.json", `{"editor":{"lineNumbers":"on"},"a":2}`)
	problems := MergeDryRun([]FileOperation{
		{LayerName: "common", SourcePath: left, TargetPath: "/target/settings.json", Entry: FileEntry{Mode: ModeMerge, Format: FormatJSON}},
		{LayerName: "work", SourcePath: right, TargetPath: "/target/settings.json", Entry: FileEntry{Mode: ModeMerge, Format: FormatJSON}},
	})
	if len(problems) != 0 {
		t.Fatalf("problems = %+v", problems)
	}
}

func TestMergeDryRunYAMLAndTOMLCompatible(t *testing.T) {
	root := t.TempDir()
	yamlA := writeTemp(t, root, "a.yaml", "editor:\n  fontSize: 12\n")
	yamlB := writeTemp(t, root, "b.yaml", "editor:\n  lineNumbers: on\n")
	tomlA := writeTemp(t, root, "a.toml", "[editor]\nfontSize = 12\n")
	tomlB := writeTemp(t, root, "b.toml", "[editor]\nlineNumbers = \"on\"\n")
	ops := []FileOperation{
		{SourcePath: yamlA, TargetPath: "/target/a.yaml", Entry: FileEntry{Mode: ModeMerge, Format: FormatYAML}},
		{SourcePath: yamlB, TargetPath: "/target/a.yaml", Entry: FileEntry{Mode: ModeMerge, Format: FormatYAML}},
		{SourcePath: tomlA, TargetPath: "/target/a.toml", Entry: FileEntry{Mode: ModeMerge, Format: FormatTOML}},
		{SourcePath: tomlB, TargetPath: "/target/a.toml", Entry: FileEntry{Mode: ModeMerge, Format: FormatTOML}},
	}
	if problems := MergeDryRun(ops); len(problems) != 0 {
		t.Fatalf("problems = %+v", problems)
	}
}

func TestMergeDryRunTypeConflict(t *testing.T) {
	root := t.TempDir()
	left := writeTemp(t, root, "left.json", `{"editor":{"fontSize":12}}`)
	right := writeTemp(t, root, "right.json", `{"editor":"bad"}`)
	problems := MergeDryRun([]FileOperation{
		{SourcePath: left, TargetPath: "/target/settings.json", Entry: FileEntry{Mode: ModeMerge, Format: FormatJSON}},
		{SourcePath: right, TargetPath: "/target/settings.json", Entry: FileEntry{Mode: ModeMerge, Format: FormatJSON}},
	})
	if !hasProblem(problems, "merge.type_conflict") {
		t.Fatalf("type conflict not found: %+v", problems)
	}
}

func TestMergeDryRunDuplicateText(t *testing.T) {
	root := t.TempDir()
	left := writeTemp(t, root, "left.txt", "one")
	right := writeTemp(t, root, "right.txt", "two")
	problems := MergeDryRun([]FileOperation{
		{SourcePath: left, TargetPath: "/target/file.txt", Entry: FileEntry{Mode: ModeCopy}},
		{SourcePath: right, TargetPath: "/target/file.txt", Entry: FileEntry{Mode: ModeCopy}},
	})
	if !hasProblem(problems, "merge.duplicate_target") {
		t.Fatalf("duplicate target not found: %+v", problems)
	}
	copyPath := writeTemp(t, root, "copy.txt", "one")
	if problems := MergeDryRun([]FileOperation{{SourcePath: left, TargetPath: "/target/same", Entry: FileEntry{Mode: ModeCopy}}, {SourcePath: copyPath, TargetPath: "/target/same", Entry: FileEntry{Mode: ModeCopy}}}); !hasProblem(problems, "merge.duplicate_target") {
		t.Fatalf("identical duplicate target should block: %+v", problems)
	}
}

func writeTemp(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
