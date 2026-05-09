package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/asudbring/loki-profile-manager/internal/config"
)

func TestExpandTargetWindows(t *testing.T) {
	expander := Expander{
		Resolver: config.PathResolver{
			GOOS:         "windows",
			HomeDir:      `C:\Users\alice`,
			LocalAppData: `C:\Users\alice\AppData\Local`,
			Env: func(key string) string {
				if key == "APPDATA" {
					return `C:\Users\alice\AppData\Roaming`
				}
				return ""
			},
		},
		Targets: map[string]TargetValue{"vscode_user_dir": {Windows: `%APPDATA%\Code\User`}},
	}
	got, err := expander.ExpandTarget(`${vscode_user_dir}\settings.json`)
	if err != nil {
		t.Fatalf("ExpandTarget() error = %v", err)
	}
	want := `C:\Users\alice\AppData\Roaming\Code\User\settings.json`
	if got != want {
		t.Fatalf("ExpandTarget() = %q, want %q", got, want)
	}
}

func TestExpandTargetMac(t *testing.T) {
	expander := Expander{
		Resolver: config.PathResolver{GOOS: "darwin", HomeDir: "/Users/alice", Env: func(key string) string { return "" }},
		Targets:  map[string]TargetValue{"vscode_user_dir": {Darwin: "${HOME}/Library/Application Support/Code/User"}},
	}
	got, err := expander.ExpandTarget("${vscode_user_dir}/settings.json")
	if err != nil {
		t.Fatalf("ExpandTarget() error = %v", err)
	}
	want := "/Users/alice/Library/Application Support/Code/User/settings.json"
	if got != want {
		t.Fatalf("ExpandTarget() = %q, want %q", got, want)
	}
}

func TestExpandTargetLinux(t *testing.T) {
	expander := Expander{
		Resolver: config.PathResolver{GOOS: "linux", HomeDir: "/home/alice", Env: func(key string) string { return "" }},
		Targets:  map[string]TargetValue{"vscode_user_dir": {Linux: "${HOME}/.config/Code/User", Default: "${HOME}/fallback"}},
	}
	got, err := expander.ExpandTarget("${vscode_user_dir}/settings.json")
	if err != nil {
		t.Fatalf("ExpandTarget() error = %v", err)
	}
	want := "/home/alice/.config/Code/User/settings.json"
	if got != want {
		t.Fatalf("ExpandTarget() = %q, want %q", got, want)
	}
}

func TestExpandUnknownVariableFails(t *testing.T) {
	expander := Expander{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: "/Users/alice"}}
	if _, err := expander.ExpandTarget("${MISSING}/file"); err == nil {
		t.Fatal("ExpandTarget() error = nil, want error")
	}
}

func TestResolveSourceRejectsEscape(t *testing.T) {
	if _, err := ResolveSource(t.TempDir(), "../outside"); err == nil {
		t.Fatal("ResolveSource() error = nil, want escape error")
	}
}

func TestValidateTargetPathRequiresHomeRoot(t *testing.T) {
	expander := Expander{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: "/Users/alice"}}
	if err := expander.ValidateTargetPath("/Users/alice/.config/app/config.json"); err != nil {
		t.Fatalf("ValidateTargetPath(home) error = %v", err)
	}
	for _, target := range []string{"relative/file", "/etc/hosts", "/Users/alice", "/Users/alice/../bob/file"} {
		if err := expander.ValidateTargetPath(target); err == nil {
			t.Fatalf("ValidateTargetPath(%q) error = nil", target)
		}
	}
}

func TestValidateSourceWithinRootRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "files", "link.txt")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	resolved, err := ResolveSource(root, "files/link.txt")
	if err != nil {
		t.Fatalf("ResolveSource() error = %v", err)
	}
	if err := ValidateSourceWithinRoot(root, resolved); err == nil {
		t.Fatal("ValidateSourceWithinRoot() error = nil, want symlink escape")
	}
}
