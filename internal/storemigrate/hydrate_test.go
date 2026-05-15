package storemigrate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestHydratePlanReadsOnlyDatalessEntries(t *testing.T) {
	plan := Plan{
		CanMigrate: true,
		DatalessEntries: []Entry{
			{RelativePath: "profiles/work/core/files/cloud.txt", SourcePath: "/cloud.txt", Kind: "file", Size: 5},
		},
	}
	reporter := &MemoryReporter{}
	var hydrated []string
	result, err := HydratePlan(context.Background(), HydrateOptions{
		Plan:     plan,
		Reporter: reporter,
		HydrateFile: func(ctx context.Context, entry Entry) (int64, error) {
			hydrated = append(hydrated, entry.RelativePath)
			return entry.Size, nil
		},
	})
	if err != nil {
		t.Fatalf("HydratePlan() error = %v", err)
	}
	if result.HydratedFiles != 1 || result.HydratedBytes != 5 {
		t.Fatalf("result = %+v", result)
	}
	if len(hydrated) != 1 || hydrated[0] != "profiles/work/core/files/cloud.txt" {
		t.Fatalf("hydrated = %+v", hydrated)
	}
	if len(reporter.Events()) == 0 || reporter.Events()[0].Phase != PhaseHydrate {
		t.Fatalf("events = %+v", reporter.Events())
	}
}

func TestHydratePlanStopsOnFileTimeout(t *testing.T) {
	plan := Plan{CanMigrate: true, DatalessEntries: []Entry{{RelativePath: "cloud.txt", SourcePath: "/cloud.txt", Kind: "file"}}}
	_, err := HydratePlan(context.Background(), HydrateOptions{
		Plan:        plan,
		FileTimeout: time.Millisecond,
		HydrateFile: func(ctx context.Context, entry Entry) (int64, error) {
			<-ctx.Done()
			return 0, ctx.Err()
		},
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("HydratePlan(timeout) error = %v", err)
	}
}

func TestHydratePlanPropagatesReadError(t *testing.T) {
	want := errors.New("read failed")
	plan := Plan{CanMigrate: true, DatalessEntries: []Entry{{RelativePath: "cloud.txt", SourcePath: "/cloud.txt", Kind: "file"}}}
	_, err := HydratePlan(context.Background(), HydrateOptions{
		Plan: plan,
		HydrateFile: func(ctx context.Context, entry Entry) (int64, error) {
			return 0, want
		},
	})
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("HydratePlan(read error) = %v, want %v", err, want)
	}
}
