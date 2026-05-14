package infisical

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/asudbring/loki-profile-manager/internal/secrets"
)

type fakeRunner struct {
	missingBinary       bool
	values              map[string]string
	runErr              error
	runOutput           string
	machineToken        string
	failStaleToken      bool
	interactiveErr      error
	commands            [][]string
	envs                [][]string
	mintedConfigs       []Config
	interactiveCommands [][]string
}

func (f *fakeRunner) LookPath(file string) (string, error) {
	if f.missingBinary {
		return "", errors.New("not found")
	}
	return "/usr/bin/" + file, nil
}

func (f *fakeRunner) Run(ctx context.Context, name string, args []string, env []string) ([]byte, error) {
	f.commands = append(f.commands, append([]string{name}, args...))
	f.envs = append(f.envs, append([]string{}, env...))
	if f.failStaleToken && hasEnv(env, "INFISICAL_TOKEN=stale-token") {
		return []byte("token-value secret-value"), errors.New("unauthorized token-value secret-value")
	}
	if f.runErr != nil {
		return []byte(f.runOutput), f.runErr
	}
	if len(args) > 0 && args[0] == "login" {
		if f.machineToken != "" {
			return []byte(f.machineToken), nil
		}
		return []byte("machine-token\n"), nil
	}
	if len(args) > 0 && args[0] == "run" {
		if f.runOutput != "" {
			return []byte(f.runOutput), nil
		}
		return []byte("infisical version test\n"), nil
	}
	if len(args) >= 3 && args[0] == "secrets" && args[1] == "get" {
		key := args[2]
		if key == readinessProbeSecret {
			return nil, errors.New("secret not found")
		}
		value, ok := f.values[key]
		if !ok {
			return nil, errors.New("secret not found")
		}
		return []byte(value + "\n"), nil
	}
	return nil, errors.New("unexpected command")
}

func (f *fakeRunner) RunInteractive(ctx context.Context, name string, args []string, env []string) error {
	f.interactiveCommands = append(f.interactiveCommands, append([]string{name}, args...))
	return f.interactiveErr
}

func (f *fakeRunner) MintMachineToken(ctx context.Context, cfg Config) (string, error) {
	_ = ctx
	f.mintedConfigs = append(f.mintedConfigs, cfg)
	if f.runErr != nil {
		return "", f.runErr
	}
	if f.machineToken != "" {
		return f.machineToken, nil
	}
	return "machine-token\n", nil
}

func testClient(runner *fakeRunner) Client {
	return Client{Runner: runner, LookupEnv: func(string) (string, bool) { return "", false }}
}

func mapLookup(values map[string]string) EnvLookup {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func hasEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

func TestClientGetSecretsUsesRunnerAndHidesValuesOnMissing(t *testing.T) {
	runner := &fakeRunner{values: map[string]string{"TOKEN": "secret-value", "PROJECT": "demo"}}
	client := testClient(runner)
	values, err := client.GetSecrets(context.Background(), []string{"TOKEN", "PROJECT", "TOKEN"})
	if err != nil {
		t.Fatalf("GetSecrets() error = %v", err)
	}
	if values["TOKEN"] != "secret-value" || values["PROJECT"] != "demo" {
		t.Fatalf("values = %+v", values)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("commands = %+v", runner.commands)
	}
	for _, command := range runner.commands {
		if len(command) < 6 || command[1] != "secrets" || command[2] != "get" || command[4] != "--plain" || command[5] != "--silent" {
			t.Fatalf("unexpected command = %+v", command)
		}
	}

	partial, err := client.GetSecrets(context.Background(), []string{"TOKEN", "MISSING"})
	if err == nil {
		t.Fatal("GetSecrets() error = nil, want missing")
	}
	if partial["TOKEN"] != "secret-value" {
		t.Fatalf("partial values = %+v", partial)
	}
	if !strings.Contains(err.Error(), "MISSING") || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("missing error leaked value or hid name: %v", err)
	}
}

