package store

import (
	"encoding/json"
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
	readFile := opts.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
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
				add(oneDriveProviderForEnv(key), value, "env:"+key, false)
			}
		}
		for _, root := range dropboxInfoRoots(goos, home, env, readFile) {
			add(ProviderDropbox, root.path, root.source, false)
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
					add(oneDriveProviderForCloudStorage(match), match, "cloudstorage", false)
				}
			}
		}
		for _, root := range dropboxInfoRoots(goos, home, env, readFile) {
			add(ProviderDropbox, root.path, root.source, false)
		}
		add(ProviderDropbox, config.JoinForOS(goos, home, "Dropbox"), "known-path", false)
	}
	return candidates
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

type dropboxInfoRoot struct {
	path   string
	source string
}

func oneDriveProviderForEnv(key string) ProviderType {
	switch key {
	case "OneDriveCommercial":
		return ProviderOneDriveBusiness
	case "OneDriveConsumer":
		return ProviderOneDrivePersonal
	default:
		return ProviderOneDrive
	}
}

func oneDriveProviderForCloudStorage(path string) ProviderType {
	name := strings.ToLower(filepath.Base(filepath.Clean(path)))
	if name == "onedrive-personal" || name == "onedrive consumer" {
		return ProviderOneDrivePersonal
	}
	if strings.HasPrefix(name, "onedrive-") || strings.Contains(name, "onedrive - ") {
		return ProviderOneDriveBusiness
	}
	return ProviderOneDrive
}

func dropboxInfoRoots(goos, home string, env func(string) string, readFile func(string) ([]byte, error)) []dropboxInfoRoot {
	paths := dropboxInfoPaths(goos, home, env)
	roots := []dropboxInfoRoot{}
	seen := map[string]bool{}
	for _, infoPath := range paths {
		content, err := readFile(infoPath)
		if err != nil || len(content) == 0 {
			continue
		}
		var payload map[string]struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(content, &payload); err != nil {
			continue
		}
		for _, key := range []string{"business", "personal"} {
			entry, ok := payload[key]
			if !ok || strings.TrimSpace(entry.Path) == "" {
				continue
			}
			root := config.CleanForOS(goos, entry.Path)
			seenKey := strings.ToLower(root)
			if seen[seenKey] {
				continue
			}
			seen[seenKey] = true
			roots = append(roots, dropboxInfoRoot{path: root, source: "dropbox-info:" + key})
		}
	}
	return roots
}

func dropboxInfoPaths(goos, home string, env func(string) string) []string {
	paths := []string{}
	if goos == "windows" {
		for _, root := range []string{env("APPDATA"), env("LOCALAPPDATA")} {
			if strings.TrimSpace(root) != "" {
				paths = append(paths, config.JoinForOS(goos, root, "Dropbox", "info.json"))
			}
		}
		return paths
	}
	if strings.TrimSpace(home) != "" {
		paths = append(paths, config.JoinForOS(goos, home, ".dropbox", "info.json"))
	}
	return paths
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
