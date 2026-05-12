package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/asudbring/loki-profile-manager/internal/infisical"
)

type SecretsConfigureInfisicalRequest struct{}

type SecretsConfigureInfisicalResult struct {
	Provider  string              `json:"provider"`
	EnvPath   string              `json:"env_path"`
	Created   bool                `json:"created"`
	Updated   []string            `json:"updated,omitempty"`
	Preserved []string            `json:"preserved,omitempty"`
	Missing   []string            `json:"missing,omitempty"`
	Status    SecretsStatusResult `json:"status"`
}

var infisicalEnvWriteOrder = []string{
	"INFISICAL_AUTH_METHOD",
	"INFISICAL_CLIENT_ID",
	"INFISICAL_CLIENT_SECRET",
	"INFISICAL_PROJECT_ID",
	"INFISICAL_ENV",
	"INFISICAL_API_URL",
	"INFISICAL_HOST",
	"INFISICAL_HOST_URL",
	"INFISICAL_TOKEN",
}

func (s *Service) SecretsConfigureInfisical(ctx context.Context, req SecretsConfigureInfisicalRequest) (SecretsConfigureInfisicalResult, error) {
	_ = req
	if s == nil {
		return SecretsConfigureInfisicalResult{}, fmt.Errorf("secrets configure infisical: service is nil")
	}
	resolver := s.resolver.WithDefaults()
	envPath := infisical.DefaultEnvPathForHome(resolver.HomeDir)
	if envPath == "" {
		return SecretsConfigureInfisicalResult{}, fmt.Errorf("secrets configure infisical: home directory is required")
	}
	result := SecretsConfigureInfisicalResult{Provider: "infisical", EnvPath: envPath, Updated: []string{}, Preserved: []string{}, Missing: []string{}}

	existing, err := readLocalEnvFile(envPath)
	if err != nil {
		return result, err
	}
	if _, err := os.Stat(envPath); errors.Is(err, os.ErrNotExist) {
		result.Created = true
	} else if err != nil {
		return result, fmt.Errorf("stat %s: %w", envPath, err)
	}

	candidates := s.discoverInfisicalConfigValues(ctx)
	if existing["INFISICAL_AUTH_METHOD"] == "" && candidates["INFISICAL_AUTH_METHOD"] == "" && candidates["INFISICAL_CLIENT_ID"] != "" && candidates["INFISICAL_CLIENT_SECRET"] != "" {
		candidates["INFISICAL_AUTH_METHOD"] = "universal-auth"
	}
	if existing["INFISICAL_ENV"] == "" && candidates["INFISICAL_ENV"] == "" {
		candidates["INFISICAL_ENV"] = "dev"
	}

	for _, key := range infisicalEnvWriteOrder {
		if strings.TrimSpace(existing[key]) != "" {
			result.Preserved = append(result.Preserved, key)
			continue
		}
		if value := strings.TrimSpace(candidates[key]); value != "" {
			if err := appendEnvAssignment(envPath, key, value, result.Created && len(result.Updated) == 0); err != nil {
				return result, err
			}
			existing[key] = value
			result.Updated = append(result.Updated, key)
		}
	}

	result.Missing = missingInfisicalConfigKeys(existing)
	status, err := s.SecretsStatus(ctx, SecretsStatusRequest{})
	if err != nil {
		return result, err
	}
	result.Status = status
	sort.Strings(result.Updated)
	sort.Strings(result.Preserved)
	sort.Strings(result.Missing)
	return result, nil
}

