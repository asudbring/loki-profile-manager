package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/asudbring/loki-profile-manager/internal/infisical"
)

type SecretsConfigureInfisicalRequest struct {
	ProjectID         string
	Environment       string
	ClientID          string
	ClientSecret      string
	HostURL           string
	OverwriteExisting bool
	SkipVerify        bool
}

type SecretsConfigureInfisicalResult struct {
	Provider  string              `json:"provider"`
	EnvPath   string              `json:"env_path"`
	Created   bool                `json:"created"`
	Updated   []string            `json:"updated,omitempty"`
	Preserved []string            `json:"preserved,omitempty"`
	Missing   []string            `json:"missing,omitempty"`
	Status    SecretsStatusResult `json:"status"`
	Verified  bool                `json:"verified"`
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
}

func (s *Service) SecretsConfigureInfisical(ctx context.Context, req SecretsConfigureInfisicalRequest) (SecretsConfigureInfisicalResult, error) {
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
	normalizeInfisicalAliases(existing)
	envExists := true
	if _, err := os.Stat(envPath); errors.Is(err, os.ErrNotExist) {
		envExists = false
	} else if err != nil {
		return result, fmt.Errorf("stat %s: %w", envPath, err)
	}

	explicit := req.hasExplicitInfisicalValues()
	var candidates map[string]string
	if explicit {
		candidates, err = req.infisicalEnvValues()
		if err != nil {
			return result, err
		}
	} else {
		candidates = s.discoverInfisicalConfigValues(ctx)
		if err := validateInfisicalHostValues(candidates); err != nil {
			return result, err
		}
		if existing["INFISICAL_AUTH_METHOD"] == "" && candidates["INFISICAL_AUTH_METHOD"] == "" && candidates["INFISICAL_CLIENT_ID"] != "" && candidates["INFISICAL_CLIENT_SECRET"] != "" {
			candidates["INFISICAL_AUTH_METHOD"] = "universal-auth"
		}
		if existing["INFISICAL_ENV"] == "" && candidates["INFISICAL_ENV"] == "" && hasAnyInfisicalMachineConfig(existing, candidates) {
			candidates["INFISICAL_ENV"] = "dev"
		}
	}

	assignments := map[string]string{}
	writeKeys := append([]string{}, infisicalEnvWriteOrder...)
	if explicit || (strings.TrimSpace(existing["INFISICAL_TOKEN"]) != "" && (strings.TrimSpace(existing["INFISICAL_CLIENT_ID"]) != "" || strings.TrimSpace(candidates["INFISICAL_CLIENT_ID"]) != "")) {
		writeKeys = append(writeKeys, "INFISICAL_TOKEN")
		candidates["INFISICAL_TOKEN"] = ""
	}
	for _, key := range writeKeys {
		existingValue := strings.TrimSpace(existing[key])
		candidateValue := strings.TrimSpace(candidates[key])
		_, candidateSet := candidates[key]
		if existingValue != "" && (!candidateSet || !req.OverwriteExisting) {
			result.Preserved = append(result.Preserved, key)
			continue
		}
		if candidateSet && (candidateValue != "" || req.OverwriteExisting) {
			assignments[key] = candidateValue
			existing[key] = candidateValue
			result.Updated = append(result.Updated, key)
		}
	}

	if len(assignments) > 0 {
		if err := upsertEnvAssignments(envPath, assignments, resolver.GOOS); err != nil {
			return result, err
		}
		result.Created = !envExists
	} else if envExists {
		if err := chmodLocalEnvFile(envPath, resolver.GOOS); err != nil {
			return result, err
		}
	}

	result.Missing = missingInfisicalConfigKeys(existing)
	if !req.SkipVerify {
		status, err := s.SecretsStatus(ctx, SecretsStatusRequest{})
		if err != nil {
			return result, err
		}
		result.Status = status
		result.Verified = true
	}
	sort.Strings(result.Updated)
	sort.Strings(result.Preserved)
	sort.Strings(result.Missing)
	return result, nil
}

func (req SecretsConfigureInfisicalRequest) hasExplicitInfisicalValues() bool {
	return strings.TrimSpace(req.ProjectID) != "" || strings.TrimSpace(req.Environment) != "" || strings.TrimSpace(req.ClientID) != "" || strings.TrimSpace(req.ClientSecret) != "" || strings.TrimSpace(req.HostURL) != ""
}

func (req SecretsConfigureInfisicalRequest) infisicalEnvValues() (map[string]string, error) {
	projectID := strings.TrimSpace(req.ProjectID)
	environment := strings.TrimSpace(req.Environment)
	clientID := strings.TrimSpace(req.ClientID)
	clientSecret := strings.TrimSpace(req.ClientSecret)
	hostURL := strings.TrimSpace(req.HostURL)
	if environment == "" {
		environment = "dev"
	}
	fields := []struct {
		name     string
		label    string
		value    string
		required bool
	}{
		{name: "project ID", label: "Infisical project ID", value: projectID, required: true},
		{name: "environment", label: "Infisical environment", value: environment, required: true},
		{name: "client ID", label: "Infisical client ID", value: clientID, required: true},
		{name: "client secret", label: "Infisical client secret", value: clientSecret, required: true},
		{name: "host URL", label: "Infisical host/API URL", value: hostURL, required: false},
	}
	for _, field := range fields {
		if field.required && field.value == "" {
			return nil, fmt.Errorf("secrets configure infisical: %s is required", field.label)
		}
		if field.value != "" && containsControlRune(field.value) {
			return nil, fmt.Errorf("secrets configure infisical: %s contains unsupported control characters", field.label)
		}
	}
	if hostURL != "" {
		parsed, err := url.Parse(hostURL)
		if err != nil || parsed == nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return nil, fmt.Errorf("secrets configure infisical: Infisical host/API URL must be an absolute http(s) URL")
		}
		if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
			return nil, fmt.Errorf("secrets configure infisical: Infisical host/API URL must use HTTPS unless it points to localhost")
		}
	}
	values := map[string]string{
		"INFISICAL_AUTH_METHOD":   "universal-auth",
		"INFISICAL_PROJECT_ID":    projectID,
		"INFISICAL_ENV":           environment,
		"INFISICAL_CLIENT_ID":     clientID,
		"INFISICAL_CLIENT_SECRET": clientSecret,
	}
	values["INFISICAL_API_URL"] = hostURL
	values["INFISICAL_HOST"] = hostURL
	values["INFISICAL_HOST_URL"] = hostURL
	values["INFISICAL_TOKEN"] = ""
	return values, nil
}

