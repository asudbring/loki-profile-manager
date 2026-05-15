# Store Migration V2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `loki store migrate` fast, observable, safe, and user-friendly for moving Loki stores between OneDrive Personal, OneDrive for Business, Dropbox, or manual filesystem locations.

**Architecture:** Replace blind recursive final-destination copy with a phased migration pipeline: preflight, optional source hydration/materialization, staged copy, staged validation, atomic promotion, transactional local rewire, and active symlink retarget. Progress events flow from `internal/storemigrate` through `internal/app` to CLI output so long-running work is never silent.

**Tech Stack:** Go 1.23, Cobra CLI, SQLite via existing `internal/db`, filesystem APIs, cross-platform cloud placeholder detector interface, macOS `Stat_t.Flags` dataless detection, conservative Windows Cloud Files attribute detection where available, existing `internal/store` operation locks and layout validation.

---

## Root Cause and Product Requirements

Current `v0.1.9` behavior correctly detects macOS dataless/cloud-only source files and refuses to start. That avoids silent multi-hour copy, but it does not solve the user problem: users must be able to move a store easily.

Observed root cause on Allen's Mac:

- Source store contained hundreds of OneDrive Personal dataless files.
- Reading each file forced OneDrive/File Provider hydration.
- Copy had no progress, no ETA, no current-file output, and no per-file stall handling.
- Interrupt left a source lock and partial final destination.
- Existing rewire updated local SQLite source paths but did not retarget active symlinks that still point to old store files.

V2 requirements:

1. Dry-run/preflight must finish quickly and explain expected work.
2. Dataless/cloud-only files must be explicit: fail fast by default, hydrate only when user opts in.
3. Long-running `--yes` must show progress by default.
4. Copy must never build partial content directly in the final destination path.
5. Interrupt must not leave an unclear state.
6. Rewire must update persisted store path, managed-target records, metadata sources, and active symlink targets.
7. Migration should remain filesystem-first; do not add OneDrive/Dropbox API auth in this iteration.
8. Cloud placeholder detection must be cross-platform by interface: macOS detects dataless flags, Windows detects Cloud Files attributes conservatively when available, and other platforms return false while retaining staged copy, progress, timeout, cleanup, DB rewire, and symlink retarget behavior.

## File Structure

### New files

- `internal/storemigrate/events.go` — progress event model and reporter interfaces.
- `internal/storemigrate/staging.go` — staging directory naming, manifest, promote, cleanup.
- `internal/storemigrate/hydrate.go` — explicit source hydration/materialization engine.
- `internal/storemigrate/hydrate_darwin.go` — macOS dataless helpers where platform-specific code is needed.
- `internal/storemigrate/hydrate_other.go` — non-macOS no-op dataless handling.
- `internal/storemigrate/detector.go` — cross-platform `CloudPlaceholderDetector` interface and default detector factory.
- `internal/storemigrate/detector_darwin.go` — macOS dataless flag detector.
- `internal/storemigrate/detector_windows.go` — conservative Windows Cloud Files attribute detector where available, otherwise false.
- `internal/storemigrate/detector_other.go` — Linux/other detector returning false.
- `internal/storemigrate/events_test.go` — progress reporter tests.
- `internal/storemigrate/staging_test.go` — staging/promote/cleanup tests.
- `internal/storemigrate/hydrate_test.go` — hydration engine tests with fake file operations.
- `internal/activation/retarget.go` — active managed symlink retarget helper.
- `internal/activation/retarget_test.go` — symlink retarget tests.

### Modified files

- `internal/storemigrate/plan.go` — turn current hard dataless failure into preflight data; support dataless policy options.
- `internal/storemigrate/copy.go` — context-aware copy to staging, progress, stall timeout, no final-destination writes.
- `internal/storemigrate/dataless.go` — keep dataless flag helper; add stable blocker/message helpers.
- `internal/storemigrate/detector*.go` — route dataless/cloud-only checks through `CloudPlaceholderDetector`.
- `internal/storemigrate/plan_test.go` — update dataless expectations.
- `internal/storemigrate/copy_test.go` — update copy function names and add timeout/progress tests.
- `internal/app/store_migrate.go` — orchestrate phases and progress, staged promotion, transaction, symlink retarget.
- `internal/app/store_migrate_test.go` — end-to-end service tests for staged copy, hydration requirement, rewire, symlink retarget.
- `internal/cli/store.go` — add `--hydrate`, `--file-timeout`, `--progress-interval`, `--resume`, `--cleanup`, human progress, optional JSON event stream.
- `internal/cli/store_test.go` — CLI progress and flag tests.
- `docs/USAGE.md`, `docs/INSTALL.md`, `docs/ARCHITECTURE.md`, `README.md`, `CHANGELOG.md` — document V2 flow and recovery.

---

## Task 1: Add Progress Event Model

**Files:**
- Create: `internal/storemigrate/events.go`
- Create: `internal/storemigrate/events_test.go`

- [ ] **Step 1: Write failing tests for reporter behavior**

Create `internal/storemigrate/events_test.go`:

```go
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
```

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/storemigrate -run 'TestMemoryReporter|TestThrottledReporter'
```

Expected: FAIL because `MemoryReporter`, `Event`, and `PhaseCopy` are undefined.

- [ ] **Step 3: Implement progress event model**

Create `internal/storemigrate/events.go`:

```go
package storemigrate

import (
	"context"
	"sync"
	"time"
)

type Phase string

const (
	PhasePreflight Phase = "preflight"
	PhaseHydrate   Phase = "hydrate"
	PhaseCopy      Phase = "copy"
	PhaseValidate  Phase = "validate"
	PhasePromote   Phase = "promote"
	PhaseRewire    Phase = "rewire"
	PhaseCleanup   Phase = "cleanup"
	PhaseDone      Phase = "done"
)

