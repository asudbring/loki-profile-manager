package infisical

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/asudbring/loki-profile-manager/internal/secrets"
)

type Runner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, name string, args []string, env []string) ([]byte, error)
}

type InteractiveRunner interface {
	RunInteractive(ctx context.Context, name string, args []string, env []string) error
}

type MachineTokenMinter interface {
	MintMachineToken(ctx context.Context, cfg Config) (string, error)
}

type EnvLookup func(key string) (string, bool)

type Config struct {
	Token        string
	ProjectID    string
	Environment  string
	AuthMethod   string
	ClientID     string
	ClientSecret string
	APIURL       string
	Host         string
}

type ExecRunner struct{}

func (ExecRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (ExecRunner) Run(ctx context.Context, name string, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.Output()
}

func (ExecRunner) RunInteractive(ctx context.Context, name string, args []string, env []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

const readinessProbeSecret = "__LOKI_INFISICAL_READINESS_PROBE__" // #nosec G101 -- sentinel secret name, not a credential value.

type Client struct {
	Binary      string
	Runner      Runner
	Config      Config
	LookupEnv   EnvLookup
	SanitizeEnv bool
}

type CLIUnavailableError struct {
	Binary string
}

func (e CLIUnavailableError) Error() string {
	return fmt.Sprintf("Infisical CLI %q not found; install Infisical CLI, then run `loki secrets login`", e.Binary)
}

type AuthUnavailableError struct{}

func (e AuthUnavailableError) Error() string {
	return "Infisical CLI is not ready; run `loki secrets login`, configure machine identity environment variables, then run `infisical init` or configure an Infisical project if needed"
}

type MachineAuthError struct{}

func (e MachineAuthError) Error() string {
	return "Infisical machine identity authentication failed"
}

type MissingSecretError struct {
	Names []string
}

func (e MissingSecretError) Error() string {
	return fmt.Sprintf("missing Infisical secret(s): %s", strings.Join(e.Names, ", "))
}

type SecretAccessError struct{}

func (e SecretAccessError) Error() string {
	return "Infisical secret read failed"
}

func NewClient(runner Runner) Client {
	return Client{Binary: "infisical", Runner: runner}
}

func (c Client) withDefaults() Client {
	if c.Binary == "" {
		c.Binary = "infisical"
	}
	if c.Runner == nil {
		c.Runner = ExecRunner{}
	}
	return c
}

func (c Client) lookup(key string) (string, bool) {
	if c.LookupEnv != nil {
		return c.LookupEnv(key)
	}
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return value, true
	}
	return lookupInfisicalEnvFile(key)
}

func lookupInfisicalEnvFile(key string) (string, bool) {
	path := defaultInfisicalEnvPath()
	if path == "" {
		return "", false
	}
	values, err := readInfisicalEnvFile(path)
	if err != nil {
		return "", false
	}
	value, ok := values[key]
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

func defaultInfisicalEnvPath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return DefaultEnvPathForHome(home)
}

func DefaultEnvPathForHome(home string) string {
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "infisical", ".env")
}

func readInfisicalEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := parseEnvLine(line)
		if ok {
			values[key] = value
		}
	}
	return values, nil
}