func (s *Service) discoverInfisicalConfigValues(ctx context.Context) map[string]string {
	out := map[string]string{}
	resolver := s.resolver.WithDefaults()
	lookup := func(key string) string {
		if resolver.Env == nil {
			return ""
		}
		return strings.TrimSpace(resolver.Env(key))
	}
	copyEnv := func(target string, keys ...string) {
		if out[target] != "" {
			return
		}
		for _, key := range keys {
			if value := lookup(key); value != "" {
				out[target] = value
				return
			}
		}
	}
	copyEnv("INFISICAL_TOKEN", "INFISICAL_TOKEN")
	copyEnv("INFISICAL_AUTH_METHOD", "INFISICAL_AUTH_METHOD")
	copyEnv("INFISICAL_CLIENT_ID", "INFISICAL_CLIENT_ID", "INFISICAL_UNIVERSAL_AUTH_CLIENT_ID")
	copyEnv("INFISICAL_CLIENT_SECRET", "INFISICAL_CLIENT_SECRET", "INFISICAL_UNIVERSAL_AUTH_CLIENT_SECRET")
	copyEnv("INFISICAL_PROJECT_ID", "INFISICAL_PROJECT_ID")
	copyEnv("INFISICAL_ENV", "INFISICAL_ENV", "INFISICAL_ENVIRONMENT")
	copyEnv("INFISICAL_API_URL", "INFISICAL_API_URL")
	copyEnv("INFISICAL_HOST", "INFISICAL_HOST")
	copyEnv("INFISICAL_HOST_URL", "INFISICAL_HOST_URL")

	for _, dir := range s.infisicalProjectConfigDirs(ctx) {
		mergeInfisicalProjectConfig(out, filepath.Join(dir, ".infisical.json"))
	}
	return out
}

func (s *Service) infisicalProjectConfigDirs(ctx context.Context) []string {
	seen := map[string]bool{}
	var dirs []string
	add := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return
		}
		clean, err := filepath.Abs(dir)
		if err != nil {
			clean = filepath.Clean(dir)
		}
		if !seen[clean] {
			seen[clean] = true
			dirs = append(dirs, clean)
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		add(cwd)
	}
	if storePath, err := s.effectiveStorePath(ctx, ""); err == nil {
		add(storePath)
	}
	return dirs
}

func mergeInfisicalProjectConfig(values map[string]string, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return
	}
	pick := func(keys ...string) string {
		for _, key := range keys {
			if value, ok := raw[key]; ok {
				if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
					return text
				}
			}
		}
		return ""
	}
	if values["INFISICAL_PROJECT_ID"] == "" {
		values["INFISICAL_PROJECT_ID"] = pick("projectId", "projectID", "workspaceId", "workspaceID")
	}
	if values["INFISICAL_ENV"] == "" {
		values["INFISICAL_ENV"] = pick("defaultEnvironment", "environment", "env")
	}
}

func readLocalEnvFile(path string) (map[string]string, error) {
	values := map[string]string{}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return values, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := parseLocalEnvLine(line)
		if ok {
			values[key] = value
		}
	}
	return values, nil
}

func parseLocalEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	if strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "export\t") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "export "), "export\t"))
	}
	idx := strings.Index(line, "=")
	if idx <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	if err := infisical.ValidateSecretName(key); err != nil {
		return "", "", false
	}
	value := strings.TrimSpace(line[idx+1:])
	if len(value) >= 2 {
		quote := value[0]
		if quote == '\'' || quote == '"' {
			if end := strings.LastIndexByte(value[1:], quote); end >= 0 {
				return key, value[1 : end+1], true
			}
		}
	}
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return key, value, true
}

func appendEnvAssignment(path, key, value string, includeHeader bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if !includeHeader {
		if err := ensureTrailingNewline(path); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	if includeHeader {
		if _, err := file.WriteString("# Loki Infisical machine-auth configuration. Secret values stay local.\n"); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	if _, err := file.WriteString(key + "=" + formatEnvValue(value) + "\n"); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func ensureTrailingNewline(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || len(data) == 0 || data[len(data)-1] == '\n' {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	if _, err := file.WriteString("\n"); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func formatEnvValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, " \t#\r\n\"'") {
		if !strings.Contains(value, "'") && !strings.ContainsAny(value, "\r\n") {
			return "'" + value + "'"
		}
		if !strings.Contains(value, "\"") && !strings.ContainsAny(value, "\r\n") {
			return "\"" + value + "\""
		}
	}
	return value
}

func missingInfisicalConfigKeys(values map[string]string) []string {
	has := func(key string) bool { return strings.TrimSpace(values[key]) != "" }
	var missing []string
	if !has("INFISICAL_TOKEN") && !(has("INFISICAL_CLIENT_ID") && has("INFISICAL_CLIENT_SECRET")) {
		missing = append(missing, "INFISICAL_CLIENT_ID", "INFISICAL_CLIENT_SECRET")
	}
	if !has("INFISICAL_PROJECT_ID") {
		missing = append(missing, "INFISICAL_PROJECT_ID")
	}
	return missing
}
