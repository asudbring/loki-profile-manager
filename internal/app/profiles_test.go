package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/store"
)

func TestProfileCatalogUsesConfiguredStore(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)
	defer svc.Close()
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := svc.EnsureStore(ctx, EnsureStoreRequest{StorePath: storePath}); err != nil {
		t.Fatalf("EnsureStore() error = %v", err)
	}
	writeProfileCatalogBucket(t, storePath, "work", "content-dev")
	writeProfileCatalogBucket(t, storePath, "work", "azure")

	catalog, err := svc.ProfileCatalog(ctx, ProfileCatalogRequest{})
	if err != nil {
		t.Fatalf("ProfileCatalog() error = %v", err)
	}
	if catalog.StorePath == "" || len(catalog.Profiles) != 3 {
		t.Fatalf("catalog = %+v", catalog)
	}
	work := findProfileSummary(catalog, "work")
	if work == nil {
		t.Fatalf("work profile missing: %+v", catalog.Profiles)
	}
	if got := bucketNames(work.Buckets); strings.Join(got, ",") != "azure,content-dev" {
		t.Fatalf("work buckets = %+v", got)
	}
}

func TestProfileCatalogUsesExplicitStore(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}

	catalog, err := svc.ProfileCatalog(context.Background(), ProfileCatalogRequest{StorePath: storePath})
	if err != nil {
		t.Fatalf("ProfileCatalog() error = %v", err)
	}
	if catalog.StorePath == "" || len(catalog.Profiles) != 3 {
		t.Fatalf("catalog = %+v", catalog)
	}
}

func TestProfileCatalogRejectsInvalidStore(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	_, err := svc.ProfileCatalog(context.Background(), ProfileCatalogRequest{StorePath: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "invalid store layout") {
		t.Fatalf("ProfileCatalog() error = %v", err)
	}
}

func TestProfileCatalogAllowsParentWithoutBucketsDirectory(t *testing.T) {
	svc := testService(t)
	defer svc.Close()
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	coreDir := filepath.Join(storePath, "profiles", "solo", "core")
	if err := os.MkdirAll(coreDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeAppFile(t, filepath.Join(coreDir, "manifest.yaml"), "version: 1\nname: solo-core\nfiles: []\nskills: []\nignore: []\nmerge_rules: {}\ntargets: {}\n")

	catalog, err := svc.ProfileCatalog(context.Background(), ProfileCatalogRequest{StorePath: storePath})
	if err != nil {
		t.Fatalf("ProfileCatalog() error = %v", err)
	}
	solo := findProfileSummary(catalog, "solo")
	if solo == nil || len(solo.Buckets) != 0 {
		t.Fatalf("solo profile = %+v", solo)
	}
}

func writeProfileCatalogBucket(t *testing.T, root, parent, bucket string) {
	t.Helper()
	dir := filepath.Join(root, "profiles", parent, "buckets", bucket)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeAppFile(t, filepath.Join(dir, "manifest.yaml"), "version: 1\nname: "+bucket+"\nfiles: []\nskills: []\nignore: []\nmerge_rules: {}\ntargets: {}\n")
}

func findProfileSummary(catalog ProfileCatalogResult, name string) *ProfileSummary {
	for i := range catalog.Profiles {
		if catalog.Profiles[i].Name == name {
			return &catalog.Profiles[i]
		}
	}
	return nil
}

func bucketNames(buckets []BucketSummary) []string {
	names := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		names = append(names, bucket.Name)
	}
	return names
}
