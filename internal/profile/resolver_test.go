package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/store"
)

func TestResolveLayerOrder(t *testing.T) {
	root := testProfileStore(t)
	writeBucket(t, root, "work", "content-dev")
	writeBucket(t, root, "work", "azure")
	layers, err := Resolve(root, "work", []string{"content-dev", "azure"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	want := []string{"common", "work-core", "content-dev", "azure"}
	if len(layers) != len(want) {
		t.Fatalf("layers = %d, want %d", len(layers), len(want))
	}
	for i, layer := range layers {
		if layer.Name != want[i] || layer.Order != i {
			t.Fatalf("layer[%d] = %+v, want name %s", i, layer, want[i])
		}
	}
}

func TestResolveUnknownParentBucket(t *testing.T) {
	root := testProfileStore(t)
	if _, err := Resolve(root, "missing", nil); err == nil {
		t.Fatal("Resolve() unknown parent error = nil")
	}
	if _, err := Resolve(root, "work", []string{"missing"}); err == nil {
		t.Fatal("Resolve() unknown bucket error = nil")
	}
}

func TestDiscoverParentsBuckets(t *testing.T) {
	root := testProfileStore(t)
	writeBucket(t, root, "work", "content-dev")
	parents, err := DiscoverParents(root)
	if err != nil {
		t.Fatalf("DiscoverParents() error = %v", err)
	}
	if len(parents) != 3 {
		t.Fatalf("parents = %+v", parents)
	}
	buckets, err := DiscoverBuckets(root, "work")
	if err != nil {
		t.Fatalf("DiscoverBuckets() error = %v", err)
	}
	if len(buckets) != 1 || buckets[0] != "content-dev" {
		t.Fatalf("buckets = %+v", buckets)
	}
}

func TestResolveRejectsPathNames(t *testing.T) {
	root := testProfileStore(t)
	for _, parent := range []string{"../evil", `..\evil`, "/evil", `C:\evil`, "evil/sub", ".", ".."} {
		if _, err := Resolve(root, parent, nil); err == nil {
			t.Fatalf("Resolve(%q) error = nil", parent)
		}
	}
	for _, bucket := range []string{"../evil", `..\evil`, "/evil", `C:\evil`, "evil/sub", ".", ".."} {
		if _, err := Resolve(root, "work", []string{bucket}); err == nil {
			t.Fatalf("Resolve(bucket %q) error = nil", bucket)
		}
	}
	if _, err := DiscoverBuckets(root, "../evil"); err == nil {
		t.Fatal("DiscoverBuckets() path parent error = nil")
	}
}

func testProfileStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	return root
}

func writeBucket(t *testing.T, root, parent, bucket string) {
	t.Helper()
	dir := filepath.Join(root, "profiles", parent, "buckets", bucket)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	content := "version: 1\nname: " + bucket + "\nfiles: []\nskills: []\nignore: []\nmerge_rules: {}\ntargets: {}\n"
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