func TestClientGetSecretsAllowsEmptyValues(t *testing.T) {
	runner := &fakeRunner{values: map[string]string{"EMPTY": ""}}
	client := testClient(runner)
	values, err := client.GetSecrets(context.Background(), []string{"EMPTY"})
	if err != nil {
		t.Fatalf("GetSecrets() error = %v", err)
	}
	value, ok := values["EMPTY"]
	if !ok || value != "" {
		t.Fatalf("values = %+v, want EMPTY present with empty value", values)
	}
}

func TestClientGetSecretsReadErrorDoesNotLeakValues(t *testing.T) {
	runner := &fakeRunner{runErr: errors.New("permission denied token-value secret-value"), runOutput: "secret-value"}
	client := testClient(runner)
	_, err := client.GetSecrets(context.Background(), []string{"TOKEN"})
	if err == nil {
		t.Fatal("GetSecrets() error = nil, want read error")
	}
	if IsMissingSecret(err) {
		t.Fatalf("GetSecrets() error = %v, want access error", err)
	}
	for _, leaked := range []string{"token-value", "secret-value"} {
		if strings.Contains(err.Error(), leaked) {
			t.Fatalf("error leaked %q: %v", leaked, err)
		}
	}
}

func TestClientCheckInstalledMissingCLI(t *testing.T) {
	client := testClient(&fakeRunner{missingBinary: true})
	if err := client.CheckInstalled(context.Background()); err == nil {
		t.Fatal("CheckInstalled() error = nil, want error")
	}
}

func TestClientCheckAuthenticatedUsesReadinessProbeWithoutLeakingOutput(t *testing.T) {
	runner := &fakeRunner{runErr: errors.New("auth failed"), runOutput: "dummy-sensitive-output"}
	client := testClient(runner)
	err := client.CheckAuthenticated(context.Background())
	if err == nil {
		t.Fatal("CheckAuthenticated() error = nil, want error")
	}
	if strings.Contains(err.Error(), "dummy-sensitive-output") {
		t.Fatalf("auth error leaked command output: %v", err)
	}
	want := []string{"infisical", "secrets", "get", readinessProbeSecret, "--plain", "--silent"}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0], want) {
		t.Fatalf("commands = %+v, want %+v", runner.commands, want)
	}
}

func TestClientCheckStatusReadyAndNotReady(t *testing.T) {
	ready := testClient(&fakeRunner{})
	status := ready.CheckStatus(context.Background())
	if !status.CLIInstalled || !status.Authenticated || !status.Ready {
		t.Fatalf("ready status = %+v", status)
	}

	missing := testClient(&fakeRunner{missingBinary: true})
	status = missing.CheckStatus(context.Background())
	if status.CLIInstalled || status.Ready || len(status.Checks) == 0 || status.Checks[0].Severity != secrets.SeverityWarning {
		t.Fatalf("missing status = %+v", status)
	}
}

func TestClientLoginUsesInteractiveRunner(t *testing.T) {
	runner := &fakeRunner{}
	client := testClient(runner)
	if err := client.Login(context.Background(), secrets.LoginRequest{Domain: "https://example.test"}); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	want := []string{"infisical", "login", "--domain", "https://example.test"}
	if len(runner.interactiveCommands) != 1 || !reflect.DeepEqual(runner.interactiveCommands[0], want) {
		t.Fatalf("interactive commands = %+v, want %+v", runner.interactiveCommands, want)
	}
}

