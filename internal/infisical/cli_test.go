package infisical

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/secrets"
)

type fakeRunner struct {
	missingBinary       bool
	values              map[string]string
	runErr              error
	runOutput           string
	interactiveErr      error
	commands            [][]string
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
	if f.runErr != nil {
		return []byte(f.runOutput), f.runErr
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
			return nil, errors.New("missing")
		}
		return []byte(value + "\n"), nil
	}
	return nil, errors.New("unexpected command")
}

func (f *fakeRunner) RunInteractive(ctx context.Context, name string, args []string, env []string) error {
	f.interactiveCommands = append(f.interactiveCommands, append([]string{name}, args...))
	return f.interactiveErr
}

func TestClientGetSecretsUsesRunnerAndHidesValuesOnMissing(t *testing.T) {
	runner := &fakeRunner{values: map[string]string{"TOKEN": "secret-value", "PROJECT": "demo"}}
	client := Client{Runner: runner}
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

func TestClientCheckInstalledMissingCLI(t *testing.T) {
	client := Client{Runner: &fakeRunner{missingBinary: true}}
	if err := client.CheckInstalled(context.Background()); err == nil {
		t.Fatal("CheckInstalled() error = nil, want error")
	}
}

func TestClientCheckAuthenticatedUsesReadinessProbeWithoutLeakingOutput(t *testing.T) {
	runner := &fakeRunner{runErr: errors.New("auth failed"), runOutput: "dummy-sensitive-output"}
	client := Client{Runner: runner}
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
	ready := Client{Runner: &fakeRunner{}}
	status := ready.CheckStatus(context.Background())
	if !status.CLIInstalled || !status.Authenticated || !status.Ready {
		t.Fatalf("ready status = %+v", status)
	}

	missing := Client{Runner: &fakeRunner{missingBinary: true}}
	status = missing.CheckStatus(context.Background())
	if status.CLIInstalled || status.Ready || len(status.Checks) == 0 || status.Checks[0].Severity != secrets.SeverityWarning {
		t.Fatalf("missing status = %+v", status)
	}
}

func TestClientLoginUsesInteractiveRunner(t *testing.T) {
	runner := &fakeRunner{}
	client := Client{Runner: runner}
	if err := client.Login(context.Background(), secrets.LoginRequest{Domain: "https://example.test"}); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	want := []string{"infisical", "login", "--domain", "https://example.test"}
	if len(runner.interactiveCommands) != 1 || !reflect.DeepEqual(runner.interactiveCommands[0], want) {
		t.Fatalf("interactive commands = %+v, want %+v", runner.interactiveCommands, want)
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
