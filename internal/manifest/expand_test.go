package manifest

import (
	"testing"

	"github.com/allensu/loki-profile-manager/internal/config"
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