func TestClientGetSecretsWithTokenPassesProjectIDAndTokenEnv(t *testing.T) {
	runner := &fakeRunner{values: map[string]string{"TOKEN": "secret-value"}}
	client := testClient(runner)
	client.Config = Config{Token: "test-token", ProjectID: "project-id"}
	values, err := client.GetSecrets(context.Background(), []string{"TOKEN"})
	if err != nil {
		t.Fatalf("GetSecrets() error = %v", err)
	}
	if values["TOKEN"] != "secret-value" {
		t.Fatalf("values = %+v", values)
	}
	want := []string{"infisical", "secrets", "get", "TOKEN", "--plain", "--silent", "--projectId", "project-id"}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0], want) {
		t.Fatalf("commands = %+v, want %+v", runner.commands, want)
	}
	if len(runner.envs) != 1 || !hasEnv(runner.envs[0], "INFISICAL_TOKEN=test-token") {
		t.Fatalf("envs = %+v", runner.envs)
	}
}

func TestClientMintsUniversalAuthTokenAndPassesProjectID(t *testing.T) {
	runner := &fakeRunner{values: map[string]string{"TOKEN": "secret-value"}, machineToken: "minted-token\n"}
	client := testClient(runner)
	client.Config = Config{AuthMethod: "universal-auth", ClientID: "client-id", ClientSecret: "client-secret", ProjectID: "project-id", APIURL: "https://app.infisical.com/api"}
	values, err := client.GetSecrets(context.Background(), []string{"TOKEN"})
	if err != nil {
		t.Fatalf("GetSecrets() error = %v", err)
	}
	if values["TOKEN"] != "secret-value" {
		t.Fatalf("values = %+v", values)
	}
	wantGet := []string{"infisical", "secrets", "get", "TOKEN", "--plain", "--silent", "--projectId", "project-id"}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0], wantGet) {
		t.Fatalf("commands = %+v", runner.commands)
	}
	if len(runner.envs) != 1 || !hasEnv(runner.envs[0], "INFISICAL_TOKEN=minted-token") {
		t.Fatalf("envs = %+v", runner.envs)
	}
	if len(runner.mintedConfigs) != 1 || runner.mintedConfigs[0].ClientSecret != "client-secret" {
		t.Fatalf("minted configs = %+v", runner.mintedConfigs)
	}
}

func TestClientReadsMachineIdentityFromEnv(t *testing.T) {
	runner := &fakeRunner{values: map[string]string{"TOKEN": "secret-value"}, machineToken: "env-token\n"}
	client := Client{Runner: runner, LookupEnv: mapLookup(map[string]string{
		"INFISICAL_AUTH_METHOD":   "universal-auth",
		"INFISICAL_CLIENT_ID":     "env-client-id",
		"INFISICAL_CLIENT_SECRET": "env-client-secret",
		"INFISICAL_PROJECT_ID":    "env-project-id",
		"INFISICAL_ENV":           "prod",
		"INFISICAL_API_URL":       "https://app.infisical.com/api",
	})}
	if _, err := client.GetSecrets(context.Background(), []string{"TOKEN"}); err != nil {
		t.Fatalf("GetSecrets() error = %v", err)
	}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0], []string{"infisical", "secrets", "get", "TOKEN", "--plain", "--silent", "--projectId", "env-project-id", "--env", "prod"}) {
		t.Fatalf("commands = %+v", runner.commands)
	}
	if len(runner.envs) != 1 || !hasEnv(runner.envs[0], "INFISICAL_TOKEN=env-token") || !hasEnv(runner.envs[0], "INFISICAL_API_URL=https://app.infisical.com/api") {
		t.Fatalf("envs = %+v", runner.envs)
	}
}

func TestClientRejectsPlainHTTPHostAtRuntime(t *testing.T) {
	runner := &fakeRunner{values: map[string]string{"TOKEN": "secret-value"}}
	client := Client{Runner: runner, LookupEnv: mapLookup(map[string]string{
		"INFISICAL_TOKEN": "token",
		"INFISICAL_HOST":  "http://infisical.example",
	})}
	_, err := client.GetSecrets(context.Background(), []string{"TOKEN"})
	if err == nil {
		t.Fatal("GetSecrets() error = nil, want invalid host error")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %+v, want none", runner.commands)
	}
}

