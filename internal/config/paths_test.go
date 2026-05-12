package config

import "testing"

func TestResolveLocalPathsWindowsLocalAppData(t *testing.T) {
	resolver := PathResolver{GOOS: "windows", LocalAppData: `C:\Users\alice\AppData\Local`}
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	want := `C:\Users\alice\AppData\Local\loki-profile-manager`
	if paths.StateDir != want {
		t.Fatalf("StateDir = %q, want %q", paths.StateDir, want)
	}
	if paths.DBPath != want+`\state.sqlite` {
		t.Fatalf("DBPath = %q", paths.DBPath)
	}
}

func TestResolveLocalPathsWindowsHomeFallback(t *testing.T) {
	resolver := PathResolver{GOOS: "windows", HomeDir: `C:\Users\alice`}
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	want := `C:\Users\alice\AppData\Local\loki-profile-manager`
	if paths.StateDir != want {
		t.Fatalf("StateDir = %q, want %q", paths.StateDir, want)
	}
	if paths.ActiveProfilePath != want+`\active_profile.txt` {
		t.Fatalf("ActiveProfilePath = %q", paths.ActiveProfilePath)
	}
}

func TestWithDefaultsUsesRedirectedDocumentsDirectory(t *testing.T) {
	resolver := PathResolver{
		GOOS:    "windows",
		HomeDir: `C:\Users\alice`,
		Env: func(key string) string {
			if key == "LOKI_DOCUMENTS_DIR" {
				return `C:\Mac\Home\Documents`
			}
			return ""
		},
	}.WithDefaults()
	if resolver.DocumentsDir != `C:\Mac\Home\Documents` {
		t.Fatalf("DocumentsDir = %q", resolver.DocumentsDir)
	}
}

func TestResolveLocalPathsWindowsMissingBase(t *testing.T) {
	resolver := PathResolver{GOOS: "windows"}
	if _, err := resolver.ResolveLocalPaths(); err == nil {
		t.Fatal("ResolveLocalPaths() error = nil, want error")
	}
}

func TestResolveLocalPathsMacOS(t *testing.T) {
	resolver := PathResolver{GOOS: "darwin", HomeDir: "/Users/alice"}
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	want := "/Users/alice/Library/Application Support/loki-profile-manager"
	if paths.StateDir != want {
		t.Fatalf("StateDir = %q, want %q", paths.StateDir, want)
	}
}

func TestResolveLocalPathsMacOSMissingHome(t *testing.T) {
	resolver := PathResolver{GOOS: "darwin"}
	if _, err := resolver.ResolveLocalPaths(); err == nil {
		t.Fatal("ResolveLocalPaths() error = nil, want error")
	}
}

func TestResolveLocalPathsLinux(t *testing.T) {
	resolver := PathResolver{GOOS: "linux", HomeDir: "/home/alice"}
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	want := "/home/alice/.local/state/loki-profile-manager"
	if paths.StateDir != want {
		t.Fatalf("StateDir = %q, want %q", paths.StateDir, want)
	}
	if paths.DBPath != want+"/state.sqlite" {
		t.Fatalf("DBPath = %q", paths.DBPath)
	}
}

func TestCleanStoreOverride(t *testing.T) {
	resolver := PathResolver{GOOS: "darwin", HomeDir: "/Users/alice"}
	got := resolver.CleanStoreOverride("/Users/alice/OneDrive//loki/")
	want := "/Users/alice/OneDrive/loki"
	if got != want {
		t.Fatalf("CleanStoreOverride() = %q, want %q", got, want)
	}
}
