package storesync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanConflictsFindsProviderConflictCopies(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "profiles", "work", "prefs conflict copy.yaml"), "b")
	writeTestFile(t, filepath.Join(root, "profiles", "work", "settings conflicted copy.json"), "a")
	writeTestFile(t, filepath.Join(root, "profiles", "work", "normal conflict notes.md"), "c")

	result, err := ScanConflicts(ConflictScanOptions{Root: root})
	if err != nil {
		t.Fatalf("ScanConflicts() error = %v", err)
	}
	if len(result.Conflicts) != 2 {
		t.Fatalf("conflicts = %+v", result.Conflicts)
	}
	if result.Conflicts[0].RelativePath != "profiles/work/prefs conflict copy.yaml" || result.Conflicts[1].RelativePath != "profiles/work/settings conflicted copy.json" {
		t.Fatalf("conflicts not sorted or wrong: %+v", result.Conflicts)
	}
	for _, conflict := range result.Conflicts {
		if conflict.Action != ConflictActionDelete || conflict.Kind != "file" {
			t.Fatalf("conflict = %+v, want file delete", conflict)
		}
	}
}

func TestScanConflictsSkipsBroadCaseConflictFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "case conflict notes.md"), "manual")

	result, err := ScanConflicts(ConflictScanOptions{Root: root})
	if err != nil {
		t.Fatalf("ScanConflicts() error = %v", err)
	}
	if len(result.Conflicts) != 1 || result.Conflicts[0].Action != ConflictActionSkip || result.Conflicts[0].Reason == "" {
		t.Fatalf("conflicts = %+v", result.Conflicts)
	}
}

func TestScanConflictsSkipsDirectories(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "profiles", "work conflicted copy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeTestFile(t, filepath.Join(dir, "inside.txt"), "content")

	result, err := ScanConflicts(ConflictScanOptions{Root: root})
	if err != nil {
		t.Fatalf("ScanConflicts() error = %v", err)
	}
	if len(result.Conflicts) != 1 || result.Conflicts[0].Action != ConflictActionSkip || result.Conflicts[0].Kind != "directory" {
		t.Fatalf("conflicts = %+v", result.Conflicts)
	}
}

func TestScanConflictsLimit(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a conflicted copy.txt", "b conflicted copy.txt", "c conflicted copy.txt"} {
		writeTestFile(t, filepath.Join(root, name), "content")
	}
	result, err := ScanConflicts(ConflictScanOptions{Root: root, Limit: 2})
	if err != nil {
		t.Fatalf("ScanConflicts() error = %v", err)
	}
	if len(result.Conflicts) != 2 || !result.Truncated {
		t.Fatalf("result = %+v", result)
	}
}

func TestScanConflictsLimitIncludesSkippedEntries(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a case conflict", "b case conflict", "c case conflict"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
	}
	result, err := ScanConflicts(ConflictScanOptions{Root: root, Limit: 2})
	if err != nil {
		t.Fatalf("ScanConflicts() error = %v", err)
	}
	if len(result.Conflicts) != 2 || !result.Truncated {
		t.Fatalf("result = %+v", result)
	}
}

func TestIsConflictCopyNameUsesHostHints(t *testing.T) {
	if !IsConflictCopyName("settings Allen-Mac conflict.json", ConflictScanOptions{Hostname: "Allen-Mac"}) {
		t.Fatal("host conflict name not detected")
	}
	if IsConflictCopyName("conflict-resolution.md", ConflictScanOptions{}) {
		t.Fatal("generic conflict name detected unexpectedly")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
