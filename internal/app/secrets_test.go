package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/infisical"
	"github.com/asudbring/loki-profile-manager/internal/secrets"
)

type fakeAppSecretProvider struct {
	values map[string]string
	err    error
}

func (f fakeAppSecretProvider) GetSecrets(ctx context.Context, names []string) (map[string]string, error) {
	out := map[string]string{}
	for _, name := range names {
		if value, ok := f.values[name]; ok {
			out[name] = value
		}
	}
	return out, f.err
}

type fakeAppSecretStatusChecker struct {
	status secrets.Status
}

func (f fakeAppSecretStatusChecker) CheckStatus(ctx context.Context) secrets.Status {
	return f.status
}

type fakeAppSecretLoginRunner struct {
	domains []string
	err     error
}

func (f *fakeAppSecretLoginRunner) Login(ctx context.Context, req secrets.LoginRequest) error {
	f.domains = append(f.domains, req.Domain)
	return f.err
}

func TestSecretsCheckListsNamesOnly(t *testing.T) {
	ctx := context.Background()
	svc := secretsTestService(t, Options{SecretProvider: fakeAppSecretProvider{values: map[string]string{"TOKEN": "dummy-secret-value", "PROJECT": "demo"}}})
	defer svc.Close()

	result, err := svc.SecretsCheck(ctx, SecretsCheckRequest{Names: []string{"TOKEN", "PROJECT", "TOKEN"}})
	if err != nil {
		t.Fatalf("SecretsCheck() error = %v", err)
	}
	if !result.Ready || !reflect.DeepEqual(result.Checked, []string{"PROJECT", "TOKEN"}) || !reflect.DeepEqual(result.Available, []string{"PROJECT", "TOKEN"}) || len(result.Missing) != 0 {
		t.Fatalf("result = %+v", result)
	}
	content, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatalf("Marshal() error = %v", marshalErr)
	}
	if strings.Contains(string(content), "dummy-secret-value") || strings.Contains(string(content), "demo") {
		t.Fatalf("result leaked secret values: %s", content)
	}
}

func TestSecretsCheckMissingListsNamesOnly(t *testing.T) {
	ctx := context.Background()
	svc := secretsTestService(t, Options{SecretProvider: fakeAppSecretProvider{values: map[string]string{"TOKEN": "dummy-secret-value"}, err: infisical.MissingSecretError{Names: []string{"PROJECT"}}}})
	defer svc.Close()

	result, err := svc.SecretsCheck(ctx, SecretsCheckRequest{Names: []string{"TOKEN", "PROJECT"}})
	if err == nil {
		t.Fatal("SecretsCheck() error = nil, want missing")
	}
	if result.Ready || !reflect.DeepEqual(result.Available, []string{"TOKEN"}) || !reflect.DeepEqual(result.Missing, []string{"PROJECT"}) {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(err.Error(), "dummy-secret-value") {
		t.Fatalf("error leaked secret value: %v", err)
	}
}

func TestSecretsStatusAndLoginUseInjectedBackends(t *testing.T) {
	ctx := context.Background()
	login := &fakeAppSecretLoginRunner{}
	svc := secretsTestService(t, Options{
		SecretStatusChecker: fakeAppSecretStatusChecker{status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: true, Ready: true}},
		SecretLoginRunner:   login,
	})
	defer svc.Close()

	status, err := svc.SecretsStatus(ctx, SecretsStatusRequest{})
	if err != nil {
		t.Fatalf("SecretsStatus() error = %v", err)
	}
	if !status.Ready || status.Provider != secrets.ProviderInfisical {
		t.Fatalf("status = %+v", status)
	}
	if err := svc.SecretsLogin(ctx, SecretsLoginRequest{Domain: "https://example.test"}); err != nil {
		t.Fatalf("SecretsLogin() error = %v", err)
	}
	if !reflect.DeepEqual(login.domains, []string{"https://example.test"}) {
		t.Fatalf("domains = %+v", login.domains)
	}
}

func TestSecretsLoginPropagatesErrorWithoutSecretValues(t *testing.T) {
	ctx := context.Background()
	login := &fakeAppSecretLoginRunner{err: errors.New("login failed")}
	svc := secretsTestService(t, Options{SecretLoginRunner: login})
	defer svc.Close()

	if err := svc.SecretsLogin(ctx, SecretsLoginRequest{}); err == nil || strings.Contains(err.Error(), "dummy-secret-value") {
		t.Fatalf("SecretsLogin() error = %v", err)
	}
}