type Event struct {
	Phase       Phase     `json:"phase"`
	Message     string    `json:"message,omitempty"`
	CurrentPath string    `json:"current_path,omitempty"`
	DoneFiles   int       `json:"done_files,omitempty"`
	TotalFiles  int       `json:"total_files,omitempty"`
	DoneBytes   int64     `json:"done_bytes,omitempty"`
	TotalBytes  int64     `json:"total_bytes,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	Err         string    `json:"error,omitempty"`
}

type Reporter interface {
	Report(context.Context, Event)
}

type ReporterFunc func(context.Context, Event)

func (fn ReporterFunc) Report(ctx context.Context, event Event) {
	if fn != nil {
		fn(ctx, event)
	}
}

type NoopReporter struct{}

func (NoopReporter) Report(context.Context, Event) {}

func reporterOrNoop(reporter Reporter) Reporter {
	if reporter == nil {
		return NoopReporter{}
	}
	return reporter
}

type MemoryReporter struct {
	mu     sync.Mutex
	events []Event
}

func (r *MemoryReporter) Report(_ context.Context, event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *MemoryReporter) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Event, len(r.events))
	copy(out, r.events)
	return out
}

type ThrottledReporter struct {
	next     Reporter
	interval time.Duration
	now      func() time.Time
	lastTime time.Time
	lastPhase Phase
}

func NewThrottledReporter(next Reporter, interval time.Duration, now func() time.Time) *ThrottledReporter {
	if now == nil {
		now = time.Now
	}
	if interval <= 0 {
		interval = time.Second
	}
	return &ThrottledReporter{next: reporterOrNoop(next), interval: interval, now: now}
}

func (r *ThrottledReporter) Report(ctx context.Context, event Event) {
	current := r.now()
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = current
	}
	phaseChanged := event.Phase != "" && event.Phase != r.lastPhase
	intervalElapsed := r.lastTime.IsZero() || current.Sub(r.lastTime) >= r.interval
	terminal := event.Phase == PhaseDone || event.Err != ""
	if phaseChanged || intervalElapsed || terminal {
		r.next.Report(ctx, event)
		r.lastTime = current
		if event.Phase != "" {
			r.lastPhase = event.Phase
		}
	}
}
```

- [ ] **Step 4: Verify GREEN**

Run:

```bash
go test ./internal/storemigrate -run 'TestMemoryReporter|TestThrottledReporter'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storemigrate/events.go internal/storemigrate/events_test.go
git commit -m "feat(store): add migration progress events"
```

---

## Task 2: Convert Preflight Into Rich Plan Output

**Files:**
- Create: `internal/storemigrate/detector.go`
- Create: `internal/storemigrate/detector_darwin.go`
- Create: `internal/storemigrate/detector_windows.go`
- Create: `internal/storemigrate/detector_other.go`
- Modify: `internal/storemigrate/plan.go`
- Modify: `internal/storemigrate/plan_test.go`
- Modify: `internal/storemigrate/dataless.go`

- [ ] **Step 1: Write failing tests for dataless preflight data**

Add to `internal/storemigrate/plan_test.go`:

```go
func TestBuildPlanReportsDatalessFilesWithoutScanningForever(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	cloudFile := filepath.Join(source, "profiles", "work", "core", "files", "cloud.txt")
	if err := os.WriteFile(cloudFile, []byte("cloud"), 0o644); err != nil {
		t.Fatalf("WriteFile(cloud) error = %v", err)
	}

	plan, err := BuildPlan(PlanOptions{
		FromPath: source,
		ToPath: filepath.Join(t.TempDir(), "dest"),
		Dataless: func(path string, info fs.FileInfo) bool {
			return path == cloudFile
		},
	})
	if err == nil || !strings.Contains(err.Error(), "cloud-only") {
		t.Fatalf("BuildPlan() error = %v, want cloud-only blocker", err)
	}
	if plan.Summary.DatalessCount != 1 {
		t.Fatalf("DatalessCount = %d, want 1", plan.Summary.DatalessCount)
	}
	if len(plan.DatalessEntries) != 1 || plan.DatalessEntries[0].RelativePath != "profiles/work/core/files/cloud.txt" {
		t.Fatalf("DatalessEntries = %+v", plan.DatalessEntries)
	}
}

func TestBuildPlanAllowsDatalessWhenHydrateRequested(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if _, err := store.EnsureLayout(source); err != nil {
		t.Fatalf("EnsureLayout(source) error = %v", err)
	}
	cloudFile := filepath.Join(source, "profiles", "work", "core", "files", "cloud.txt")
	if err := os.WriteFile(cloudFile, []byte("cloud"), 0o644); err != nil {
		t.Fatalf("WriteFile(cloud) error = %v", err)
	}

	plan, err := BuildPlan(PlanOptions{
		FromPath: source,
		ToPath: filepath.Join(t.TempDir(), "dest"),
		AllowDataless: true,
		Dataless: func(path string, info fs.FileInfo) bool { return path == cloudFile },
	})
	if err != nil {
		t.Fatalf("BuildPlan(AllowDataless) error = %v", err)
	}
	if !plan.CanMigrate || plan.Summary.DatalessCount != 1 {
		t.Fatalf("plan = %+v", plan)
	}
}
```

Add imports if missing:

```go
import "io/fs"
```

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/storemigrate -run 'TestBuildPlanReportsDataless|TestBuildPlanAllowsDataless'
```

Expected: FAIL because `PlanOptions.Dataless`, `AllowDataless`, `Summary.DatalessCount`, and `Plan.DatalessEntries` are undefined.

- [ ] **Step 3: Add preflight fields**

Modify `internal/storemigrate/plan.go` types:

```go
type CloudPlaceholderDetector interface {
	IsCloudOnly(path string, info fs.FileInfo) bool
}

type PlanOptions struct {
	FromPath      string
	ToPath        string
	Provider      store.ProviderType
	AllowDataless bool
	Dataless      func(path string, info fs.FileInfo) bool
	Detector      CloudPlaceholderDetector
}

type Summary struct {
	FileCount     int   `json:"file_count"`
	DirCount      int   `json:"dir_count"`
	SymlinkCount  int   `json:"symlink_count"`
	DatalessCount int   `json:"dataless_count,omitempty"`
	ByteCount     int64 `json:"byte_count"`
}

type Plan struct {
	FromPath        string             `json:"from_path"`
	ToPath          string             `json:"to_path"`
	Provider        store.ProviderType `json:"provider,omitempty"`
	CanMigrate      bool               `json:"can_migrate"`
	Summary         Summary            `json:"summary"`
	Entries         []Entry            `json:"entries,omitempty"`
	DatalessEntries []Entry            `json:"dataless_entries,omitempty"`
	Warnings        []string           `json:"warnings,omitempty"`
	Blockers        []string           `json:"blockers,omitempty"`
}
```

Add `internal/storemigrate/detector.go`:

```go
type CloudPlaceholderDetector interface {
	IsCloudOnly(path string, info fs.FileInfo) bool
}

func DefaultCloudPlaceholderDetector() CloudPlaceholderDetector {
	return platformCloudPlaceholderDetector{}
}
```

Implement platform files:

- `detector_darwin.go`: `platformCloudPlaceholderDetector.IsCloudOnly` returns `fileInfoDataless(info)`.
- `detector_windows.go`: `platformCloudPlaceholderDetector.IsCloudOnly` checks conservative Windows Cloud Files attributes if available, otherwise returns false.
- `detector_other.go`: `platformCloudPlaceholderDetector.IsCloudOnly` returns false.

Inside `BuildPlan`, replace direct `fileInfoDataless(info)` calls:

```go
	dataless := opts.Dataless
	if dataless == nil {
		detector := opts.Detector
		if detector == nil {
			detector = DefaultCloudPlaceholderDetector()
		}
		dataless = detector.IsCloudOnly
	}
```

When building each entry:

```go
		if dataless(path, info) {
			plan.Summary.DatalessCount++
			plan.DatalessEntries = append(plan.DatalessEntries, entryPlan)
			if !opts.AllowDataless {
				plan.Blockers = append(plan.Blockers, datalessBlocker(entryPlan))
			}
		}
```

Update error wording:

```go
	if len(plan.Blockers) > 0 {
		return plan, fmt.Errorf("store migrate: source contains %d cloud-only or unsupported entries; first blocker: %s", len(plan.Blockers), plan.Blockers[0])
	}
```

- [ ] **Step 4: Verify GREEN**

Run:

```bash
go test ./internal/storemigrate -run 'TestBuildPlanReportsDataless|TestBuildPlanAllowsDataless|TestBuildPlan'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storemigrate/plan.go internal/storemigrate/plan_test.go internal/storemigrate/dataless.go
git commit -m "feat(store): report cloud-only files in preflight"
```

---

## Task 3: Add Explicit Hydration Engine

**Files:**
- Create: `internal/storemigrate/hydrate.go`
- Create: `internal/storemigrate/hydrate_test.go`
- Modify: `internal/storemigrate/events.go`

- [ ] **Step 1: Write failing hydration tests**

Create `internal/storemigrate/hydrate_test.go`:

```go
package storemigrate

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHydratePlanReadsDatalessEntriesWithProgress(t *testing.T) {
	plan := Plan{DatalessEntries: []Entry{
		{RelativePath: "a.txt", SourcePath: filepath.Join("source", "a.txt"), Size: 3},
		{RelativePath: "b.txt", SourcePath: filepath.Join("source", "b.txt"), Size: 4},
	}}
	opened := map[string]bool{}
	reporter := &MemoryReporter{}

	result, err := HydratePlan(context.Background(), plan, HydrateOptions{
		Concurrency: 2,
		FileTimeout: time.Second,
		Reporter: reporter,
		Open: func(path string) (io.ReadCloser, error) {
			opened[path] = true
			return io.NopCloser(strings.NewReader("content")), nil
		},
	})
	if err != nil {
		t.Fatalf("HydratePlan() error = %v", err)
	}
	if result.HydratedFiles != 2 || len(opened) != 2 {
		t.Fatalf("result=%+v opened=%+v", result, opened)
	}
	if len(reporter.Events()) == 0 {
		t.Fatal("no progress events recorded")
	}
}

func TestHydratePlanFailsOnTimeout(t *testing.T) {
	plan := Plan{DatalessEntries: []Entry{{RelativePath: "slow.txt", SourcePath: "slow.txt", Size: 1}}}
	_, err := HydratePlan(context.Background(), plan, HydrateOptions{
		Concurrency: 1,
		FileTimeout: 10 * time.Millisecond,
		Open: func(path string) (io.ReadCloser, error) {
			reader, writer := io.Pipe()
			_ = writer
			return reader, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("HydratePlan(timeout) error = %v", err)
	}
}

func TestHydratePlanReturnsOpenError(t *testing.T) {
	plan := Plan{DatalessEntries: []Entry{{RelativePath: "missing.txt", SourcePath: "missing.txt", Size: 1}}}
	_, err := HydratePlan(context.Background(), plan, HydrateOptions{
		Open: func(path string) (io.ReadCloser, error) { return nil, errors.New("boom") },
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("HydratePlan(open error) error = %v", err)
	}
}
```

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/storemigrate -run 'TestHydratePlan'
```

Expected: FAIL because hydration types are undefined.

- [ ] **Step 3: Implement hydration engine**

Create `internal/storemigrate/hydrate.go`:

```go
package storemigrate

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type HydrateOptions struct {
	Concurrency int
	FileTimeout time.Duration
	Reporter    Reporter
	Open        func(string) (io.ReadCloser, error)
}

type HydrateResult struct {
	HydratedFiles int   `json:"hydrated_files"`
	HydratedBytes int64 `json:"hydrated_bytes"`
}

func HydratePlan(ctx context.Context, plan Plan, opts HydrateOptions) (HydrateResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	fileTimeout := opts.FileTimeout
	if fileTimeout <= 0 {
		fileTimeout = 30 * time.Second
	}
	open := opts.Open
	if open == nil {
		open = func(path string) (io.ReadCloser, error) { return os.Open(path) }
	}
	reporter := reporterOrNoop(opts.Reporter)
	jobs := make(chan Entry)
	var result HydrateResult
	var firstErr atomic.Value
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for entry := range jobs {
			if firstErr.Load() != nil {
				continue
			}
			reporter.Report(ctx, Event{Phase: PhaseHydrate, CurrentPath: entry.RelativePath, TotalFiles: len(plan.DatalessEntries), DoneFiles: result.HydratedFiles, TotalBytes: plan.datalessBytes()})
			bytesRead, err := hydrateEntry(ctx, entry, fileTimeout, open)
			if err != nil {
				firstErr.Store(err)
				continue
			}
			atomic.AddInt64(&result.HydratedBytes, bytesRead)
			atomic.AddInt32((*int32)(&result.HydratedFiles), 1)
			reporter.Report(ctx, Event{Phase: PhaseHydrate, CurrentPath: entry.RelativePath, TotalFiles: len(plan.DatalessEntries), DoneFiles: result.HydratedFiles, DoneBytes: result.HydratedBytes, TotalBytes: plan.datalessBytes()})
		}
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go worker()
	}
	for _, entry := range plan.DatalessEntries {
		if firstErr.Load() != nil {
			break
		}
		select {
		case <-ctx.Done():
			firstErr.Store(ctx.Err())
			break
		case jobs <- entry:
		}
	}
	close(jobs)
	wg.Wait()
	if err, ok := firstErr.Load().(error); ok && err != nil {
		return result, err
	}
	return result, nil
}

func hydrateEntry(ctx context.Context, entry Entry, timeout time.Duration, open func(string) (io.ReadCloser, error)) (int64, error) {
	fileCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	reader, err := open(entry.SourcePath)
	if err != nil {
		return 0, fmt.Errorf("hydrate %s: %w", entry.RelativePath, err)
	}
	defer reader.Close()
	done := make(chan struct{})
	var bytesRead int64
	var readErr error
	go func() {
		bytesRead, readErr = io.Copy(io.Discard, reader)
		close(done)
	}()
	select {
	case <-fileCtx.Done():
		_ = reader.Close()
		return bytesRead, fmt.Errorf("hydrate %s timed out after %s", entry.RelativePath, timeout)
	case <-done:
		if readErr != nil {
			return bytesRead, fmt.Errorf("hydrate %s: %w", entry.RelativePath, readErr)
		}
		return bytesRead, nil
	}
}

func (p Plan) datalessBytes() int64 {
	var total int64
	for _, entry := range p.DatalessEntries {
		total += entry.Size
	}
	return total
}
```

If Go rejects atomic conversion for `HydratedFiles`, replace `HydrateResult` mutation with a mutex-protected result. Prefer clarity over clever atomics.

- [ ] **Step 4: Verify GREEN**

Run:

```bash
go test ./internal/storemigrate -run 'TestHydratePlan'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storemigrate/hydrate.go internal/storemigrate/hydrate_test.go internal/storemigrate/events.go
git commit -m "feat(store): add explicit migration hydration"
```

---

## Task 4: Replace Final-Destination Copy With Staging and Promotion

**Files:**
- Create: `internal/storemigrate/staging.go`
- Create: `internal/storemigrate/staging_test.go`
- Modify: `internal/storemigrate/copy.go`
- Modify: `internal/storemigrate/copy_test.go`

- [ ] **Step 1: Write failing staging tests**

Create `internal/storemigrate/staging_test.go`:

```go
package storemigrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewStagingPathIsSiblingOfDestination(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "LokiProfileManager")
	staging := NewStagingPath(dest, "abc123")
	if filepath.Dir(staging) != filepath.Dir(dest) {
		t.Fatalf("staging dir = %s, want sibling of %s", staging, dest)
	}
	if !strings.Contains(filepath.Base(staging), ".loki-migrate-abc123.staging") {
		t.Fatalf("staging = %s", staging)
	}
}

func TestPromoteStagingRenamesOnlyWhenDestinationMissing(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, ".loki-migrate-id.staging")
	dest := filepath.Join(root, "LokiProfileManager")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "marker.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PromoteStaging(staging, dest); err != nil {
		t.Fatalf("PromoteStaging() error = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "marker.txt")); err != nil || string(got) != "ok" {
		t.Fatalf("promoted marker got=%q err=%v", got, err)
	}
}

func TestPromoteStagingRejectsExistingDestination(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, ".loki-migrate-id.staging")
	dest := filepath.Join(root, "LokiProfileManager")
	if err := os.MkdirAll(staging, 0o755); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(dest, 0o755); err != nil { t.Fatal(err) }
	if err := PromoteStaging(staging, dest); err == nil || !strings.Contains(err.Error(), "destination already exists") {
		t.Fatalf("PromoteStaging(existing) error = %v", err)
	}
}
```

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/storemigrate -run 'TestNewStagingPath|TestPromoteStaging'
```

Expected: FAIL because staging functions are undefined.

- [ ] **Step 3: Implement staging helpers**

Create `internal/storemigrate/staging.go`:

```go
package storemigrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func NewStagingPath(destination, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		id = "manual"
	}
	return filepath.Join(filepath.Dir(filepath.Clean(destination)), ".loki-migrate-"+id+".staging")
}

func PromoteStaging(staging, destination string) error {
	if staging == "" || destination == "" {
		return fmt.Errorf("promote staging: staging and destination are required")
	}
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("promote staging: destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("promote staging: inspect destination %s: %w", destination, err)
	}
	if err := os.Rename(staging, destination); err != nil {
		return fmt.Errorf("promote staging %s to %s: %w", staging, destination, err)
	}
	return nil
}

func CleanupStaging(staging string) error {
	if strings.TrimSpace(staging) == "" {
		return nil
	}
	return os.RemoveAll(staging)
}
```

- [ ] **Step 4: Update copy API to write staging**

Modify `internal/storemigrate/copy.go` API:

```go
type CopyOptions struct {
	StagingPath string
	Reporter    Reporter
	FileTimeout time.Duration
}

func CopyPlan(ctx context.Context, plan Plan, opts CopyOptions) (CopyResult, error) {
	// destination safety recheck remains against plan.ToPath
	// actual file DestPath should be filepath.Join(opts.StagingPath, entry.RelativePath)
}
```

Inside the copy loop, compute each destination from staging path:

```go
stagedEntry := entry
stagedEntry.DestPath = filepath.Join(opts.StagingPath, filepath.FromSlash(entry.RelativePath))
```

Validate staging layout, not final destination:

```go
validation := store.ValidateLayout(opts.StagingPath)
```

Report progress before/after each file:

```go
reporter.Report(ctx, Event{Phase: PhaseCopy, CurrentPath: entry.RelativePath, DoneFiles: result.CopiedFiles, TotalFiles: plan.Summary.FileCount, DoneBytes: result.CopiedBytes, TotalBytes: plan.Summary.ByteCount})
```

- [ ] **Step 5: Update copy tests**

Change existing tests to call:

```go
staging := NewStagingPath(dest, "test")
result, err := CopyPlan(context.Background(), plan, CopyOptions{StagingPath: staging})
if err := PromoteStaging(staging, dest); err != nil { t.Fatal(err) }
```

Add a test that final destination stays missing until promotion:

```go
func TestCopyPlanWritesOnlyToStagingBeforePromotion(t *testing.T) {
	// Build source and plan.
	// Call CopyPlan with staging.
	// Assert final dest does not exist.
	// Assert staging validates.
}
```

- [ ] **Step 6: Verify GREEN**

Run:

```bash
go test ./internal/storemigrate
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/storemigrate/staging.go internal/storemigrate/staging_test.go internal/storemigrate/copy.go internal/storemigrate/copy_test.go
git commit -m "feat(store): stage migration copies before promotion"
```

---

## Task 5: Wire StoreMigrate Service Orchestration to Phases

**Files:**
- Modify: `internal/app/store_migrate.go`
- Modify: `internal/app/store_migrate_test.go`

- [ ] **Step 1: Write failing app tests for staged migration**

Add to `internal/app/store_migrate_test.go`:

```go
func TestStoreMigrateYesUsesStagingAndPromotesFinalStore(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	oldStore := filepath.Join(t.TempDir(), "old")
	if _, err := svc.EnsureStore(ctx, EnsureStoreRequest{StorePath: oldStore}); err != nil { t.Fatal(err) }
	writeStoreMigrateTestFile(t, filepath.Join(oldStore, "profiles", "work", "core", "files", "settings.json"), "old")
	dest := filepath.Join(t.TempDir(), "new")
	reporter := &storemigrate.MemoryReporter{}

	result, err := svc.StoreMigrate(ctx, StoreMigrateRequest{ToPath: dest, Yes: true, Progress: reporter})
	if err != nil { t.Fatalf("StoreMigrate() error = %v", err) }
	if !result.Switched || result.StagingPath == "" { t.Fatalf("result = %+v", result) }
	if _, err := os.Stat(result.StagingPath); !os.IsNotExist(err) { t.Fatalf("staging remains or stat err = %v", err) }
	if validation := store.ValidateLayout(dest); !validation.Valid { t.Fatalf("dest invalid: %+v", validation) }
	if len(reporter.Events()) == 0 { t.Fatal("no progress events") }
}
```

Add imports:

```go
"github.com/asudbring/loki-profile-manager/internal/storemigrate"
```

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
go test ./internal/app -run TestStoreMigrateYesUsesStagingAndPromotesFinalStore
```

Expected: FAIL because `StoreMigrateRequest.Progress` and `StoreMigrateResult.StagingPath` are undefined or staging not removed.

- [ ] **Step 3: Extend request/result**

Modify `internal/app/store_migrate.go`:

```go
type StoreMigrateRequest struct {
	FromPath        string
	ToPath          string
	Provider        store.ProviderType
	DryRun          bool
	Yes             bool
	CopyOnly        bool
	CaptureLocal    bool
	Hydrate         bool
	HydrateTimeout  time.Duration
	CopyTimeout     time.Duration
	Progress        storemigrate.Reporter
}

type StoreMigrateResult struct {
	// existing fields...
	StagingPath   string `json:"staging_path,omitempty"`
	HydratedFiles int    `json:"hydrated_files,omitempty"`
	HydratedBytes int64  `json:"hydrated_bytes,omitempty"`
}
```

- [ ] **Step 4: Implement phase orchestration**

Inside `StoreMigrate`:

```go
reporter := storemigrate.NewThrottledReporter(req.Progress, time.Second, time.Now)
plan, err := storemigrate.BuildPlan(storemigrate.PlanOptions{FromPath: fromPath, ToPath: toPath, Provider: req.Provider, AllowDataless: req.Hydrate})
result.Plan = plan
if err != nil { return result, err }

if req.Hydrate && plan.Summary.DatalessCount > 0 {
	hydrated, err := storemigrate.HydratePlan(ctx, plan, storemigrate.HydrateOptions{Reporter: reporter, FileTimeout: req.HydrateTimeout})
	result.HydratedFiles = hydrated.HydratedFiles
	result.HydratedBytes = hydrated.HydratedBytes
	if err != nil { return result, err }
	plan, err = storemigrate.BuildPlan(storemigrate.PlanOptions{FromPath: fromPath, ToPath: toPath, Provider: req.Provider})
	result.Plan = plan
	if err != nil { return result, err }
}
```

For `--yes` path:

```go
stagingID := time.Now().UTC().Format("20060102T150405Z")
stagingPath := storemigrate.NewStagingPath(toPath, stagingID)
result.StagingPath = stagingPath
copyResult, err := storemigrate.CopyPlan(ctx, plan, storemigrate.CopyOptions{StagingPath: stagingPath, Reporter: reporter, FileTimeout: req.CopyTimeout})
// on error: keep staging for inspection/resume and return error naming staging path
if err != nil { return result, err }
if err := storemigrate.PromoteStaging(stagingPath, toPath); err != nil { return result, err }
result.StagingPath = ""
```

Only after promotion should `rebaseAndPersistStorePath` run.

- [ ] **Step 5: Verify GREEN**

Run:

```bash
go test ./internal/app -run 'TestStoreMigrate'
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/store_migrate.go internal/app/store_migrate_test.go
git commit -m "feat(store): orchestrate staged migration phases"
```

---

## Task 6: Add CLI Progress, Hydrate, and Timeout Flags

**Files:**
- Modify: `internal/cli/store.go`
- Modify: `internal/cli/store_test.go`

- [ ] **Step 1: Write failing CLI tests**

Add to `internal/cli/store_test.go`:

```go
func TestStoreMigrateYesPrintsProgress(t *testing.T) {
	home := t.TempDir()
	storePath := filepath.Join(t.TempDir(), "old")
	cmd, _, _ := testCommandWithHome(home)
	cmd.SetArgs([]string{"store", "init", storePath})
	if err := cmd.Execute(); err != nil { t.Fatal(err) }
	dest := filepath.Join(t.TempDir(), "new")
	cmd, out, _ := testCommandWithHome(home)
	cmd.SetArgs([]string{"store", "migrate", "--to", dest, "--yes"})
	if err := cmd.Execute(); err != nil { t.Fatalf("migrate error = %v", err) }
	got := out.String()
	if !strings.Contains(got, "Preflight") || !strings.Contains(got, "Copy") || !strings.Contains(got, "Store migrated") {
		t.Fatalf("progress output missing: %s", got)
	}
}

func TestStoreMigrateHydrateFlagIsAccepted(t *testing.T) {
	cmd, _, _ := testCommand(t)
	cmd.SetArgs([]string{"store", "migrate", "--to", filepath.Join(t.TempDir(), "new"), "--dry-run", "--hydrate"})
	_ = cmd.Execute()
	// This test only proves Cobra accepts the flag; service path may fail because store not configured.
	if cmd.Flags().Lookup("hydrate") == nil {
		t.Fatal("hydrate flag missing")
	}
}
```

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/cli -run 'TestStoreMigrateYesPrintsProgress|TestStoreMigrateHydrateFlagIsAccepted'
```

Expected: FAIL because flags/progress output are missing.

- [ ] **Step 3: Add CLI flags**

In `newStoreMigrateCommand` add variables:

```go
var hydrate bool
var hydrateTimeout time.Duration
var copyTimeout time.Duration
var progressInterval time.Duration
```

Add flags:

```go
cmd.Flags().BoolVar(&hydrate, "hydrate", false, "download cloud-only source files before copying")
cmd.Flags().DurationVar(&hydrateTimeout, "hydrate-timeout", 30*time.Second, "maximum time to wait for one cloud-only file")
cmd.Flags().DurationVar(&copyTimeout, "file-timeout", 30*time.Second, "maximum time to wait for one copied file")
cmd.Flags().DurationVar(&progressInterval, "progress-interval", time.Second, "minimum time between progress updates")
```

Pass request fields:

```go
progress := newCLIStoreMigrateReporter(cmd.OutOrStdout(), progressInterval, jsonOutput)
result, err := svc.StoreMigrate(cmd.Context(), app.StoreMigrateRequest{
	FromPath: fromPath, ToPath: toPath, Provider: provider,
	DryRun: dryRun, Yes: yes, CopyOnly: copyOnly, CaptureLocal: captureLocal,
	Hydrate: hydrate, HydrateTimeout: hydrateTimeout, CopyTimeout: copyTimeout,
	Progress: progress,
})
```

- [ ] **Step 4: Implement human progress reporter**

Add helper in `internal/cli/store.go`:

```go
type cliStoreMigrateReporter struct {
	out io.Writer
	json bool
}

func newCLIStoreMigrateReporter(out io.Writer, interval time.Duration, jsonOutput bool) storemigrate.Reporter {
	base := &cliStoreMigrateReporter{out: out, json: jsonOutput}
	return storemigrate.NewThrottledReporter(base, interval, time.Now)
}

func (r *cliStoreMigrateReporter) Report(ctx context.Context, event storemigrate.Event) {
	if r.json {
		_ = json.NewEncoder(r.out).Encode(event)
		return
	}
	label := strings.Title(string(event.Phase))
	if event.TotalFiles > 0 {
		fmt.Fprintf(r.out, "%s: %d/%d files, %d/%d bytes", label, event.DoneFiles, event.TotalFiles, event.DoneBytes, event.TotalBytes)
	} else {
		fmt.Fprintf(r.out, "%s", label)
	}
	if event.CurrentPath != "" {
		fmt.Fprintf(r.out, " — %s", event.CurrentPath)
	}
	if event.Message != "" {
		fmt.Fprintf(r.out, " — %s", event.Message)
	}
	fmt.Fprintln(r.out)
}
```

Use `cases.Title(language.English)` later if desired; `strings.Title` is deprecated but acceptable for a quick internal label if lint does not block. If lint complains, use a small switch mapping phases to labels.

- [ ] **Step 5: Verify GREEN**

Run:

```bash
go test ./internal/cli -run 'TestStoreMigrate'
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/store.go internal/cli/store_test.go
git commit -m "feat(store): show migration progress in CLI"
```

---

## Task 7: Retarget Active Managed Symlinks After Rewire

**Files:**
- Create: `internal/activation/retarget.go`
- Create: `internal/activation/retarget_test.go`
- Modify: `internal/app/store_migrate.go`
- Modify: `internal/app/store_migrate_test.go`

- [ ] **Step 1: Write failing symlink retarget test**

Create `internal/activation/retarget_test.go`:

```go
package activation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetargetManagedSymlinksMovesOldStoreLinksToNewStore(t *testing.T) {
	ctx := context.Background()
	database := activationDB(t)
	defer database.Close()
	oldRoot := filepath.Join(t.TempDir(), "old")
	newRoot := filepath.Join(t.TempDir(), "new")
	oldSource := filepath.Join(oldRoot, "profiles", "work", "core", "files", "tool.toml")
	newSource := filepath.Join(newRoot, "profiles", "work", "core", "files", "tool.toml")
	target := filepath.Join(t.TempDir(), "tool.toml")
	writeFile(t, oldSource, "old")
	writeFile(t, newSource, "new")
	if err := os.Symlink(oldSource, target); err != nil { t.Skipf("symlink unavailable: %v", err) }
	if err := PutManagedTarget(ctx, database, ManagedTarget{TargetPath: target, SourcePath: newSource, Mode: string(OperationSymlink), ContentHash: "hash", LastAppliedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil { t.Fatal(err) }

	changed, err := RetargetManagedSymlinks(ctx, database, oldRoot, newRoot)
	if err != nil { t.Fatalf("RetargetManagedSymlinks() error = %v", err) }
	if changed != 1 { t.Fatalf("changed = %d, want 1", changed) }
	link, err := os.Readlink(target)
	if err != nil { t.Fatal(err) }
	if link != newSource { t.Fatalf("link = %q, want %q", link, newSource) }
}
```

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
go test ./internal/activation -run TestRetargetManagedSymlinks
```

Expected: FAIL because `RetargetManagedSymlinks` is undefined.

- [ ] **Step 3: Implement symlink retarget helper**

Create `internal/activation/retarget.go`:

```go
package activation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func RetargetManagedSymlinks(ctx context.Context, database *sql.DB, oldRoot, newRoot string) (int, error) {
	records, err := ListManagedTargets(ctx, database)
	if err != nil { return 0, err }
	changed := 0
	for _, record := range records {
		if record.Mode != string(OperationSymlink) { continue }
		linkTarget, err := os.Readlink(record.TargetPath)
		if os.IsNotExist(err) { continue }
		if err != nil { return changed, fmt.Errorf("read managed symlink %s: %w", record.TargetPath, err) }
		nextTarget, ok, err := rebasePathUnderRoot(linkTarget, oldRoot, newRoot)
		if err != nil { return changed, err }
		if !ok || nextTarget == linkTarget { continue }
		if record.SourcePath != "" && filepath.Clean(record.SourcePath) != filepath.Clean(nextTarget) {
			return changed, fmt.Errorf("managed symlink %s source mismatch: record=%s link=%s", record.TargetPath, record.SourcePath, nextTarget)
		}
		if err := os.Remove(record.TargetPath); err != nil { return changed, fmt.Errorf("remove symlink %s: %w", record.TargetPath, err) }
		if err := os.Symlink(nextTarget, record.TargetPath); err != nil { return changed, fmt.Errorf("retarget symlink %s: %w", record.TargetPath, err) }
		changed++
	}
	return changed, nil
}
```

Add missing import `database/sql`.

- [ ] **Step 4: Call retarget after DB rebase**

In `internal/app/store_migrate.go`, after successful transaction rewire:

```go
retargeted, err := activation.RetargetManagedSymlinks(ctx, s.database, fromPath, toPath)
if err != nil { return err }
result.RetargetedSymlinks = retargeted
```

Add `RetargetedSymlinks int` to `StoreMigrateResult`.

- [ ] **Step 5: Verify GREEN**

Run:

```bash
go test ./internal/activation -run TestRetargetManagedSymlinks
go test ./internal/app -run TestStoreMigrate
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/activation/retarget.go internal/activation/retarget_test.go internal/app/store_migrate.go internal/app/store_migrate_test.go
git commit -m "feat(store): retarget active migration symlinks"
```

---

## Task 8: Add Cancel Cleanup and Clear Error Messages

**Files:**
- Modify: `internal/app/store_migrate.go`
- Modify: `internal/storemigrate/staging.go`
- Modify: `internal/app/store_migrate_test.go`
- Modify: `internal/cli/store.go`

- [ ] **Step 1: Write failing cancellation test**

Add to `internal/app/store_migrate_test.go`:

```go
func TestStoreMigrateCancelReportsStagingPathAndRemovesLock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	svc := testService(t)
	defer svc.Close()
	oldStore := filepath.Join(t.TempDir(), "old")
	if _, err := svc.EnsureStore(context.Background(), EnsureStoreRequest{StorePath: oldStore}); err != nil { t.Fatal(err) }
	dest := filepath.Join(t.TempDir(), "new")
	cancel()

	result, err := svc.StoreMigrate(ctx, StoreMigrateRequest{ToPath: dest, Yes: true})
	if err == nil { t.Fatal("StoreMigrate(cancelled) error = nil") }
	if result.StagingPath == "" { t.Fatalf("result missing staging path: %+v", result) }
	if _, statErr := os.Stat(store.OperationLockPath(oldStore)); !os.IsNotExist(statErr) {
		t.Fatalf("lock remains or stat err = %v", statErr)
	}
}
```

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
go test ./internal/app -run TestStoreMigrateCancelReportsStagingPathAndRemovesLock
```

Expected: FAIL if context is not checked early or staging path is absent.

- [ ] **Step 3: Add explicit context checks at phase boundaries**

In app orchestration and copy/hydrate loops, before each major phase:

```go
if err := ctx.Err(); err != nil {
	return result, fmt.Errorf("store migrate cancelled before copy; staging preserved at %s: %w", result.StagingPath, err)
}
```

In copy loop:

```go
select {
case <-ctx.Done():
	return result, fmt.Errorf("store migrate cancelled during copy; staging preserved at %s: %w", opts.StagingPath, ctx.Err())
default:
}
```

- [ ] **Step 4: Ensure lock cleanup relies on existing defer**

Do not remove operation-lock code. `store.WithOperationLock` already defers unlock. Tests should confirm cancellation returns out of lock callback.

- [ ] **Step 5: Verify GREEN**

Run:

```bash
go test ./internal/app -run 'TestStoreMigrateCancel|TestStoreMigrate'
go test ./internal/storemigrate
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/store_migrate.go internal/app/store_migrate_test.go internal/storemigrate/copy.go internal/storemigrate/hydrate.go internal/cli/store.go
git commit -m "fix(store): make migration cancellation explicit"
```

---

## Task 9: Add `--cleanup` for Interrupted Staging Directories

**Files:**
- Modify: `internal/app/store_migrate.go`
- Modify: `internal/cli/store.go`
- Modify: `internal/cli/store_test.go`
- Modify: `docs/USAGE.md`

- [ ] **Step 1: Write failing CLI cleanup test**

Add to `internal/cli/store_test.go`:

```go
func TestStoreMigrateCleanupRemovesStagingDirectory(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, ".loki-migrate-test.staging")
	if err := os.MkdirAll(staging, 0o755); err != nil { t.Fatal(err) }
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"store", "migrate", "--cleanup", staging})
	if err := cmd.Execute(); err != nil { t.Fatalf("cleanup error = %v", err) }
	if _, err := os.Stat(staging); !os.IsNotExist(err) { t.Fatalf("staging remains or stat err = %v", err) }
	if !strings.Contains(out.String(), "Migration staging cleaned") { t.Fatalf("output = %s", out.String()) }
}
```

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
go test ./internal/cli -run TestStoreMigrateCleanupRemovesStagingDirectory
```

Expected: FAIL because `--cleanup` does not exist.

- [ ] **Step 3: Add app cleanup request**

In `internal/app/store_migrate.go`:

```go
type StoreMigrateCleanupRequest struct { StagingPath string }
type StoreMigrateCleanupResult struct { StagingPath string `json:"staging_path"`; Removed bool `json:"removed"` }

func (s *Service) StoreMigrateCleanup(ctx context.Context, req StoreMigrateCleanupRequest) (StoreMigrateCleanupResult, error) {
	if strings.TrimSpace(req.StagingPath) == "" { return StoreMigrateCleanupResult{}, fmt.Errorf("store migrate cleanup: staging path is required") }
	if !strings.Contains(filepath.Base(req.StagingPath), ".loki-migrate-") || !strings.HasSuffix(filepath.Base(req.StagingPath), ".staging") {
		return StoreMigrateCleanupResult{}, fmt.Errorf("store migrate cleanup: refusing non-migration staging path: %s", req.StagingPath)
	}
	if err := storemigrate.CleanupStaging(req.StagingPath); err != nil { return StoreMigrateCleanupResult{}, err }
	return StoreMigrateCleanupResult{StagingPath: req.StagingPath, Removed: true}, nil
}
```

- [ ] **Step 4: Add CLI cleanup mode**

In `newStoreMigrateCommand`, add:

```go
var cleanupPath string
cmd.Flags().StringVar(&cleanupPath, "cleanup", "", "remove an interrupted Loki migration staging directory")
```

At command start:

```go
if cleanupPath != "" {
	result, err := svc.StoreMigrateCleanup(cmd.Context(), app.StoreMigrateCleanupRequest{StagingPath: cleanupPath})
	// print JSON or human result
	return nil
}
```

Cleanup mode must not require `--to`, `--dry-run`, or `--yes`.

- [ ] **Step 5: Verify GREEN**

Run:

```bash
go test ./internal/cli -run 'TestStoreMigrateCleanup|TestStoreMigrate'
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/store_migrate.go internal/cli/store.go internal/cli/store_test.go docs/USAGE.md
git commit -m "feat(store): clean interrupted migration staging"
```

---

## Task 10: Documentation and Release Notes

**Files:**
- Modify: `README.md`
- Modify: `docs/USAGE.md`
- Modify: `docs/INSTALL.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Update user docs**

In `docs/USAGE.md`, replace current `loki store migrate` behavior section with:

```markdown
### `loki store migrate`

```bash
loki store migrate --to <path> --dry-run
loki store migrate --to <path> --yes
loki store migrate --to <path> --yes --hydrate
loki store migrate --cleanup <staging-path>
```

Behavior:

- `--dry-run` is a fast preflight. It counts files, bytes, cloud-only files, conflicts, and destination state.
- If macOS OneDrive/Dropbox reports cloud-only source files, migration stops before copy unless `--hydrate` is set.
- `--hydrate` downloads source cloud-only files first and prints progress.
- `--yes` copies to a hidden sibling staging directory, validates the staged store, then promotes it to the final destination.
- If interrupted, Loki prints the staging path. Re-run with `--cleanup <staging-path>` to remove it.
- After promotion, Loki rewires local SQLite state and retargets active Loki-managed symlinks to the new store.
```

- [ ] **Step 2: Update install docs for OneDrive Business**

In `docs/INSTALL.md`, add:

```markdown
If dry-run reports cloud-only files, open the source store in Finder and choose **Always Keep on This Device**, or rerun with `--hydrate` to let Loki materialize source files with progress. Hydration time depends on your sync provider and network, not local copy speed.
```

- [ ] **Step 3: Update architecture docs**

In `docs/ARCHITECTURE.md`, describe:

- Preflight.
- Hydration.
- Staging.
- Promotion.
- Rewire and symlink retarget.
- Progress events.

- [ ] **Step 4: Update changelog**

Add under `## Unreleased`:

```markdown
- Reworked `loki store migrate` into a staged, observable migration pipeline with cloud-only source-file preflight, optional hydration, copy progress, staging cleanup, and active symlink retargeting.
```

- [ ] **Step 5: Verify docs grep**

Run:

```bash
rg -n "--hydrate|--cleanup|cloud-only|staging|retarget" README.md docs/USAGE.md docs/INSTALL.md docs/ARCHITECTURE.md CHANGELOG.md
```

Expected: all terms appear in appropriate docs.

- [ ] **Step 6: Commit**

```bash
git add README.md docs/USAGE.md docs/INSTALL.md docs/ARCHITECTURE.md CHANGELOG.md
git commit -m "docs(store): document migration v2 flow"
```

---

## Task 11: Full Validation and Local Dogfood

**Files:**
- No code changes expected.

- [ ] **Step 1: Run full automated validation**

Run:

```bash
go test ./...
git diff --check
go vet ./...
go mod verify
```

Expected:

```text
all packages pass
all modules verified
```

- [ ] **Step 2: Build local binary**

Run:

```bash
go build -trimpath -ldflags "-X github.com/asudbring/loki-profile-manager/internal/app.Version=store-migrate-v2-test" -o dist/loki-migrate-v2-test ./cmd/loki
./dist/loki-migrate-v2-test --version
```

Expected:

```text
store-migrate-v2-test
```

- [ ] **Step 3: Run disposable fast migration smoke**

Run:

```bash
tmp=$(mktemp -d)
old="$tmp/old"
new="$tmp/new"
HOME="$tmp/home" ./dist/loki-migrate-v2-test store init "$old"
HOME="$tmp/home" ./dist/loki-migrate-v2-test store migrate --from "$old" --to "$new" --dry-run
HOME="$tmp/home" ./dist/loki-migrate-v2-test store migrate --from "$old" --to "$new" --yes
HOME="$tmp/home" ./dist/loki-migrate-v2-test --store "$new" store status
```

Expected:

```text
Store migration dry-run
Copy: ... progress lines ...
Store migrated
Valid: yes
```

- [ ] **Step 4: Run real-source dry-run on Allen's OneDrive Personal store**

Run:

```bash
OLD="$HOME/Library/CloudStorage/OneDrive-Personal/LokiProfileManager"
NEW="$HOME/Library/CloudStorage/OneDrive-SudbringLab/LokiProfileManager"
./dist/loki-migrate-v2-test store migrate --from "$OLD" --to "$NEW" --provider onedrive-business --dry-run
```

Expected if source still has cloud-only files:

```text
Preflight: ...
Cloud-only files: <n>
Migration requires hydration. Rerun with --hydrate or materialize the source store in Finder.
```

- [ ] **Step 5: Decide dogfood hydration path**

If user approves CLI hydration:

```bash
./dist/loki-migrate-v2-test store migrate --from "$OLD" --to "$NEW" --provider onedrive-business --yes --hydrate --progress-interval 1s
```

If user prefers Finder hydration:

1. In Finder, right-click the source store folder.
2. Choose **Always Keep on This Device**.
3. Wait for sync provider to finish.
4. Rerun `--dry-run`; cloud-only count must be zero.
5. Run `--yes` without `--hydrate`.

- [ ] **Step 6: Commit validation note if docs changed during dogfood**

Only if dogfood reveals doc changes:

```bash
git add docs/INSTALL.md docs/USAGE.md CHANGELOG.md
git commit -m "docs(store): record migration dogfood notes"
```

---

## Task 12: Release V2 Patch

**Files:**
- Modify: `CHANGELOG.md`
- Modify: `README.md`

- [ ] **Step 1: Choose version**

Use next patch after current npm version. At plan time current npm latest is `0.1.9`, so use:

```text
v0.1.10
```

- [ ] **Step 2: Update README and changelog release headers**

In `README.md`:

```markdown
- Current npm release: `v0.1.10`; latest full Windows app/manual dogfood validation: `v0.1.7`.
```

In `CHANGELOG.md`, convert Unreleased bullets into:

```markdown
## v0.1.10 — 2026-05-15

Patch release for staged, observable store migration.

- Reworked `loki store migrate` into a staged migration pipeline with fast preflight, optional cloud-only source hydration, copy progress, staging cleanup, and active symlink retargeting.
```

- [ ] **Step 3: Run release validation**

Run:

```bash
go test ./...
git diff --check
go vet ./...
go mod verify
./scripts/release-local.sh v0.1.10
```

Expected: release assets built and checksums verified.

- [ ] **Step 4: Commit release notes**

```bash
git add README.md CHANGELOG.md
git commit -m "docs: add v0.1.10 changelog"
```

- [ ] **Step 5: Push and tag**

```bash
git push origin main
git tag v0.1.10
git push origin v0.1.10
```

- [ ] **Step 6: Watch workflows**

Run:

```bash
gh run list --limit 5
gh run watch <release-run-id> --exit-status
npm view @asudbring/loki-profile-manager version
```

Expected:

```text
Release workflow: success
npm version: 0.1.10
```

---

## Risk Register

| Risk | Impact | Mitigation |
|---|---|---|
| OneDrive hydration still slow | User thinks app hung | Explicit hydrate phase, progress, current file, timeout. |
| File timeout cannot interrupt kernel/file-provider read | Worker goroutine may linger | Close file on timeout, keep concurrency low, report exact file, allow Ctrl+C. |
| Final destination gets partial store | Data confusion | Copy only to staging; promote final by rename after validation. |
| Interrupted staging consumes space | User confusion | Print staging path and add `--cleanup`. |
| Active symlinks still point old store | Migration appears successful but tools use old store | Retarget managed symlinks after DB rebase; test it. |
| JSON automation breaks if `--json` streams events | Existing scripts may expect one JSON object | Prefer human progress by default and add `--json-stream` if compatibility becomes concern. If `--json` must stream, document it as changed for write-mode only. |
| Cross-platform symlink tests fail on Windows | CI failures | Skip symlink retarget tests when symlink creation unavailable. |

## Acceptance Criteria

- `loki store migrate --dry-run` completes quickly for OneDrive stores and reports cloud-only file count.
- `loki store migrate --yes` prints visible progress at least once per second by default.
- No command writes partial content to the final destination path.
- Interrupt leaves no operation lock and reports staging cleanup instructions.
- `--hydrate` can materialize cloud-only files with progress and timeout.
- After migration, `loki store status` points at new store.
- Managed target source paths and metadata source paths point at new store.
- Active Loki-managed symlinks point at new store.
- `go test ./...`, `go vet ./...`, `go mod verify`, and release packaging pass.
