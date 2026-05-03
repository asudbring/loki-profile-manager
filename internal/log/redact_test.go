package log

import (
	"log/slog"
	"strings"
	"testing"
)

func TestSensitiveKeysRedacted(t *testing.T) {
	redactor := NewRedactor()
	attr := redactor.RedactAttr(slog.String("token", "secret-token"))
	if attr.Value.String() != Redacted {
		t.Fatalf("redacted token = %q, want %q", attr.Value.String(), Redacted)
	}
}

func TestRegisteredSecretValueRedactedInString(t *testing.T) {
	redactor := NewRedactor("abc123")
	got := redactor.RedactString("command failed with token abc123")
	if strings.Contains(got, "abc123") {
		t.Fatalf("secret remained in %q", got)
	}
	if got != "command failed with token "+Redacted {
		t.Fatalf("RedactString() = %q", got)
	}
}

func TestNonSensitiveValueUnchanged(t *testing.T) {
	redactor := NewRedactor("abc123")
	got := redactor.RedactString("plain text")
	if got != "plain text" {
		t.Fatalf("RedactString() = %q", got)
	}
}

func TestRedactAttrsDoesNotMutateInput(t *testing.T) {
	redactor := NewRedactor()
	attrs := []slog.Attr{slog.String("password", "super-secret")}
	redacted := redactor.RedactAttrs(attrs)
	if attrs[0].Value.String() != "super-secret" {
		t.Fatalf("input mutated: %q", attrs[0].Value.String())
	}
	if redacted[0].Value.String() != Redacted {
		t.Fatalf("redacted = %q", redacted[0].Value.String())
	}
}