func parseEnvLine(line string) (string, string, bool) {
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
	if !secretNameRE.MatchString(key) {
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

func (c Client) resolvedConfig() Config {
	cfg := c.Config
	if cfg.Token == "" {
		cfg.Token = lookupFirst(c.lookup, "INFISICAL_TOKEN")
	}
	if cfg.ProjectID == "" {
		cfg.ProjectID = lookupFirst(c.lookup, "INFISICAL_PROJECT_ID")
	}
	if cfg.Environment == "" {
		cfg.Environment = lookupFirst(c.lookup, "INFISICAL_ENV", "INFISICAL_ENVIRONMENT")
	}
	if cfg.AuthMethod == "" {
		cfg.AuthMethod = lookupFirst(c.lookup, "INFISICAL_AUTH_METHOD")
	}
	if cfg.ClientID == "" {
		cfg.ClientID = lookupFirst(c.lookup, "INFISICAL_CLIENT_ID", "INFISICAL_UNIVERSAL_AUTH_CLIENT_ID")
	}
	if cfg.ClientSecret == "" {
		cfg.ClientSecret = lookupFirst(c.lookup, "INFISICAL_CLIENT_SECRET", "INFISICAL_UNIVERSAL_AUTH_CLIENT_SECRET")
	}
	if cfg.APIURL == "" {
		cfg.APIURL = lookupFirst(c.lookup, "INFISICAL_API_URL")
	}
	if cfg.Host == "" {
		cfg.Host = lookupFirst(c.lookup, "INFISICAL_HOST", "INFISICAL_HOST_URL")
	}
	if strings.TrimSpace(cfg.Host) != "" {
		cfg.APIURL = cfg.Host
	}
	return cfg
}

func lookupFirst(lookup EnvLookup, keys ...string) string {
	for _, key := range keys {
		if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (cfg Config) machineAuthConfigured() bool {
	if strings.TrimSpace(cfg.Token) != "" {
		return true
	}
	return cfg.universalAuthConfigured()
}

func (cfg Config) universalAuthConfigured() bool {
	if strings.EqualFold(strings.TrimSpace(cfg.AuthMethod), "universal-auth") {
		return strings.TrimSpace(cfg.ClientID) != "" && strings.TrimSpace(cfg.ClientSecret) != ""
	}
	return strings.TrimSpace(cfg.ClientID) != "" && strings.TrimSpace(cfg.ClientSecret) != ""
}

func (cfg Config) projectArgs() []string {
	projectID := strings.TrimSpace(cfg.ProjectID)
	if projectID == "" || !cfg.machineAuthConfigured() {
		return nil
	}
	return []string{"--projectId", projectID}
}

func (cfg Config) environmentArgs() []string {
	environment := strings.TrimSpace(cfg.Environment)
	if environment == "" {
		return nil
	}
	return []string{"--env", environment}
}

func (cfg Config) validateHostURLs() error {
	for _, value := range []string{cfg.APIURL, cfg.Host} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, err := universalAuthLoginEndpoint(value); err != nil {
			return err
		}
	}
	return nil
}

func (cfg Config) commandEnv() []string {
	var env []string
	appendEnv := func(key, value string) {
		if value != "" {
			env = append(env, key+"="+value)
		}
	}
	appendEnv("INFISICAL_TOKEN", cfg.Token)
	appendEnv("INFISICAL_API_URL", cfg.APIURL)
	appendEnv("INFISICAL_HOST", cfg.Host)
	return env
}

var infisicalAmbientEnvKeys = []string{
	"INFISICAL_TOKEN",
	"INFISICAL_AUTH_METHOD",
	"INFISICAL_CLIENT_ID",
	"INFISICAL_CLIENT_SECRET",
	"INFISICAL_UNIVERSAL_AUTH_CLIENT_ID",
	"INFISICAL_UNIVERSAL_AUTH_CLIENT_SECRET",
	"INFISICAL_PROJECT_ID",
	"INFISICAL_ENV",
	"INFISICAL_ENVIRONMENT",
	"INFISICAL_API_URL",
	"INFISICAL_HOST",
	"INFISICAL_HOST_URL",
}

func (c Client) commandEnvForConfig(cfg Config) []string {
	env := cfg.commandEnv()
	if !c.SanitizeEnv {
		return env
	}
	sanitized := make([]string, 0, len(infisicalAmbientEnvKeys)+len(env))
	for _, key := range infisicalAmbientEnvKeys {
		sanitized = append(sanitized, key+"=")
	}
	return append(sanitized, env...)
}

func (cfg Config) loginDomain() string {
	if strings.TrimSpace(cfg.APIURL) != "" {
		return strings.TrimSpace(cfg.APIURL)
	}
	return strings.TrimSpace(cfg.Host)
}

func (c Client) commandEnv(ctx context.Context, cfg Config) ([]string, Config, error) {
	if err := cfg.validateHostURLs(); err != nil {
		return nil, cfg, err
	}
	if strings.TrimSpace(cfg.Token) != "" {
		return c.commandEnvForConfig(cfg), cfg, nil
	}
	return c.commandEnvWithMachineToken(ctx, cfg)
}

func (c Client) commandEnvWithMachineToken(ctx context.Context, cfg Config) ([]string, Config, error) {
	if !cfg.universalAuthConfigured() {
		return c.commandEnvForConfig(cfg), cfg, nil
	}
	cfg.Token = ""
	token, err := c.mintMachineToken(ctx, cfg)
	if err != nil {
		return nil, cfg, err
	}
	cfg.Token = token
	return c.commandEnvForConfig(cfg), cfg, nil
}

func (c Client) shouldRetryWithMachineToken(cfg Config) bool {
	return strings.TrimSpace(cfg.Token) != "" && cfg.universalAuthConfigured()
}

func (c Client) mintMachineToken(ctx context.Context, cfg Config) (string, error) {
	if minter, ok := c.Runner.(MachineTokenMinter); ok {
		token, err := minter.MintMachineToken(ctx, cfg)
		if err != nil || strings.TrimSpace(token) == "" {
			return "", MachineAuthError{}
		}
		return strings.TrimSpace(token), nil
	}
	return mintMachineTokenHTTP(ctx, cfg)
}

func mintMachineTokenHTTP(ctx context.Context, cfg Config) (string, error) {
	endpoint, err := universalAuthLoginEndpoint(cfg.loginDomain())
	if err != nil {
		return "", MachineAuthError{}
	}
	body, err := json.Marshal(map[string]string{"clientId": cfg.ClientID, "clientSecret": cfg.ClientSecret})
	if err != nil {
		return "", MachineAuthError{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", MachineAuthError{}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", MachineAuthError{}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", MachineAuthError{}
	}
	var decoded struct {
		AccessToken string `json:"accessToken"`
		Token       string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", MachineAuthError{}
	}
	token := strings.TrimSpace(firstNonEmpty(decoded.AccessToken, decoded.Token))
	if token == "" {
		return "", MachineAuthError{}
	}
	return token, nil
}

func universalAuthLoginEndpoint(domain string) (string, error) {
	domain = strings.TrimRight(strings.TrimSpace(domain), "/")
	if domain == "" {
		domain = "https://app.infisical.com/api"
	}
	parsed, err := url.Parse(domain)
	if err != nil || parsed == nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", fmt.Errorf("invalid Infisical domain")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return "", fmt.Errorf("Infisical domain must use HTTPS unless it points to localhost")
	}
	if strings.HasSuffix(domain, "/api") {
		return domain + "/v1/auth/universal-auth/login", nil
	}
	return domain + "/api/v1/auth/universal-auth/login", nil
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (c Client) secretGetArgs(name string, cfg Config) []string {
	args := []string{"secrets", "get", name, "--plain", "--silent"}
	args = append(args, cfg.projectArgs()...)
	args = append(args, cfg.environmentArgs()...)
	return args
}

func (c Client) CheckInstalled(ctx context.Context) error {
	_ = ctx
	c = c.withDefaults()
	if _, err := c.Runner.LookPath(c.Binary); err != nil {
		return CLIUnavailableError{Binary: c.Binary}
	}
	return nil
}

func (c Client) ValidateConfig(ctx context.Context, cfg Config) error {
	c.Config = cfg
	c.LookupEnv = func(string) (string, bool) { return "", false }
	c.SanitizeEnv = true
	return c.CheckAuthenticated(ctx)
}

func (c Client) CheckAuthenticated(ctx context.Context) error {
	c = c.withDefaults()
	if err := c.CheckInstalled(ctx); err != nil {
		return err
	}
	cfg := c.resolvedConfig()
	env, cfg, err := c.commandEnv(ctx, cfg)
	if err != nil {
		return authErrorForConfig(err, cfg)
	}
	out, err := c.Runner.Run(ctx, c.Binary, c.secretGetArgs(readinessProbeSecret, cfg), env)
	if err == nil || secretGetMissing(readinessProbeSecret, err, out) {
		return nil
	}
	if c.shouldRetryWithMachineToken(cfg) {
		env, cfg, err = c.commandEnvWithMachineToken(ctx, cfg)
		if err != nil {
			return authErrorForConfig(err, cfg)
		}
		out, err = c.Runner.Run(ctx, c.Binary, c.secretGetArgs(readinessProbeSecret, cfg), env)
		if err == nil || secretGetMissing(readinessProbeSecret, err, out) {
			return nil
		}
	}
	if cfg.machineAuthConfigured() {
		return MachineAuthError{}
	}
	return AuthUnavailableError{}
}

func authErrorForConfig(err error, cfg Config) error {
	var machineErr MachineAuthError
	if errors.As(err, &machineErr) {
		return MachineAuthError{}
	}
	if cfg.machineAuthConfigured() {
		return MachineAuthError{}
	}
	return AuthUnavailableError{}
}

func (c Client) CheckStatus(ctx context.Context) secrets.Status {
	status := secrets.Status{Provider: secrets.ProviderInfisical, Checks: []secrets.Check{}}
	if err := c.CheckInstalled(ctx); err != nil {
		status.Checks = append(status.Checks, secrets.Check{Severity: secrets.SeverityWarning, Code: "infisical.cli_missing", Message: err.Error(), Remediation: "Install Infisical CLI, then run `loki secrets login`."})
		return status
	}
	status.CLIInstalled = true
	status.Checks = append(status.Checks, secrets.Check{Severity: secrets.SeverityInfo, Code: "infisical.cli_found", Message: "Infisical CLI is installed"})
	if err := c.CheckAuthenticated(ctx); err != nil {
		check := secrets.Check{Severity: secrets.SeverityWarning, Code: "infisical.not_ready", Message: err.Error(), Remediation: "Run `loki secrets login`, configure Infisical machine identity environment variables, then run `infisical init` or set INFISICAL_PROJECT_ID if needed."}
		var machineErr MachineAuthError
		if errors.As(err, &machineErr) {
			check.Code = "infisical.machine_auth_invalid"
			check.Message = MachineAuthError{}.Error()
			check.Remediation = machineAuthRemediation()
		}
		status.Checks = append(status.Checks, check)
		return status
	}
	status.Authenticated = true
	status.Ready = true
	status.Checks = append(status.Checks, secrets.Check{Severity: secrets.SeverityInfo, Code: "infisical.ready", Message: "Infisical CLI is ready for render templates"})
	return status
}

func machineAuthRemediation() string {
	envPath := defaultInfisicalEnvPath()
	if envPath == "" {
		return "Rerun `loki secrets configure infisical` with valid machine identity credentials, or remove the local Infisical env file to use interactive CLI login."
	}
	return fmt.Sprintf("Rerun `loki secrets configure infisical` with valid machine identity credentials, or remove %s to use interactive CLI login.", envPath)
}

func secretGetMissing(name string, err error, output []byte) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error()) + "\n" + strings.ToLower(string(output))
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		message += "\n" + strings.ToLower(string(exitErr.Stderr))
	}
	secretContext := strings.Contains(message, "secret") || strings.Contains(message, strings.ToLower(name))
	if !secretContext {
		return false
	}
	for _, needle := range []string{"not found", "not exist", "notfound", "could not find", "unable to find", "missing"} {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}

func (c Client) Login(ctx context.Context, req secrets.LoginRequest) error {
	c = c.withDefaults()
	if err := c.CheckInstalled(ctx); err != nil {
		return err
	}
	runner, ok := c.Runner.(InteractiveRunner)
	if !ok {
		return fmt.Errorf("infisical login: runner does not support interactive execution")
	}
	args := []string{"login"}
	if domain := strings.TrimSpace(req.Domain); domain != "" {
		args = append(args, "--domain", domain)
	}
	if err := runner.RunInteractive(ctx, c.Binary, args, nil); err != nil {
		return fmt.Errorf("infisical login failed: %w", err)
	}
	return nil
}

func (c Client) GetSecrets(ctx context.Context, names []string) (map[string]string, error) {
	c = c.withDefaults()
	if err := c.CheckInstalled(ctx); err != nil {
		return nil, err
	}
	unique, err := normalizeNames(names)
	if err != nil {
		return nil, err
	}
	cfg := c.resolvedConfig()
	env, cfg, err := c.commandEnv(ctx, cfg)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	var missing []string
	retriedMachineToken := false
	for _, name := range unique {
		out, err := c.Runner.Run(ctx, c.Binary, c.secretGetArgs(name, cfg), env)
		if err != nil {
			if secretGetMissing(name, err, out) {
				missing = append(missing, name)
				continue
			}
			if !retriedMachineToken && c.shouldRetryWithMachineToken(cfg) {
				retriedMachineToken = true
				env, cfg, err = c.commandEnvWithMachineToken(ctx, cfg)
				if err != nil {
					return values, err
				}
				out, err = c.Runner.Run(ctx, c.Binary, c.secretGetArgs(name, cfg), env)
				if err == nil {
					values[name] = strings.TrimRight(string(out), "\r\n")
					continue
				}
				if secretGetMissing(name, err, out) {
					missing = append(missing, name)
					continue
				}
			}
			return values, SecretAccessError{}
		}
		values[name] = strings.TrimRight(string(out), "\r\n")
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return values, MissingSecretError{Names: missing}
	}
	return values, nil
}

func (c Client) RunWithSecrets(ctx context.Context, command []string, extraEnv []string) ([]byte, error) {
	c = c.withDefaults()
	if err := c.CheckInstalled(ctx); err != nil {
		return nil, err
	}
	if len(command) == 0 {
		return nil, fmt.Errorf("infisical run: command is required")
	}
	cfg := c.resolvedConfig()
	env, cfg, err := c.commandEnv(ctx, cfg)
	if err != nil {
		return nil, err
	}
	env = append(env, extraEnv...)
	args := []string{"run"}
	args = append(args, cfg.projectArgs()...)
	args = append(args, cfg.environmentArgs()...)
	args = append(args, "--")
	args = append(args, command...)
	out, err := c.Runner.Run(ctx, c.Binary, args, env)
	if err == nil {
		return out, nil
	}
	if c.shouldRetryWithMachineToken(cfg) {
		env, cfg, err = c.commandEnvWithMachineToken(ctx, cfg)
		if err != nil {
			return nil, err
		}
		env = append(env, extraEnv...)
		args = []string{"run"}
		args = append(args, cfg.projectArgs()...)
		args = append(args, cfg.environmentArgs()...)
		args = append(args, "--")
		args = append(args, command...)
		out, err = c.Runner.Run(ctx, c.Binary, args, env)
		if err == nil {
			return out, nil
		}
	}
	return nil, fmt.Errorf("infisical run failed")
}

var secretNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func ValidateSecretName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("invalid Infisical secret name %q", name)
	}
	if !secretNameRE.MatchString(name) {
		return fmt.Errorf("invalid Infisical secret name %q", name)
	}
	return nil
}

func ValidateSecretNames(names []string) error {
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if err := ValidateSecretName(name); err != nil {
			return err
		}
	}
	return nil
}

func normalizeNames(names []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		if err := ValidateSecretName(name); err != nil {
			return nil, err
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func IsMissingSecret(err error) bool {
	var missing MissingSecretError
	return errors.As(err, &missing)
}
