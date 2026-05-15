package storemigrate

import (
	"context"
	"sync"
	"time"
)

// Phase identifies a high-level store migration phase.
type Phase string

const (
	PhasePreflight Phase = "preflight"
	PhaseHydrate   Phase = "hydrate"
	PhaseCopy      Phase = "copy"
	PhaseValidate  Phase = "validate"
	PhasePromote   Phase = "promote"
	PhaseRewire    Phase = "rewire"
	PhaseRetarget  Phase = "retarget"
	PhaseCleanup   Phase = "cleanup"
	PhaseDone      Phase = "done"
)

// Event describes observable migration progress.
type Event struct {
	Phase       Phase  `json:"phase"`
	Message     string `json:"message,omitempty"`
	CurrentPath string `json:"current_path,omitempty"`
	DoneFiles   int    `json:"done_files,omitempty"`
	TotalFiles  int    `json:"total_files,omitempty"`
	DoneBytes   int64  `json:"done_bytes,omitempty"`
	TotalBytes  int64  `json:"total_bytes,omitempty"`
}

// Reporter receives migration progress events.
type Reporter interface {
	Report(ctx context.Context, event Event)
}

// ReporterFunc adapts a function into a Reporter.
type ReporterFunc func(context.Context, Event)

func (f ReporterFunc) Report(ctx context.Context, event Event) {
	if f != nil {
		f(ctx, event)
	}
}

// NoopReporter drops all progress events.
type NoopReporter struct{}

func (NoopReporter) Report(context.Context, Event) {}

// MemoryReporter records progress events for tests and callers that need replay.
type MemoryReporter struct {
	mu     sync.Mutex
	events []Event
}

func (r *MemoryReporter) Report(_ context.Context, event Event) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *MemoryReporter) Events() []Event {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Event, len(r.events))
	copy(out, r.events)
	return out
}

// ThrottledReporter forwards phase changes immediately and same-phase events at most once per interval.
type ThrottledReporter struct {
	next     Reporter
	interval time.Duration
	now      func() time.Time

	mu           sync.Mutex
	lastPhase    Phase
	lastReported time.Time
	hasReported  bool
}

func NewThrottledReporter(next Reporter, interval time.Duration, now func() time.Time) *ThrottledReporter {
	if next == nil {
		next = NoopReporter{}
	}
	if now == nil {
		now = time.Now
	}
	return &ThrottledReporter{next: next, interval: interval, now: now}
}

func (r *ThrottledReporter) Report(ctx context.Context, event Event) {
	if r == nil {
		return
	}
	now := r.now()
	r.mu.Lock()
	phaseChanged := !r.hasReported || event.Phase != r.lastPhase
	intervalElapsed := r.interval <= 0 || now.Sub(r.lastReported) >= r.interval
	if !phaseChanged && !intervalElapsed {
		r.mu.Unlock()
		return
	}
	r.hasReported = true
	r.lastPhase = event.Phase
	r.lastReported = now
	next := r.next
	r.mu.Unlock()
	next.Report(ctx, event)
}

func report(ctx context.Context, reporter Reporter, event Event) {
	if reporter == nil {
		return
	}
	reporter.Report(ctx, event)
}
