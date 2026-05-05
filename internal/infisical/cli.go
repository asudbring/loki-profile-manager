package infisical

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/allensu/loki-profile-manager/internal/secrets"
)

type Runner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, name string, args []string, env []string) ([]byte, error)
}

type InteractiveRunner interface {
	RunInteractive(ctx context.Context, name string, args []string, env []string) error
}

type EnvLookup func(key string) (string, bool)

type Config struct {
	Token        string
	ProjectID    string
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

const readinessProbeSecret = "__LOKI_INFISICAL_READINESS_PROBE__"

type Client struct {
	Binary    string
	Runner    Runner
	Config    Config
	LookupEnv EnvLookup
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
	return os.LookupEnv(key)
}

func (c Client) resolvedConfig() Config {
	cfg := c.Config
	if cfg.Token == "" {
		cfg.Token = lookupFirst(c.lookup, "INFISICAL_TOKEN")
	}
	if cfg.ProjectID == "" {
		cfg.ProjectID = lookupFirst(c.lookup, "INFISICAL_PROJECT_ID")
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
		cfg.Host = lookupFirst(c.lookup, "INFISICAL_HOST")
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

func (cfg Config) loginDomain() string {
	if strings.TrimSpace(cfg.APIURL) != "" {
		return strings.TrimSpace(cfg.APIURL)
	}
	return strings.TrimSpace(cfg.Host)
}

func (c Client) commandEnv(ctx context.Context, cfg Config) ([]string, Config, error) {
	if strings.TrimSpace(cfg.Token) != "" {
		return cfg.commandEnv(), cfg, nil
	}
	if !cfg.universalAuthConfigured() {
		return cfg.commandEnv(), cfg, nil
	}
	token, err := c.mintMachineToken(ctx, cfg)
	if err != nil {
		return nil, cfg, err
	}
	cfg.Token = token
	return cfg.commandEnv(), cfg, nil
}

func (c Client) mintMachineToken(ctx context.Context, cfg Config) (string, error) {
	args := []string{"login", "--method=universal-auth", "--client-id", cfg.ClientID, "--client-secret", cfg.ClientSecret}
	if domain := cfg.loginDomain(); domain != "" {
		args = append(args, "--domain", domain)
	}
	args = append(args, "--plain", "--silent")
	out, err := c.Runner.Run(ctx, c.Binary, args, cfg.commandEnv())
	if err != nil {
		return "", MachineAuthError{}
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", MachineAuthError{}
	}
	return token, nil
}

func (c Client) secretGetArgs(name string, cfg Config) []string {
	args := []string{"secrets", "get", name, "--plain", "--silent"}
	args = append(args, cfg.projectArgs()...)
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

func (c Client) CheckAuthenticated(ctx context.Context) error {
	c = c.withDefaults()
	if err := c.CheckInstalled(ctx); err != nil {
		return err
	}
	cfg := c.resolvedConfig()
	env, cfg, err := c.commandEnv(ctx, cfg)
	if err != nil {
		return AuthUnavailableError{}
	}
	_, err = c.Runner.Run(ctx, c.Binary, c.secretGetArgs(readinessProbeSecret, cfg), env)
	if err == nil || readinessProbeMissing(err) {
		return nil
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
		status.Checks = append(status.Checks, secrets.Check{Severity: secrets.SeverityWarning, Code: "infisical.not_ready", Message: err.Error(), Remediation: "Run `loki secrets login`, configure Infisical machine identity environment variables, then run `infisical init` or set INFISICAL_PROJECT_ID if needed."})
		return status
	}
	status.Authenticated = true
	status.Ready = true
	status.Checks = append(status.Checks, secrets.Check{Severity: secrets.SeverityInfo, Code: "infisical.ready", Message: "Infisical CLI is ready for render templates"})
	return status
}

func readinessProbeMissing(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		message += "\n" + strings.ToLower(string(exitErr.Stderr))
	}
	secretContext := strings.Contains(message, "secret") || strings.Contains(message, strings.ToLower(readinessProbeSecret))
	if !secretContext {
		return false
	}
	for _, needle := range []string{"not found", "not exist", "notfound", "could not find", "unable to find"} {
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
	for _, name := range unique {
		out, err := c.Runner.Run(ctx, c.Binary, c.secretGetArgs(name, cfg), env)
		value := strings.TrimRight(string(out), "\r\n")
		if err != nil || value == "" {
			missing = append(missing, name)
			continue
		}
		values[name] = value
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
	args = append(args, "--")
	args = append(args, command...)
	out, err := c.Runner.Run(ctx, c.Binary, args, env)
	if err != nil {
		return nil, fmt.Errorf("infisical run failed")
	}
	return out, nil
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