func TestClientReadsInfisicalHostURLAliasFromEnv(t *testing.T) {
	runner := &fakeRunner{values: map[string]string{"TOKEN": "secret-value"}, machineToken: "env-token\n"}
	client := Client{Runner: runner, LookupEnv: mapLookup(map[string]string{
		"INFISICAL_AUTH_METHOD":   "universal-auth",
		"INFISICAL_CLIENT_ID":     "env-client-id",
		"INFISICAL_CLIENT_SECRET": "env-client-secret",
		"INFISICAL_PROJECT_ID":    "env-project-id",
		"INFISICAL_HOST_URL":      "https://app.infisical.com",
	})}
	if _, err := client.GetSecrets(context.Background(), []string{"TOKEN"}); err != nil {
		t.Fatalf("GetSecrets() error = %v", err)
	}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0], []string{"infisical", "secrets", "get", "TOKEN", "--plain", "--silent", "--projectId", "env-project-id"}) {
		t.Fatalf("commands = %+v", runner.commands)
	}
	if len(runner.envs) != 1 || !hasEnv(runner.envs[0], "INFISICAL_TOKEN=env-token") || !hasEnv(runner.envs[0], "INFISICAL_HOST=https://app.infisical.com") {
		t.Fatalf("envs = %+v", runner.envs)
	}
	if len(runner.mintedConfigs) != 1 || runner.mintedConfigs[0].Host != "https://app.infisical.com" {
		t.Fatalf("minted configs = %+v", runner.mintedConfigs)
	}
}

func TestParseEnvLineStopsAtQuotedValueBeforeComment(t *testing.T) {
	_, got, ok := parseEnvLine(`INFISICAL_CLIENT_SECRET="abc" # note "prod"`)
	if !ok || got != "abc" {
		t.Fatalf("parseEnvLine() got %q ok=%v, want abc/true", got, ok)
	}
}

func TestReadInfisicalEnvFileUnescapesDoubleQuotedValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	want := `has 'single' "double" # hash \path`
	if err := os.WriteFile(path, []byte("INFISICAL_CLIENT_SECRET=\"has 'single' \\\"double\\\" # hash \\path\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	values, err := readInfisicalEnvFile(path)
	if err != nil {
		t.Fatalf("readInfisicalEnvFile() error = %v", err)
	}
	if got := values["INFISICAL_CLIENT_SECRET"]; got != want {
		t.Fatalf("INFISICAL_CLIENT_SECRET = %q, want %q", got, want)
	}
}

func TestClientHostOverridesStaleAPIURL(t *testing.T) {
	runner := &fakeRunner{values: map[string]string{"TOKEN": "secret-value"}, machineToken: "env-token\n"}
	client := Client{Runner: runner, LookupEnv: mapLookup(map[string]string{
		"INFISICAL_AUTH_METHOD":   "universal-auth",
		"INFISICAL_CLIENT_ID":     "env-client-id",
		"INFISICAL_CLIENT_SECRET": "env-client-secret",
		"INFISICAL_PROJECT_ID":    "env-project-id",
		"INFISICAL_API_URL":       "https://old-api.example",
		"INFISICAL_HOST":          "https://new-host.example",
	})}
	if _, err := client.GetSecrets(context.Background(), []string{"TOKEN"}); err != nil {
		t.Fatalf("GetSecrets() error = %v", err)
	}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0], []string{"infisical", "secrets", "get", "TOKEN", "--plain", "--silent", "--projectId", "env-project-id"}) {
		t.Fatalf("commands = %+v", runner.commands)
	}
	if len(runner.envs) != 1 || hasEnv(runner.envs[0], "INFISICAL_API_URL=https://old-api.example") || !hasEnv(runner.envs[0], "INFISICAL_API_URL=https://new-host.example") || !hasEnv(runner.envs[0], "INFISICAL_HOST=https://new-host.example") {
		t.Fatalf("envs = %+v", runner.envs)
	}
	if len(runner.mintedConfigs) != 1 || runner.mintedConfigs[0].APIURL != "https://new-host.example" {
		t.Fatalf("minted configs = %+v", runner.mintedConfigs)
	}
}

