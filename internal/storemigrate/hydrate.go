package storemigrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// HydrateOptions controls explicit materialization of cloud-only source files.
type HydrateOptions struct {
	Plan        Plan
	Reporter    Reporter
	FileTimeout time.Duration
	HydrateFile func(context.Context, Entry) (int64, error)
}

// HydrateResult summarizes source materialization.
type HydrateResult struct {
	HydratedFiles int   `json:"hydrated_files"`
	HydratedBytes int64 `json:"hydrated_bytes"`
}

// HydratePlan explicitly reads cloud-only entries so providers materialize them before copy.
func HydratePlan(ctx context.Context, opts HydrateOptions) (HydrateResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !opts.Plan.CanMigrate {
		return HydrateResult{}, fmt.Errorf("store migrate: plan is not migratable")
	}
	hydrateFile := opts.HydrateFile
	if hydrateFile == nil {
		hydrateFile = hydrateFileFromDisk
	}
	entries := opts.Plan.DatalessEntries
	totalFiles := len(entries)
	var totalBytes int64
	for _, entry := range entries {
		totalBytes += entry.Size
	}
	report(ctx, opts.Reporter, Event{Phase: PhaseHydrate, Message: "hydrating cloud-only files", TotalFiles: totalFiles, TotalBytes: totalBytes})
	var result HydrateResult
	for _, entry := range entries {
		if entry.Kind != "file" {
			continue
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		report(ctx, opts.Reporter, Event{Phase: PhaseHydrate, Message: "hydrating", CurrentPath: entry.RelativePath, DoneFiles: result.HydratedFiles, TotalFiles: totalFiles, DoneBytes: result.HydratedBytes, TotalBytes: totalBytes})
		bytes, err := runHydrateFile(ctx, opts.FileTimeout, entry, hydrateFile)
		if err != nil {
			return result, err
		}
		result.HydratedFiles++
		result.HydratedBytes += bytes
		report(ctx, opts.Reporter, Event{Phase: PhaseHydrate, Message: "hydrated", CurrentPath: entry.RelativePath, DoneFiles: result.HydratedFiles, TotalFiles: totalFiles, DoneBytes: result.HydratedBytes, TotalBytes: totalBytes})
	}
	return result, nil
}

func runHydrateFile(ctx context.Context, timeout time.Duration, entry Entry, hydrateFile func(context.Context, Entry) (int64, error)) (int64, error) {
	return runFileOperationWithTimeout(ctx, timeout, entry.RelativePath, "hydrate", func(ctx context.Context) (int64, error) {
		return hydrateFile(ctx, entry)
	})
}

func runFileOperationWithTimeout(ctx context.Context, timeout time.Duration, relativePath, operation string, fn func(context.Context) (int64, error)) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		bytes, err := fn(ctx)
		if err != nil {
			return bytes, fmt.Errorf("%s %s: %w", operation, relativePath, err)
		}
		return bytes, nil
	}
	fileCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type outcome struct {
		bytes int64
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		bytes, err := fn(fileCtx)
		done <- outcome{bytes: bytes, err: err}
	}()
	select {
	case result := <-done:
		if result.err != nil {
			if errors.Is(result.err, context.DeadlineExceeded) || fileCtx.Err() == context.DeadlineExceeded {
				return result.bytes, fmt.Errorf("%s %s timed out after %s", operation, relativePath, timeout)
			}
			return result.bytes, fmt.Errorf("%s %s: %w", operation, relativePath, result.err)
		}
		if fileCtx.Err() == context.DeadlineExceeded {
			return result.bytes, fmt.Errorf("%s %s timed out after %s", operation, relativePath, timeout)
		}
		return result.bytes, nil
	case <-fileCtx.Done():
		if errors.Is(fileCtx.Err(), context.DeadlineExceeded) {
			return 0, fmt.Errorf("%s %s timed out after %s", operation, relativePath, timeout)
		}
		return 0, fileCtx.Err()
	}
}

func hydrateFileFromDisk(ctx context.Context, entry Entry) (int64, error) {
	file, err := os.Open(entry.SourcePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	return copyWithContextAndCloseOnCancel(ctx, io.Discard, file, file)
}
