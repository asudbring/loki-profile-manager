package manifest

import (
	"strings"
	"testing"
)

func TestParseValidManifest(t *testing.T) {
	m, err := ParseBytes("valid.yaml", []byte(`version: 1
name: work-core
files:
  - id: gitconfig
    source: files/.gitconfig
    target: ~/.gitconfig
    mode: symlink
skills: []
ignore:
  - "**/Cache/**"
merge_rules: {}
targets:
  vscode_user_dir:
    windows: "${APPDATA}/Code/User"
    darwin: "${HOME}/Library/Application Support/Code/User"
`))
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	if m.Version != 1 || m.Name != "work-core" || len(m.Files) != 1 || m.Targets["vscode_user_dir"].Windows == "" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
}

func TestParseMalformedYAMLIncludesPath(t *testing.T) {
	_, err := ParseBytes("bad.yaml", []byte("version: ["))
	if err == nil || !strings.Contains(err.Error(), "bad.yaml") {
		t.Fatalf("error = %v, want path", err)
	}
}

func TestParseUnsupportedVersion(t *testing.T) {
	_, err := ParseBytes("future.yaml", []byte("version: 99\nname: future\n"))
	if err == nil || !strings.Contains(err.Error(), "unsupported version") {
		t.Fatalf("error = %v, want unsupported version", err)
	}
}

func TestParseUnknownFieldFails(t *testing.T) {
	_, err := ParseBytes("unknown.yaml", []byte("version: 1\nname: test\nwat: nope\n"))
	if err == nil {
		t.Fatal("ParseBytes() error = nil, want unknown field error")
	}
}
