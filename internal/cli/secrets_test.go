package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/infisical"
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

type fakeCLIInfisicalConfigValidator struct {
	configs []infisical.Config
	err     error
}

func (f *fakeCLIInfisicalConfigValidator) ValidateConfig(ctx context.Context, cfg infisical.Config) error {
	_ = ctx
	f.configs = append(f.configs, cfg)
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

func TestSecretsStatusReportsInvalidMachineAuthState(t *testing.T) {
	status := secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Ready: false, Checks: []secrets.Check{{Severity: secrets.SeverityWarning, Code: "infisical.machine_auth_invalid", Message: "Infisical machine identity authentication failed", Remediation: "Rerun `loki secrets configure infisical`, or remove the local Infisical env file to use CLI login."}}}
	cmd, out, _ := secretsTestCommand(t, app.Options{SecretStatusChecker: fakeCLISecretStatusChecker{status: status}})
	cmd.SetArgs([]string{"secrets", "status"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("secrets status error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Auth: machine identity invalid") || !strings.Contains(got, "Next step: Rerun `loki secrets configure infisical`") {
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

func TestSecretsConfigureInfisicalWizardPromptsWritesConfigAndHidesValues(t *testing.T) {
	home := t.TempDir()
	validator := &fakeCLIInfisicalConfigValidator{}
	cmd, out, errOut := secretsTestCommandWithResolver(t,
		app.Options{SecretStatusChecker: fakeCLISecretStatusChecker{status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: true, Ready: true}}, InfisicalConfigValidator: validator},
		config.PathResolver{GOOS: "darwin", HomeDir: home},
	)
	cmd.SetArgs([]string{"secrets", "configure", "infisical"})
	cmd.SetIn(strings.NewReader("project-123\n\nclient-123\ndummy-client-secret\nhttps://infisical.example\ny\n"))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("secrets configure infisical error = %v", err)
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "Loki Infisical configuration") || !strings.Contains(combined, "INFISICAL_CLIENT_SECRET") || !strings.Contains(combined, "Verification: skipped") {
		t.Fatalf("wizard output = %s", combined)
	}
	for _, leak := range []string{"dummy-client-secret", "client-123", "project-123"} {
		if strings.Contains(combined, leak) {
			t.Fatalf("wizard output leaked value %q: %s", leak, combined)
		}
	}
	content, err := os.ReadFile(filepath.Join(home, ".config", "infisical", ".env"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(content)
	for _, want := range []string{"INFISICAL_PROJECT_ID=project-123", "INFISICAL_ENV=dev", "INFISICAL_CLIENT_ID=client-123", "INFISICAL_CLIENT_SECRET=dummy-client-secret", "INFISICAL_HOST_URL=https://infisical.example"} {
		if !strings.Contains(text, want) {
			t.Fatalf("env file missing %q:\n%s", want, text)
		}
	}
	if len(validator.configs) != 1 || validator.configs[0].ProjectID != "project-123" || validator.configs[0].ClientID != "client-123" {
		t.Fatalf("validated configs = %+v", validator.configs)
	}
}

func TestSecretsConfigureInfisicalWizardValidationFailureDoesNotWriteOrLeak(t *testing.T) {
	home := t.TempDir()
	envPath := filepath.Join(home, ".config", "infisical", ".env")
	if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := "INFISICAL_CLIENT_ID=old-client\n"
	if err := os.WriteFile(envPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd, out, errOut := secretsTestCommandWithResolver(t,
		app.Options{InfisicalConfigValidator: &fakeCLIInfisicalConfigValidator{err: infisical.MachineAuthError{}}},
		config.PathResolver{GOOS: "darwin", HomeDir: home},
	)
	cmd.SetArgs([]string{"secrets", "configure", "infisical"})
	cmd.SetIn(strings.NewReader("project-123\n\nclient-123\ndummy-client-secret\nhttps://infisical.example\ny\n"))
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "validation") {
		t.Fatalf("secrets configure infisical error = %v", err)
	}
	combined := out.String() + errOut.String()
	for _, leak := range []string{"dummy-client-secret", "client-123", "project-123"} {
		if strings.Contains(combined, leak) {
			t.Fatalf("wizard output leaked %q: %s", leak, combined)
		}
	}
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != original {
		t.Fatalf("env file changed after validation failure:\n%s", content)
	}
}

func TestSecretsConfigureInfisicalCommandAppearsInHelp(t *testing.T) {
	cmd, out, _ := secretsTestCommand(t, app.Options{})
	cmd.SetArgs([]string{"secrets", "configure", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("secrets configure --help error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "infisical") || !strings.Contains(got, "interactive") {
		t.Fatalf("help output = %s", got)
	}
}

func TestSecretsInfisicalAllowsReadyCliLoginWithProjectConfigOnly(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer os.Chdir(oldWD)
	if err := os.Chdir(work); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(work, ".infisical.json"), []byte(`{"workspaceId":"project-from-config","defaultEnvironment":"dev"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd, out, _ := secretsTestCommandWithResolver(t, app.Options{SecretStatusChecker: fakeCLISecretStatusChecker{status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: true, Ready: true}}}, config.PathResolver{GOOS: "darwin", HomeDir: home})
	cmd.SetArgs([]string{"secrets", "--infisical"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("secrets --infisical error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Auth: authenticated") || strings.Contains(got, "Missing keys:") {
		t.Fatalf("output = %s", got)
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
