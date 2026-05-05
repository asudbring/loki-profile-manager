package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunRejectsNilClient(t *testing.T) {
	err := Run(context.Background(), nil, Options{AllowNonTTY: true})
	if err == nil || !strings.Contains(err.Error(), "client is nil") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunRejectsNonTTY(t *testing.T) {
	err := Run(context.Background(), fakeClient{}, Options{Input: strings.NewReader("q"), Output: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("Run() error = %v", err)
	}
}
