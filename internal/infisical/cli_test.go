package infisical

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	missingBinary bool
	values        map[string]string
	commands      [][]string
}

func (f *fakeRunner) LookPath(file string) (string, error) {
	if f.missingBinary {
		return "", errors.New("not found")
	}
	return "/usr/bin/" + file, nil
}

func (f *fakeRunner) Run(ctx context.Context, name string, args []string, env []string) ([]byte, error) {
	f.commands = append(f.commands, append([]string{name}, args...))
	key := args[len(args)-1]
	value, ok := f.values[key]
	if !ok {
		return nil, errors.New("missing")
	}
	return []byte(value + "\n"), nil
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

	_, err = client.GetSecrets(context.Background(), []string{"MISSING"})
	if err == nil {
		t.Fatal("GetSecrets() error = nil, want missing")
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
}
