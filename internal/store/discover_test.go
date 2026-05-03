package store

import (
	"path/filepath"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/config"
)

func TestDiscoverWindowsEnvOneDrive(t *testing.T) {
	env := map[string]string{"ONEDRIVE": `C:\Users\alice\OneDrive`}
	candidates := DiscoverProviderFolders(DiscoveryOptions{
		GOOS: "windows",
		Env:  func(key string) string { return env[key] },
	})
	if len(candidates) != 1 {
		t.Fatalf("candidates = %d, want 1: %+v", len(candidates), candidates)
	}
	got := candidates[0]
	if got.Provider != ProviderOneDrive || got.StorePath != `C:\Users\alice\OneDrive\loki` || got.Source != "env:ONEDRIVE" {
		t.Fatalf("unexpected candidate: %+v", got)
	}
}

func TestDiscoverWindowsHomeDropboxFallback(t *testing.T) {
	candidates := DiscoverProviderFolders(DiscoveryOptions{GOOS: "windows", HomeDir: `C:\Users\alice`})
	if !hasCandidate(candidates, ProviderDropbox, `C:\Users\alice\Dropbox\loki`) {
		t.Fatalf("dropbox candidate missing: %+v", candidates)
	}
}

func TestDiscoverMacDropbox(t *testing.T) {
	home := t.TempDir()
	dropbox := filepath.Join(home, "Dropbox")
	candidates := DiscoverProviderFolders(DiscoveryOptions{GOOS: "darwin", HomeDir: home, Exists: func(path string) bool { return path == dropbox }})
	if !hasCandidate(candidates, ProviderDropbox, config.JoinForOS("darwin", dropbox, StoreDirName)) {
		t.Fatalf("dropbox candidate missing: %+v", candidates)
	}
}

func TestDiscoverMacCloudStorageOneDrive(t *testing.T) {
	home := t.TempDir()
	cloud := filepath.Join(home, "Library", "CloudStorage", "OneDrive-Contoso")
	candidates := DiscoverProviderFolders(DiscoveryOptions{
		GOOS:    "darwin",
		HomeDir: home,
		Glob: func(pattern string) ([]string, error) {
			return []string{cloud}, nil
		},
	})
	if !hasCandidate(candidates, ProviderOneDrive, config.JoinForOS("darwin", cloud, StoreDirName)) {
		t.Fatalf("cloudstorage candidate missing: %+v", candidates)
	}
}

func TestDiscoverManualReturnedWithoutProviders(t *testing.T) {
	manual := filepath.Join(t.TempDir(), "custom-loki")
	candidates := DiscoverProviderFolders(DiscoveryOptions{GOOS: "darwin", ManualPath: manual})
	if len(candidates) == 0 || candidates[0].Provider != ProviderManual || candidates[0].StorePath != config.CleanForOS("darwin", manual) {
		t.Fatalf("manual candidate missing: %+v", candidates)
	}
}

func TestDiscoverDedupesCandidates(t *testing.T) {
	env := map[string]string{"ONEDRIVE": `C:\Users\alice\OneDrive`}
	candidates := DiscoverProviderFolders(DiscoveryOptions{
		GOOS:    "windows",
		HomeDir: `C:\Users\alice`,
		Env:     func(key string) string { return env[key] },
	})
	count := 0
	for _, candidate := range candidates {
		if candidate.StorePath == `C:\Users\alice\OneDrive\loki` {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("duplicate count = %d, candidates: %+v", count, candidates)
	}
}

func hasCandidate(candidates []ProviderCandidate, provider ProviderType, storePath string) bool {
	for _, candidate := range candidates {
		if candidate.Provider == provider && candidate.StorePath == storePath {
			return true
		}
	}
	return false
}
