package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestKVSetGetDelete(t *testing.T) {
	database, err := Bootstrap(context.Background(), filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	defer database.Close()

	if _, ok, err := GetKV(context.Background(), database, "store_path"); err != nil || ok {
		t.Fatalf("initial GetKV() = ok %v err %v, want missing", ok, err)
	}
	if err := SetKV(context.Background(), database, "store_path", "/tmp/loki"); err != nil {
		t.Fatalf("SetKV() error = %v", err)
	}
	value, ok, err := GetKV(context.Background(), database, "store_path")
	if err != nil || !ok || value != "/tmp/loki" {
		t.Fatalf("GetKV() = value %q ok %v err %v", value, ok, err)
	}
	if err := DeleteKV(context.Background(), database, "store_path"); err != nil {
		t.Fatalf("DeleteKV() error = %v", err)
	}
	if _, ok, err := GetKV(context.Background(), database, "store_path"); err != nil || ok {
		t.Fatalf("final GetKV() = ok %v err %v, want missing", ok, err)
	}
}
