package log

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/config"
)

func TestLoggerWritesRedactedLogFile(t *testing.T) {
	tmp := t.TempDir()
	paths := config.LocalPaths{
		LogDir:  filepath.Join(tmp, "logs"),
		LogPath: filepath.Join(tmp, "logs", "loki.log"),
	}
	logger, err := NewLogger(paths, Options{Redactor: NewRedactor("abc123")})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	logger.Slog().InfoContext(context.Background(), "using abc123", "token", "abc123")
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	content, err := os.ReadFile(paths.LogPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(content), "abc123") {
		t.Fatalf("secret remained in log: %s", content)
	}
	if !strings.Contains(string(content), Redacted) {
		t.Fatalf("redacted marker missing in log: %s", content)
	}
}

func TestVerboseLoggerWritesSanitizedTerminalOutput(t *testing.T) {
	tmp := t.TempDir()
	paths := config.LocalPaths{
		LogDir:  filepath.Join(tmp, "logs"),
		LogPath: filepath.Join(tmp, "logs", "loki.log"),
	}
	var stderr bytes.Buffer
	logger, err := NewLogger(paths, Options{Verbose: true, TerminalWriter: &stderr, Redactor: NewRedactor("abc123")})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	logger.Slog().InfoContext(context.Background(), "status abc123", "client_secret", "abc123")
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	got := stderr.String()
	if strings.Contains(got, "abc123") {
		t.Fatalf("secret remained in stderr: %s", got)
	}
	if !strings.Contains(got, Redacted) {
		t.Fatalf("redacted marker missing in stderr: %s", got)
	}
}
