package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/secrets"
)

type fakeCLISecretProvider struct {
	values map[string]string
	err    error
}

func (f fakeCLISecretProvider) GetSecrets(ctx context.Context, names []string) (map[string]string, error) {
	out := map[string]string{}
	for _, name := range names {
		if value, ok := f.values[name]; ok {
			out[name] = value
		}
	}
	return out, f.err
}

type fakeCLISecretStatusChecker struct {
	status secrets.Status
}

func (f fakeCLISecretStatusChecker) CheckStatus(ctx context.Context) secrets.Status {
	return f.status
}

type fakeCLISecretLoginRunner struct {
	domains []string
	err     error
}

func (f *fakeCLISecretLoginRunner) Login(ctx context.Context, req secrets.LoginRequest) error {
	f.domains = append(f.domains, req.Domain)
	return f.err
}

func TestSecretsStatusJSON(t *testing.T) {
	status := secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: true, Ready: true}
	cmd, out, _ := secretsTestCommand(t, app.Options{SecretStatusChecker: fakeCLISecretStatusChecker{status: status}})
	cmd.SetArgs([]string{"secrets", "status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("secrets status error = %v", err)
	}
	var result app.SecretsStatusResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("status JSON invalid: %v\n%s", err, out.String())
	}
	if !result.Ready || !result.CLIInstalled || result.Provider != secrets.ProviderInfisical {
		t.Fatalf("result = %+v", result)
	}
}

func TestSecretsStatusNotReadyReturnsErrorAfterOutput(t *testing.T) {
	status := secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Ready: false, Checks: []secrets.Check{{Severity: secrets.SeverityWarning, Remediation: "run login"}}}
	cmd, out, _ := secretsTestCommand(t, app.Options{SecretStatusChecker: fakeCLISecretStatusChecker{status: status}})
	cmd.SetArgs([]string{"secrets", "status"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("secrets status error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Loki secrets") || !strings.Contains(got, "Next step: run login") {
		t.Fatalf("status output = %s", got)
	}
}

func TestSecretsCheckHumanOutputHidesValues(t *testing.T) {
	cmd, out, _ := secretsTestCommand(t, app.Options{SecretProvider: fakeCLISecretProvider{values: map[string]string{"TOKEN": "dummy-secret-value"}}})
	cmd.SetArgs([]string{"secrets", "check", "TOKEN"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("secrets check error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "TOKEN") || strings.Contains(got, "dummy-secret-value") {
		t.Fatalf("check output = %s", got)
	}
}

func TestSecretsLoginUsesDomain(t *testing.T) {
	login := &fakeCLISecretLoginRunner{}
	cmd, out, _ := secretsTestCommand(t, app.Options{SecretLoginRunner: login})
	cmd.SetArgs([]string{"secrets", "login", "--domain", "https://example.test"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("secrets login error = %v", err)
	}
	if !reflect.DeepEqual(login.domains, []string{"https://example.test"}) {
		t.Fatalf("domains = %+v", login.domains)
	}
	if !strings.Contains(out.String(), "Infisical login command completed") {
		t.Fatalf("login output = %s", out.String())
	}
}

func TestSecretsCheckRequiresNames(t *testing.T) {
	cmd, _, _ := secretsTestCommand(t, app.Options{})
	cmd.SetArgs([]string{"secrets", "check"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("secrets check error = nil, want arg error")
	}
}

func TestSecretsInfisicalConfiguresEnvAndHidesValues(t *testing.T) {
	env := map[string]string{
		"INFISICAL_CLIENT_ID":     "dummy-client-id",
		"INFISICAL_CLIENT_SECRET": "dummy-client-secret",
		"INFISICAL_PROJECT_ID":    "dummy-project-id",
	}
	cmd, out, _ := secretsTestCommandWithResolver(t, app.Options{SecretStatusChecker: fakeCLISecretStatusChecker{status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: true, Ready: true}}}, config.PathResolver{GOOS: "darwin", HomeDir: t.TempDir(), Env: func(key string) string { return env[key] }})
	cmd.SetArgs([]string{"secrets", "--infisical"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("secrets --infisical error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Loki Infisical configuration") || !strings.Contains(got, "INFISICAL_CLIENT_SECRET") {
		t.Fatalf("output = %s", got)
	}
	if strings.Contains(got, "dummy-client-secret") || strings.Contains(got, "dummy-client-id") {
		t.Fatalf("output leaked secret values: %s", got)
	}
}

func secretsTestCommand(t *testing.T, appOpts app.Options) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	return secretsTestCommandWithResolver(t, appOpts, config.PathResolver{GOOS: "darwin", HomeDir: t.TempDir()})
}

func secretsTestCommandWithResolver(t *testing.T, appOpts app.Options, resolver config.PathResolver) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	factory := func(ctx context.Context, opts app.Options) (*app.Service, error) {
		merged := appOpts
		merged.Resolver = opts.Resolver
		merged.StoreOverride = opts.StoreOverride
		merged.Verbose = opts.Verbose
		merged.Stderr = opts.Stderr
		return app.NewService(ctx, merged)
	}
	cmd := NewRootCommand(Options{Resolver: resolver, Out: out, Err: errOut, Factory: factory})
	return cmd, out, errOut
}
