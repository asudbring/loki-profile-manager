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
	Binary string
	Runner Runner
}

type CLIUnavailableError struct {
	Binary string
}

func (e CLIUnavailableError) Error() string {
	return fmt.Sprintf("Infisical CLI %q not found; install Infisical CLI, then run `loki secrets login`", e.Binary)
}

type AuthUnavailableError struct{}

func (e AuthUnavailableError) Error() string {
	return "Infisical CLI is not ready; run `loki secrets login`, then run `infisical init` or configure an Infisical project if needed"
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
	_, err := c.Runner.Run(ctx, c.Binary, []string{"secrets", "get", readinessProbeSecret, "--plain", "--silent"}, nil)
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
		status.Checks = append(status.Checks, secrets.Check{Severity: secrets.SeverityWarning, Code: "infisical.not_ready", Message: err.Error(), Remediation: "Run `loki secrets login`, then run `infisical init` or configure an Infisical project if needed."})
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
	values := map[string]string{}
	var missing []string
	for _, name := range unique {
		out, err := c.Runner.Run(ctx, c.Binary, []string{"secrets", "get", name, "--plain", "--silent"}, nil)
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
