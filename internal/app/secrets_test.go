package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/infisical"
	"github.com/allensu/loki-profile-manager/internal/secrets"
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

func secretsTestService(t *testing.T, opts Options) *Service {
	t.Helper()
	opts.Resolver = config.PathResolver{GOOS: "darwin", HomeDir: t.TempDir()}
	svc, err := NewService(context.Background(), opts)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}
