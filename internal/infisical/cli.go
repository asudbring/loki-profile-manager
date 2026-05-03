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
)

type Runner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, name string, args []string, env []string) ([]byte, error)
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

type Client struct {
	Binary string
	Runner Runner
}

type CLIUnavailableError struct {
	Binary string
}

func (e CLIUnavailableError) Error() string {
	return fmt.Sprintf("Infisical CLI %q not found; install and authenticate with infisical before rendering templates", e.Binary)
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
		out, err := c.Runner.Run(ctx, c.Binary, []string{"run", "--", "printenv", name}, nil)
		value := strings.TrimRight(string(out), "\r\n")
		if err != nil || value == "" {
			missing = append(missing, name)
			continue
		}
		values[name] = value
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, MissingSecretError{Names: missing}
	}
	return values, nil
}

var secretNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func normalizeNames(names []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		if !secretNameRE.MatchString(name) {
			return nil, fmt.Errorf("invalid Infisical secret name %q", name)
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
