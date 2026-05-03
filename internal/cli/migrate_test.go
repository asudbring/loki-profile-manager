package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateLocalRequiresYesCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliMigrationStore(t)
	cliWrite(t, filepath.Join(home, ".gitconfig"), "[user]\n\tname = CLI\n")
	cmd, out, _ := migrationTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "migrate", "local", "--profile", "work"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("Execute() error = %v output=%s", err, out.String())
	}
}

func TestMigrateRepoJSONDryRunCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliMigrationStore(t)
	repo := t.TempDir()
	cliWrite(t, filepath.Join(repo, ".gitconfig"), "[user]\n\tname = Repo\n")
	cmd, out, _ := migrationTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "migrate", "repo", repo, "--profile", "work", "--dry-run", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var decoded struct {
		Plan struct {
			Items []struct {
				Target string `json:"target"`
			} `json:"items"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json output invalid: %v\n%s", err, out.String())
	}
	if len(decoded.Plan.Items) != 1 || decoded.Plan.Items[0].Target != "~/.gitconfig" {
		t.Fatalf("decoded = %+v output=%s", decoded, out.String())
	}
}