func TestClientReadsMachineIdentityFromDefaultEnvFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, key := range []string{"INFISICAL_TOKEN", "INFISICAL_AUTH_METHOD", "INFISICAL_CLIENT_ID", "INFISICAL_CLIENT_SECRET", "INFISICAL_UNIVERSAL_AUTH_CLIENT_ID", "INFISICAL_UNIVERSAL_AUTH_CLIENT_SECRET", "INFISICAL_PROJECT_ID", "INFISICAL_ENV", "INFISICAL_ENVIRONMENT", "INFISICAL_API_URL", "INFISICAL_HOST", "INFISICAL_HOST_URL"} {
		t.Setenv(key, "")
	}
	configDir := filepath.Join(home, ".config", "infisical")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	envPath := filepath.Join(configDir, ".env")
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

	runner := &fakeRunner{values: map[string]string{"TOKEN": "secret-value"}, machineToken: "file-token\n"}
	client := Client{Runner: runner}
	if _, err := client.GetSecrets(context.Background(), []string{"TOKEN"}); err != nil {
		t.Fatalf("GetSecrets() error = %v", err)
	}
	wantGet := []string{"infisical", "secrets", "get", "TOKEN", "--plain", "--silent", "--projectId", "file-project", "--env", "stage"}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0], wantGet) {
		t.Fatalf("commands = %+v", runner.commands)
	}
	if len(runner.envs) != 1 || !hasEnv(runner.envs[0], "INFISICAL_TOKEN=file-token") || !hasEnv(runner.envs[0], "INFISICAL_HOST=https://app.infisical.com") {
		t.Fatalf("envs = %+v", runner.envs)
	}
}

func TestClientRetriesStaleTokenWithUniversalAuth(t *testing.T) {
	runner := &fakeRunner{values: map[string]string{"TOKEN": "secret-value"}, machineToken: "fresh-token\n", failStaleToken: true}
	client := testClient(runner)
	client.Config = Config{Token: "stale-token", AuthMethod: "universal-auth", ClientID: "client-id", ClientSecret: "client-secret", ProjectID: "project-id"}
	values, err := client.GetSecrets(context.Background(), []string{"TOKEN"})
	if err != nil {
		t.Fatalf("GetSecrets() error = %v", err)
	}
	if values["TOKEN"] != "secret-value" {
		t.Fatalf("values = %+v", values)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("commands = %+v", runner.commands)
	}
	if !reflect.DeepEqual(runner.commands[0], []string{"infisical", "secrets", "get", "TOKEN", "--plain", "--silent", "--projectId", "project-id"}) || !reflect.DeepEqual(runner.commands[1], []string{"infisical", "secrets", "get", "TOKEN", "--plain", "--silent", "--projectId", "project-id"}) {
		t.Fatalf("commands = %+v", runner.commands)
	}
	if !hasEnv(runner.envs[0], "INFISICAL_TOKEN=stale-token") || !hasEnv(runner.envs[1], "INFISICAL_TOKEN=fresh-token") {
		t.Fatalf("envs = %+v", runner.envs)
	}
}

func TestClientCheckAuthenticatedRetriesStaleTokenWithUniversalAuth(t *testing.T) {
	runner := &fakeRunner{machineToken: "fresh-token\n", failStaleToken: true}
	client := testClient(runner)
	client.Config = Config{Token: "stale-token", AuthMethod: "universal-auth", ClientID: "client-id", ClientSecret: "client-secret", ProjectID: "project-id"}
	if err := client.CheckAuthenticated(context.Background()); err != nil {
		t.Fatalf("CheckAuthenticated() error = %v", err)
	}
	if len(runner.commands) != 2 || runner.commands[0][1] != "secrets" || runner.commands[1][1] != "secrets" {
		t.Fatalf("commands = %+v", runner.commands)
	}
}

