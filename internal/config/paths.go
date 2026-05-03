package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"
)

const AppDirName = "loki-profile-manager"

// EnvLookup returns environment variable values. It exists so tests can avoid
// reading the real process environment.
type EnvLookup func(string) string

// PathResolver resolves OS-specific Loki paths. All fields are injectable so
// tests never need the real home directory or app-data folder.
type PathResolver struct {
	GOOS         string
	HomeDir      string
	LocalAppData string
	Env          EnvLookup
}

// LocalPaths contains all Phase 1 local runtime paths. These paths are machine
// local and must not be treated as the synced Loki source of truth.
type LocalPaths struct {
	StateDir      string `json:"state_dir"`
	DBPath        string `json:"db_path"`
	LogDir        string `json:"log_dir"`
	LogPath       string `json:"log_path"`
	SnapshotDir   string `json:"snapshot_dir"`
	LockDir       string `json:"lock_dir"`
	CacheDir      string `json:"cache_dir"`
	MachineIDPath string `json:"machine_id_path"`
}

// NewPathResolverFromEnv builds a resolver from the real process environment.
// Command code may use this; tests should pass explicit fields instead.
func NewPathResolverFromEnv() PathResolver {
	home, _ := os.UserHomeDir()
	return PathResolver{
		GOOS:         runtime.GOOS,
		HomeDir:      home,
		LocalAppData: os.Getenv("LOCALAPPDATA"),
		Env:          os.Getenv,
	}
}

// WithDefaults fills unset runtime-dependent fields. It intentionally does not
// read the real home directory unless every injected value is absent.
func (r PathResolver) WithDefaults() PathResolver {
	if r.GOOS == "" {
		r.GOOS = runtime.GOOS
	}
	if r.Env == nil {
		r.Env = func(string) string { return "" }
	}
	if r.HomeDir == "" {
		if r.GOOS == "windows" {
			r.HomeDir = firstNonEmpty(r.Env("USERPROFILE"), r.Env("HOME"))
		} else {
			r.HomeDir = r.Env("HOME")
		}
	}
	if r.LocalAppData == "" && r.GOOS == "windows" {
		r.LocalAppData = r.Env("LOCALAPPDATA")
	}
	return r
}

// ResolveLocalPaths returns local machine-state paths for Loki.
func (r PathResolver) ResolveLocalPaths() (LocalPaths, error) {
	r = r.WithDefaults()

	var stateDir string
	switch r.GOOS {
	case "windows":
		base := r.LocalAppData
		if base == "" && r.HomeDir != "" {
			base = joinForOS(r.GOOS, r.HomeDir, "AppData", "Local")
		}
		if base == "" {
			return LocalPaths{}, errors.New("resolve local state path: LOCALAPPDATA or home directory is required on Windows")
		}
		stateDir = joinForOS(r.GOOS, base, AppDirName)
	case "darwin":
		if r.HomeDir == "" {
			return LocalPaths{}, errors.New("resolve local state path: home directory is required on macOS")
		}
		stateDir = joinForOS(r.GOOS, r.HomeDir, "Library", "Application Support", AppDirName)
	default:
		// Linux is not a V1 target, but supporting it keeps development and CI sane.
		if r.HomeDir == "" {
			return LocalPaths{}, fmt.Errorf("resolve local state path: home directory is required on %s", r.GOOS)
		}
		stateDir = joinForOS(r.GOOS, r.HomeDir, ".local", "state", AppDirName)
	}

	return LocalPaths{
		StateDir:      stateDir,
		DBPath:        joinForOS(r.GOOS, stateDir, "state.sqlite"),
		LogDir:        joinForOS(r.GOOS, stateDir, "logs"),
		LogPath:       joinForOS(r.GOOS, stateDir, "logs", "loki.log"),
		SnapshotDir:   joinForOS(r.GOOS, stateDir, "snapshots"),
		LockDir:       joinForOS(r.GOOS, stateDir, "locks"),
		CacheDir:      joinForOS(r.GOOS, stateDir, "cache"),
		MachineIDPath: joinForOS(r.GOOS, stateDir, "machine_id"),
	}, nil
}

// CleanStoreOverride normalizes a user-provided store override path without
// creating or validating it.
func (r PathResolver) CleanStoreOverride(value string) string {
	r = r.WithDefaults()
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return cleanForOS(r.GOOS, value)
}

// JoinForOS joins path elements using the requested OS convention. It is
// exported for tests and cross-platform discovery code that simulate Windows or
// macOS paths on any host OS.
func JoinForOS(goos string, parts ...string) string {
	return joinForOS(goos, parts...)
}

// CleanForOS cleans a path using the requested OS convention.
func CleanForOS(goos, value string) string {
	return cleanForOS(goos, value)
}

func joinForOS(goos string, parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			cleaned = append(cleaned, p)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	if goos == "windows" {
		out := strings.TrimRight(strings.ReplaceAll(cleaned[0], "/", `\`), `\`)
		for _, p := range cleaned[1:] {
			p = strings.Trim(strings.ReplaceAll(p, "/", `\`), `\`)
			if p == "" {
				continue
			}
			if out == "" {
				out = p
			} else {
				out += `\` + p
			}
		}
		return cleanForOS(goos, out)
	}
	return path.Clean(strings.Join(cleaned, "/"))
}

func cleanForOS(goos, value string) string {
	if goos == "windows" {
		value = strings.ReplaceAll(value, "/", `\`)
		for strings.Contains(value, `\\`) {
			value = strings.ReplaceAll(value, `\\`, `\`)
		}
		return strings.TrimRight(value, `\`)
	}
	return path.Clean(strings.ReplaceAll(value, `\`, "/"))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
