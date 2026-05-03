package app

import (
	"context"
	"path/filepath"
	"testing"
)

func TestServiceVerifyValidStore(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := svc.EnsureStore(context.Background(), EnsureStoreRequest{StorePath: storePath}); err != nil {
		t.Fatalf("EnsureStore() error = %v", err)
	}
	report, err := svc.Verify(context.Background(), VerifyRequest{ParentProfile: "work"})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !report.Valid {
		t.Fatalf("report = %+v", report)
	}
}

func TestServiceVerifyInvalidStore(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	report, err := svc.Verify(context.Background(), VerifyRequest{StorePath: filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if report.Valid || report.Summary.Blocking == 0 {
		t.Fatalf("report = %+v", report)
	}
}
