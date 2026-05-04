package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/config"
	diagnostics "github.com/allensu/loki-profile-manager/internal/doctor"
)

func TestDoctorNoStoreReportsWarning(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	report, err := svc.Doctor(ctx, DoctorRequest{})
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if !report.Healthy {
		t.Fatalf("Healthy = false, want true for warning-only report: %+v", report.Summary)
	}
	if !reportHasCheck(report, "store.not_configured", diagnostics.SeverityWarning) {
		t.Fatalf("report missing store.not_configured warning: %+v", report.Checks)
	}
}

func TestRunDoctorDoesNotCreateLocalState(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}.WithDefaults()
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}

	report, err := RunDoctor(ctx, Options{Resolver: resolver})
	if err != nil {
		t.Fatalf("RunDoctor() error = %v", err)
	}
	if !reportHasCheck(report, "sqlite.database_missing", diagnostics.SeverityWarning) {
		t.Fatalf("report missing sqlite.database_missing warning: %+v", report.Checks)
	}
	if _, err := os.Stat(paths.StateDir); !os.IsNotExist(err) {
		t.Fatalf("RunDoctor created local state or stat failed: %v", err)
	}
}

func TestDoctorInvalidStoreReportsBlocking(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	svc, err := NewService(ctx, Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()

	report, err := svc.Doctor(ctx, DoctorRequest{StorePath: filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if report.Healthy || report.Summary.Blocking == 0 {
		t.Fatalf("report healthy = %v summary = %+v, want blocking", report.Healthy, report.Summary)
	}
	if !reportHasCheck(report, "store.root_missing", diagnostics.SeverityBlocking) {
		t.Fatalf("report missing store.root_missing blocking: %+v", report.Checks)
	}
}

func reportHasCheck(report DoctorResult, code string, severity diagnostics.Severity) bool {
	for _, check := range report.Checks {
		if check.Code == code && check.Severity == severity {
			return true
		}
	}
	return false
}