func TestSecretsConfigureInfisicalWritesLocalEnvWithoutLeakingValues(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	env := map[string]string{
		"INFISICAL_CLIENT_ID":     "dummy-client-id",
		"INFISICAL_CLIENT_SECRET": "dummy-client-secret",
		"INFISICAL_PROJECT_ID":    "dummy-project-id",
		"INFISICAL_ENV":           "dev",
	}
	svc, err := NewService(ctx, Options{
		Resolver:            config.PathResolver{GOOS: "darwin", HomeDir: home, Env: func(key string) string { return env[key] }},
		SecretStatusChecker: fakeAppSecretStatusChecker{status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: true, Ready: true}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	result, err := svc.SecretsConfigureInfisical(ctx, SecretsConfigureInfisicalRequest{})
	if err != nil {
		t.Fatalf("SecretsConfigureInfisical() error = %v", err)
	}
	if !result.Created || !result.Status.Ready || len(result.Missing) != 0 {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(strings.Join(result.Updated, ","), "dummy-client-secret") {
		t.Fatalf("result leaked secret value: %+v", result)
	}
	content, err := os.ReadFile(filepath.Join(home, ".config", "infisical", ".env"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(content)
	for _, want := range []string{"INFISICAL_AUTH_METHOD=universal-auth", "INFISICAL_CLIENT_ID=dummy-client-id", "INFISICAL_CLIENT_SECRET=dummy-client-secret", "INFISICAL_PROJECT_ID=dummy-project-id", "INFISICAL_ENV=dev"} {
		if !strings.Contains(text, want) {
			t.Fatalf("env file missing %q:\n%s", want, text)
		}
	}
}

func TestSecretsConfigureInfisicalDoesNotPersistTokenFromEnvironment(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	env := map[string]string{
		"INFISICAL_TOKEN":      "dummy-token",
		"INFISICAL_PROJECT_ID": "project-123",
	}
	svc, err := NewService(ctx, Options{
		Resolver:            config.PathResolver{GOOS: "darwin", HomeDir: home, Env: func(key string) string { return env[key] }},
		SecretStatusChecker: fakeAppSecretStatusChecker{status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: true, Ready: true}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	result, err := svc.SecretsConfigureInfisical(ctx, SecretsConfigureInfisicalRequest{})
	if err != nil {
		t.Fatalf("SecretsConfigureInfisical() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(home, ".config", "infisical", ".env"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v result=%+v", err, result)
	}
	if strings.Contains(string(content), "INFISICAL_TOKEN") || strings.Contains(string(content), "dummy-token") {
		t.Fatalf("env file persisted token:\n%s", content)
	}
}

func TestSecretsConfigureInfisicalPreservesExistingAliasConfig(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	envPath := filepath.Join(home, ".config", "infisical", ".env")
	if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	envFile := "# local Infisical machine auth\n" +
		"export INFISICAL_AUTH_METHOD=universal-auth\n" +
		"INFISICAL_UNIVERSAL_AUTH_CLIENT_ID=file-client\n" +
		"INFISICAL_UNIVERSAL_AUTH_CLIENT_SECRET='file-x'\n" +
		"INFISICAL_PROJECT_ID=file-project\n" +
		"INFISICAL_ENVIRONMENT=stage\n" +
		"INFISICAL_HOST_URL=\"https://app.infisical.com\" # legacy alias\n"
	if err := os.WriteFile(envPath, []byte(envFile), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	svc, err := NewService(ctx, Options{
		Resolver:            config.PathResolver{GOOS: "darwin", HomeDir: home},
		SecretStatusChecker: fakeAppSecretStatusChecker{status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: true, Ready: true}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	result, err := svc.SecretsConfigureInfisical(ctx, SecretsConfigureInfisicalRequest{})
	if err != nil {
		t.Fatalf("SecretsConfigureInfisical() error = %v", err)
	}
	if len(result.Missing) != 0 || !result.Status.Ready || result.Created {
		t.Fatalf("result = %+v", result)
	}
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(content)
	if strings.Contains(text, "INFISICAL_ENV=dev") || strings.Contains(text, "INFISICAL_CLIENT_ID=file-client") || strings.Contains(text, "INFISICAL_CLIENT_SECRET=file-x") {
		t.Fatalf("legacy alias config was rewritten unexpectedly:\n%s", text)
	}
}

func TestSecretsConfigureInfisicalDoesNotClaimCreatedWhenNoValuesWritten(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{
		Resolver:            config.PathResolver{GOOS: "darwin", HomeDir: home},
		SecretStatusChecker: fakeAppSecretStatusChecker{status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: false, Ready: false}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	result, err := svc.SecretsConfigureInfisical(ctx, SecretsConfigureInfisicalRequest{})
	if err != nil {
		t.Fatalf("SecretsConfigureInfisical() error = %v", err)
	}
	if result.Created {
		t.Fatalf("Created = true with no values written: %+v", result)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".config", "infisical", ".env")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("env file created unexpectedly: %v", statErr)
	}
}

func TestParseLocalEnvLineStopsAtQuotedValueBeforeComment(t *testing.T) {
	_, got, ok := parseLocalEnvLine(`INFISICAL_CLIENT_SECRET="abc" # note "prod"`)
	if !ok || got != "abc" {
		t.Fatalf("parseLocalEnvLine() got %q ok=%v, want abc/true", got, ok)
	}
}

func TestFormatEnvValueRoundTripsQuotesAndHash(t *testing.T) {
	want := `has 'single' "double" # hash \path`
	key, got, ok := parseLocalEnvLine("KEY=" + formatEnvValue(want))
	if !ok || key != "KEY" || got != want {
		t.Fatalf("round trip key=%q got=%q ok=%v formatted=%s", key, got, ok, formatEnvValue(want))
	}
}

func TestSecretsConfigureInfisicalExplicitRequestOverwritesLocalConfigAndOmitsToken(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	envPath := filepath.Join(home, ".config", "infisical", ".env")
	if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(envPath, []byte("# existing local config\nINFISICAL_CLIENT_ID=old-client\nINFISICAL_API_URL=https://old-api.example\nINFISICAL_HOST=https://old-host.example\nUNKNOWN_KEY=keep-me\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	env := map[string]string{"INFISICAL_TOKEN": "dummy-env-token"}
	svc, err := NewService(ctx, Options{
		Resolver:            config.PathResolver{GOOS: "darwin", HomeDir: home, Env: func(key string) string { return env[key] }},
		SecretStatusChecker: fakeAppSecretStatusChecker{status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: true, Ready: true}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	result, err := svc.SecretsConfigureInfisical(ctx, SecretsConfigureInfisicalRequest{
		ProjectID:         "project-123",
		Environment:       "prod",
		ClientID:          "client-123",
		ClientSecret:      "dummy-client-secret",
		HostURL:           "https://infisical.example",
		OverwriteExisting: true,
	})
	if err != nil {
		t.Fatalf("SecretsConfigureInfisical() error = %v", err)
	}
	if result.Created || !result.Status.Ready || len(result.Missing) != 0 {
		t.Fatalf("result = %+v", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), "dummy-client-secret") || strings.Contains(string(encoded), "client-123") {
		t.Fatalf("result leaked configured values: %s", encoded)
	}
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(content)
	for _, want := range []string{"INFISICAL_AUTH_METHOD=universal-auth", "INFISICAL_CLIENT_ID=client-123", "INFISICAL_CLIENT_SECRET=dummy-client-secret", "INFISICAL_PROJECT_ID=project-123", "INFISICAL_ENV=prod", "INFISICAL_API_URL=https://infisical.example", "INFISICAL_HOST=https://infisical.example", "INFISICAL_HOST_URL=https://infisical.example", "UNKNOWN_KEY=keep-me"} {
		if !strings.Contains(text, want) {
			t.Fatalf("env file missing %q:\n%s", want, text)
		}
	}
	for _, leak := range []string{"old-client", "old-api.example", "old-host.example", "INFISICAL_TOKEN", "dummy-env-token"} {
		if strings.Contains(text, leak) {
			t.Fatalf("env file leaked or preserved forbidden value %q:\n%s", leak, text)
		}
	}
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("env file mode = %v, want 0600", got)
	}
}

func TestSecretsConfigureInfisicalExplicitBlankHostClearsExistingHost(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	envPath := filepath.Join(home, ".config", "infisical", ".env")
	if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(envPath, []byte("INFISICAL_TOKEN=old-token\nINFISICAL_API_URL=https://old-api.example\nINFISICAL_HOST=https://old-host.example\nINFISICAL_HOST_URL=https://old-host-url.example\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	svc, err := NewService(ctx, Options{
		Resolver:            config.PathResolver{GOOS: "darwin", HomeDir: home},
		SecretStatusChecker: fakeAppSecretStatusChecker{status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: true, Ready: true}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	_, err = svc.SecretsConfigureInfisical(ctx, SecretsConfigureInfisicalRequest{ProjectID: "project-123", Environment: "dev", ClientID: "client-123", ClientSecret: "client-secret", OverwriteExisting: true, SkipVerify: true})
	if err != nil {
		t.Fatalf("SecretsConfigureInfisical() error = %v", err)
	}
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(content)
	for _, leak := range []string{"old-token", "old-api.example", "old-host.example", "old-host-url.example"} {
		if strings.Contains(text, leak) {
			t.Fatalf("old host value preserved %q:\n%s", leak, text)
		}
	}
	for _, want := range []string{"INFISICAL_TOKEN=", "INFISICAL_API_URL=", "INFISICAL_HOST=", "INFISICAL_HOST_URL="} {
		if !strings.Contains(text, want) {
			t.Fatalf("env file missing cleared host key %q:\n%s", want, text)
		}
	}
}

func TestSecretsConfigureInfisicalRejectsDiscoveredPlainHTTPHost(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	env := map[string]string{
		"INFISICAL_CLIENT_ID":     "client-123",
		"INFISICAL_CLIENT_SECRET": "client-secret",
		"INFISICAL_PROJECT_ID":    "project-123",
		"INFISICAL_HOST":          "http://infisical.example",
	}
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home, Env: func(key string) string { return env[key] }}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	_, err = svc.SecretsConfigureInfisical(ctx, SecretsConfigureInfisicalRequest{})
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("SecretsConfigureInfisical() error = %v, want HTTPS validation", err)
	}
}

func TestSecretsConfigureInfisicalRejectsPlainHTTPHost(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	_, err = svc.SecretsConfigureInfisical(ctx, SecretsConfigureInfisicalRequest{ProjectID: "project-123", ClientID: "client-123", ClientSecret: "client-secret", HostURL: "http://infisical.example", OverwriteExisting: true})
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("SecretsConfigureInfisical() error = %v, want HTTPS validation", err)
	}
}

func TestSecretsConfigureInfisicalExplicitRequestRejectsUnsafeValuesWithoutLeaking(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	_, err = svc.SecretsConfigureInfisical(ctx, SecretsConfigureInfisicalRequest{
		ProjectID:         "project-123",
		ClientID:          "client-123",
		ClientSecret:      "dummy-client-secret\nsecond-line",
		OverwriteExisting: true,
	})
	if err == nil {
		t.Fatal("SecretsConfigureInfisical() error = nil, want validation error")
	}
	if strings.Contains(err.Error(), "dummy-client-secret") || strings.Contains(err.Error(), "second-line") {
		t.Fatalf("validation error leaked secret value: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".config", "infisical", ".env")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("env file created after validation failure: %v", statErr)
	}
}

func TestSecretsConfigureInfisicalReadsProjectConfig(t *testing.T) {
	ctx := context.Background()
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
	if err := os.WriteFile(filepath.Join(work, ".infisical.json"), []byte(`{"workspaceId":"project-from-config","defaultEnvironment":"prod"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	env := map[string]string{
		"INFISICAL_CLIENT_ID":     "dummy-client-id",
		"INFISICAL_CLIENT_SECRET": "dummy-client-secret",
	}
	svc, err := NewService(ctx, Options{
		Resolver:            config.PathResolver{GOOS: "darwin", HomeDir: home, Env: func(key string) string { return env[key] }},
		SecretStatusChecker: fakeAppSecretStatusChecker{status: secrets.Status{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: true, Ready: true}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	result, err := svc.SecretsConfigureInfisical(ctx, SecretsConfigureInfisicalRequest{})
	if err != nil {
		t.Fatalf("SecretsConfigureInfisical() error = %v", err)
	}
	if len(result.Missing) != 0 {
		t.Fatalf("missing = %+v", result.Missing)
	}
	content, err := os.ReadFile(filepath.Join(home, ".config", "infisical", ".env"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "INFISICAL_PROJECT_ID=project-from-config") || !strings.Contains(text, "INFISICAL_ENV=prod") {
		t.Fatalf("env file = %s", text)
	}
}

func secretsTestService(t *testing.T, opts Options) *Service {
	t.Helper()
	opts.Resolver = config.PathResolver{GOOS: "darwin", HomeDir: t.TempDir()}
	svc, err := NewService(context.Background(), opts)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}
