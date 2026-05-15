package storemigrate

import (
	"context"
	"testing"
	"time"
)

func TestMemoryReporterRecordsEvents(t *testing.T) {
	reporter := &MemoryReporter{}
	reporter.Report(context.Background(), Event{Phase: PhaseCopy, Message: "copying", CurrentPath: "profiles/work/core/manifest.yaml"})
	reporter.Report(context.Background(), Event{Phase: PhaseCopy, DoneFiles: 1, TotalFiles: 2})

	events := reporter.Events()
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Phase != PhaseCopy || events[0].Message != "copying" {
		t.Fatalf("first event = %+v", events[0])
	}
}

func TestThrottledReporterAlwaysReportsPhaseChanges(t *testing.T) {
	memory := &MemoryReporter{}
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	reporter := NewThrottledReporter(memory, time.Minute, func() time.Time { return now })

	reporter.Report(context.Background(), Event{Phase: PhasePreflight, Message: "preflight"})
	reporter.Report(context.Background(), Event{Phase: PhasePreflight, Message: "too soon"})
	reporter.Report(context.Background(), Event{Phase: PhaseCopy, Message: "phase changed"})

	events := memory.Events()
	if len(events) != 2 {
		t.Fatalf("events = %+v, want preflight and copy only", events)
	}
	if events[0].Phase != PhasePreflight || events[1].Phase != PhaseCopy {
		t.Fatalf("events = %+v", events)
	}
}

func TestThrottledReporterReportsAfterInterval(t *testing.T) {
	memory := &MemoryReporter{}
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	reporter := NewThrottledReporter(memory, time.Second, func() time.Time { return now })

	reporter.Report(context.Background(), Event{Phase: PhaseCopy, Message: "first"})
	now = now.Add(500 * time.Millisecond)
	reporter.Report(context.Background(), Event{Phase: PhaseCopy, Message: "hidden"})
	now = now.Add(600 * time.Millisecond)
	reporter.Report(context.Background(), Event{Phase: PhaseCopy, Message: "visible"})

	events := memory.Events()
	if len(events) != 2 || events[1].Message != "visible" {
		t.Fatalf("events = %+v", events)
	}
}
