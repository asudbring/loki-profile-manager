package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/config"
)

func TestMigrateLocalWritesAdoptionRecords(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := adoptStore(t)
	target := filepath.Join(home, ".gitconfig")
	writeAppFile(t, target, "[user]\n\temail = local@example.test\n")

	result, err := svc.MigrateLocal(ctx, MigrateLocalRequest{StorePath: storePath, Profile: "work", Yes: true})
	if err != nil {
		t.Fatalf("MigrateLocal() error = %v", err)
	}
	if len(result.Plan.Items) != 1 || result.Changed != 1 {
		t.Fatalf("result = %+v", result)
	}
	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", DryRun: true}); err != nil {
		t.Fatalf("Switch(dry-run after migrate local) error = %v", err)
	}
}

func TestMigrateRepoAdoptsMatchingExistingTarget(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	storePath := adoptStore(t)
	repo := t.TempDir()
	writeAppFile(t, filepath.Join(repo, ".gitconfig"), "[core]\n\teditor = vim\n")
	writeAppFile(t, filepath.Join(home, ".gitconfig"), "[core]\n\teditor = vim\n")

	result, err := svc.MigrateRepo(ctx, MigrateRepoRequest{StorePath: storePath, RepoPath: repo, Profile: "work", Yes: true})
	if err != nil {
		t.Fatalf("MigrateRepo() error = %v", err)
	}
	if len(result.Plan.Items) != 1 || !result.Plan.Items[0].WillAdoptRecord {
		t.Fatalf("result = %+v", result)
	}
	if _, err := svc.Switch(ctx, SwitchRequest{StorePath: storePath, ParentProfile: "work", DryRun: true}); err != nil {
		t.Fatalf("Switch(dry-run after repo migrate) error = %v", err)
	}
}
