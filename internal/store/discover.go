package store

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/asudbring/loki-profile-manager/internal/config"
)

func DiscoverProviderFolders(opts DiscoveryOptions) []ProviderCandidate {
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	env := opts.Env
	if env == nil {
		env = os.Getenv
	}
	exists := opts.Exists
	if exists == nil {
		exists = pathExists
	}
	glob := opts.Glob
	if glob == nil {
		glob = filepath.Glob
	}

	home := opts.HomeDir
	if strings.TrimSpace(home) == "" {
		if goos == "windows" {
			home = firstNonEmpty(env("USERPROFILE"), env("HOME"))
		} else {
			home = env("HOME")
		}
	}

	candidates := []ProviderCandidate{}
	seen := map[string]bool{}
	add := func(provider ProviderType, root, source string, manual bool) {
		root = config.CleanForOS(goos, root)
		if root == "" || root == "." {
			return
		}
		storePath := root
		if !manual {
			storePath = config.JoinForOS(goos, root, StoreDirName)
		}
		key := strings.ToLower(config.CleanForOS(goos, storePath))
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, ProviderCandidate{
			Provider:  provider,
			Path:      root,
			StorePath: storePath,
			Source:    source,
			Exists:    exists(root),
		})
	}

	if strings.TrimSpace(opts.ManualPath) != "" {
		add(ProviderManual, opts.ManualPath, "manual", true)
	}

	if goos == "windows" {
		for _, key := range []string{"ONEDRIVE", "OneDrive", "OneDriveCommercial", "OneDriveConsumer"} {
			if value := env(key); value != "" {
				add(ProviderOneDrive, value, "env:"+key, false)
			}
		}
		if home != "" {
			add(ProviderOneDrive, config.JoinForOS(goos, home, "OneDrive"), "known-path", false)
			add(ProviderDropbox, config.JoinForOS(goos, home, "Dropbox"), "known-path", false)
		}
		return candidates
	}

	if home != "" {
		add(ProviderOneDrive, config.JoinForOS(goos, home, "OneDrive"), "known-path", false)
		if goos == "darwin" {
			pattern := config.JoinForOS(goos, home, "Library", "CloudStorage", "OneDrive*")
			if matches, err := glob(pattern); err == nil {
				for _, match := range matches {
					add(ProviderOneDrive, match, "cloudstorage", false)
				}
			}
		}
		add(ProviderDropbox, config.JoinForOS(goos, home, "Dropbox"), "known-path", false)
	}
	return candidates
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