func TestClientMachineAuthErrorDoesNotLeakSecrets(t *testing.T) {
	runner := &fakeRunner{runErr: errors.New("client-secret minted-token secret-value")}
	client := testClient(runner)
	client.Config = Config{AuthMethod: "universal-auth", ClientID: "client-id", ClientSecret: "client-secret", ProjectID: "project-id"}
	_, err := client.GetSecrets(context.Background(), []string{"TOKEN"})
	if err == nil {
		t.Fatal("GetSecrets() error = nil, want auth error")
	}
	for _, leaked := range []string{"client-secret", "minted-token", "secret-value"} {
		if strings.Contains(err.Error(), leaked) {
			t.Fatalf("error leaked %q: %v", leaked, err)
		}
	}
}

func TestClientValidateConfigIgnoresAmbientEnv(t *testing.T) {
	runner := &fakeRunner{}
	client := Client{Runner: runner, LookupEnv: mapLookup(map[string]string{
		"INFISICAL_HOST":          "http://infisical.example",
		"INFISICAL_CLIENT_SECRET": "ambient-secret",
	})}
	err := client.ValidateConfig(context.Background(), Config{AuthMethod: "universal-auth", ClientID: "client-id", ClientSecret: "client-secret", ProjectID: "project-id", Environment: "dev"})
	if err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	if len(runner.mintedConfigs) != 1 {
		t.Fatalf("minted configs = %+v", runner.mintedConfigs)
	}
	minted := runner.mintedConfigs[0]
	if minted.Host != "" || minted.ClientSecret != "client-secret" {
		t.Fatalf("minted config used ambient values: %+v", minted)
	}
	if len(runner.envs) != 1 || !hasEnv(runner.envs[0], "INFISICAL_HOST=") || !hasEnv(runner.envs[0], "INFISICAL_CLIENT_SECRET=") || !hasEnv(runner.envs[0], "INFISICAL_TOKEN=machine-token") {
		t.Fatalf("validation env did not clear ambient Infisical values: %+v", runner.envs)
	}
}

func TestClientCheckStatusReportsInvalidMachineAuth(t *testing.T) {
	runner := &fakeRunner{runErr: errors.New("client-secret minted-token secret-value")}
	client := testClient(runner)
	client.Config = Config{AuthMethod: "universal-auth", ClientID: "client-id", ClientSecret: "client-secret", ProjectID: "project-id", Environment: "dev"}

	status := client.CheckStatus(context.Background())
	if !status.CLIInstalled || status.Authenticated || status.Ready {
		t.Fatalf("status = %+v", status)
	}
	if len(status.Checks) < 2 {
		t.Fatalf("checks = %+v", status.Checks)
	}
	check := status.Checks[len(status.Checks)-1]
	if check.Code != "infisical.machine_auth_invalid" || !strings.Contains(check.Message, "machine identity") || !strings.Contains(check.Remediation, "loki secrets configure infisical") || !strings.Contains(check.Remediation, "remove") {
		t.Fatalf("machine auth check = %+v", check)
	}
	for _, leaked := range []string{"client-secret", "minted-token", "secret-value", "client-id", "project-id"} {
		if strings.Contains(check.Message, leaked) || strings.Contains(check.Remediation, leaked) {
			t.Fatalf("status leaked %q: %+v", leaked, status.Checks)
		}
	}
}