func containsControlRune(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func validateInfisicalHostValues(values map[string]string) error {
	for _, key := range []string{"INFISICAL_API_URL", "INFISICAL_HOST", "INFISICAL_HOST_URL"} {
		value := strings.TrimSpace(values[key])
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed == nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return fmt.Errorf("secrets configure infisical: %s must be an absolute http(s) URL", key)
		}
		if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
			return fmt.Errorf("secrets configure infisical: %s must use HTTPS unless it points to localhost", key)
		}
	}
	return nil
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizeInfisicalAliases(values map[string]string) {
	copyAlias := func(canonical string, aliases ...string) {
		if strings.TrimSpace(values[canonical]) != "" {
			return
		}
		for _, alias := range aliases {
			if value := strings.TrimSpace(values[alias]); value != "" {
				values[canonical] = value
				return
			}
		}
	}
	copyAlias("INFISICAL_CLIENT_ID", "INFISICAL_UNIVERSAL_AUTH_CLIENT_ID")
	copyAlias("INFISICAL_CLIENT_SECRET", "INFISICAL_UNIVERSAL_AUTH_CLIENT_SECRET")
	copyAlias("INFISICAL_ENV", "INFISICAL_ENVIRONMENT")
}

func hasAnyInfisicalMachineConfig(values ...map[string]string) bool {
	keys := []string{"INFISICAL_TOKEN", "INFISICAL_AUTH_METHOD", "INFISICAL_CLIENT_ID", "INFISICAL_CLIENT_SECRET", "INFISICAL_PROJECT_ID"}
	for _, set := range values {
		for _, key := range keys {
			if strings.TrimSpace(set[key]) != "" {
				return true
			}
		}
	}
	return false
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
	if quoted, ok := parseQuotedEnvValue(value); ok {
		return key, quoted, true
	}
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return key, value, true
}

func parseQuotedEnvValue(value string) (string, bool) {
	if len(value) < 2 {
		return "", false
	}
	quote := value[0]
	if quote != '\'' && quote != '"' {
		return "", false
	}
	escaped := false
	for i := 1; i < len(value); i++ {
		ch := value[i]
		if quote == '"' && escaped {
			escaped = false
			continue
		}
		if quote == '"' && ch == '\\' {
			escaped = true
			continue
		}
		if ch == quote {
			quoted := value[1:i]
			if quote == '"' {
				quoted = unescapeDoubleQuotedEnvValue(quoted)
			}
			return quoted, true
		}
	}
	return "", false
}

func upsertEnvAssignments(path string, assignments map[string]string, goos string) error {
	if len(assignments) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	data, err := os.ReadFile(path)
	created := false
	if errors.Is(err, os.ErrNotExist) {
		created = true
		data = nil
	} else if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	lines := []string{}
	if len(data) > 0 {
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}
	out := make([]string, 0, len(lines)+len(assignments)+1)
	if created || len(lines) == 0 {
		out = append(out, "# Loki Infisical machine-auth configuration. Secret values stay local.")
	}
	seen := map[string]bool{}
	for _, line := range lines {
		key, _, ok := parseLocalEnvLine(line)
		if !ok {
			out = append(out, line)
			continue
		}
		value, shouldWrite := assignments[key]
		if !shouldWrite {
			out = append(out, line)
			continue
		}
		if seen[key] {
			continue
		}
		out = append(out, key+"="+formatEnvValue(value))
		seen[key] = true
	}
	for _, key := range infisicalEnvWriteOrder {
		value, ok := assignments[key]
		if !ok || seen[key] {
			continue
		}
		out = append(out, key+"="+formatEnvValue(value))
		seen[key] = true
	}
	content := strings.Join(out, "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return chmodLocalEnvFile(path, goos)
}

func chmodLocalEnvFile(path, goos string) error {
	if goos == "windows" {
		return nil
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}

func formatEnvValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, " \t#\r\n\"'\\") {
		return "\"" + escapeDoubleQuotedEnvValue(value) + "\""
	}
	return value
}

func escapeDoubleQuotedEnvValue(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"\n", "\\n",
		"\r", "\\r",
		"\t", "\\t",
	)
	return replacer.Replace(value)
}

func unescapeDoubleQuotedEnvValue(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' || i+1 >= len(value) {
			b.WriteByte(value[i])
			continue
		}
		i++
		switch value[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\', '"':
			b.WriteByte(value[i])
		default:
			b.WriteByte('\\')
			b.WriteByte(value[i])
		}
	}
	return b.String()
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