func TestClientRunWithSecretsPassesProjectID(t *testing.T) {
	runner := &fakeRunner{runOutput: "ok\n"}
	client := testClient(runner)
	client.Config = Config{Token: "test-token", ProjectID: "project-id"}
	out, err := client.RunWithSecrets(context.Background(), []string{"printenv", "TOKEN"}, []string{"EXTRA=value"})
	if err != nil {
		t.Fatalf("RunWithSecrets() error = %v", err)
	}
	if string(out) != "ok\n" {
		t.Fatalf("out = %q", out)
	}
	want := []string{"infisical", "run", "--projectId", "project-id", "--", "printenv", "TOKEN"}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0], want) {
		t.Fatalf("commands = %+v, want %+v", runner.commands, want)
	}
	if len(runner.envs) != 1 || !hasEnv(runner.envs[0], "INFISICAL_TOKEN=test-token") || !hasEnv(runner.envs[0], "EXTRA=value") {
		t.Fatalf("envs = %+v", runner.envs)
	}
}

func TestClientRunWithSecretsRetriesStaleToken(t *testing.T) {
	runner := &fakeRunner{runOutput: "ok\n", machineToken: "fresh-token\n", failStaleToken: true}
	client := testClient(runner)
	client.Config = Config{Token: "stale-token", AuthMethod: "universal-auth", ClientID: "client-id", ClientSecret: "client-secret", ProjectID: "project-id"}
	out, err := client.RunWithSecrets(context.Background(), []string{"printenv", "TOKEN"}, nil)
	if err != nil {
		t.Fatalf("RunWithSecrets() error = %v", err)
	}
	if string(out) != "ok\n" {
		t.Fatalf("out = %q", out)
	}
	if len(runner.commands) != 2 || runner.commands[0][1] != "run" || runner.commands[1][1] != "run" {
		t.Fatalf("commands = %+v", runner.commands)
	}
	if !hasEnv(runner.envs[0], "INFISICAL_TOKEN=stale-token") || !hasEnv(runner.envs[1], "INFISICAL_TOKEN=fresh-token") {
		t.Fatalf("envs = %+v", runner.envs)
	}
}

func TestClientRunWithSecretsErrorDoesNotLeakSecrets(t *testing.T) {
	runner := &fakeRunner{runErr: errors.New("test-token secret-value")}
	client := testClient(runner)
	client.Config = Config{Token: "test-token", ProjectID: "project-id"}
	out, err := client.RunWithSecrets(context.Background(), []string{"printenv", "TOKEN"}, nil)
	if err == nil {
		t.Fatal("RunWithSecrets() error = nil, want failure")
	}
	if out != nil {
		t.Fatalf("out = %q, want nil", out)
	}
	for _, leaked := range []string{"test-token", "secret-value"} {
		if strings.Contains(err.Error(), leaked) {
			t.Fatalf("error leaked %q: %v", leaked, err)
		}
	}
}

func TestRequiredSecretsAndRenderTemplate(t *testing.T) {
	template := []byte("token={{ TOKEN }}\nproject=${PROJECT}\n")
	if got := RequiredSecrets(template, []string{"EXTRA"}); !reflect.DeepEqual(got, []string{"EXTRA", "PROJECT", "TOKEN"}) {
		t.Fatalf("RequiredSecrets() = %#v", got)
	}
	rendered, err := RenderTemplate(template, map[string]string{"TOKEN": "secret", "PROJECT": "demo", "EXTRA": "unused"}, []string{"EXTRA"})
	if err != nil {
		t.Fatalf("RenderTemplate() error = %v", err)
	}
	if string(rendered) != "token=secret\nproject=demo\n" {
		t.Fatalf("rendered = %q", rendered)
	}
	_, err = RenderTemplate(template, map[string]string{"TOKEN": "super-private"}, nil)
	if err == nil || strings.Contains(err.Error(), "super-private") || !strings.Contains(err.Error(), "PROJECT") {
		t.Fatalf("missing error = %v", err)
	}
	if _, err := RenderTemplate(template, map[string]string{}, []string{"BAD-NAME"}); err == nil {
		t.Fatal("RenderTemplate() invalid secret error = nil")
	}
}
