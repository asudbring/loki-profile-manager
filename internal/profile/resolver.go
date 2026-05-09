package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/asudbring/loki-profile-manager/internal/manifest"
)

type LayerKind string

const (
	LayerCommon LayerKind = "common"
	LayerCore   LayerKind = "core"
	LayerBucket LayerKind = "bucket"
)

type Layer struct {
	Name         string            `json:"name"`
	Kind         LayerKind         `json:"kind"`
	Parent       string            `json:"parent,omitempty"`
	Bucket       string            `json:"bucket,omitempty"`
	RootDir      string            `json:"root_dir"`
	ManifestPath string            `json:"manifest_path"`
	Manifest     manifest.Manifest `json:"-"`
	Order        int               `json:"order"`
}

func Resolve(storeRoot, parent string, buckets []string) ([]Layer, error) {
	parent = strings.TrimSpace(parent)
	if err := validateName("parent profile", parent); err != nil {
		return nil, fmt.Errorf("resolve profile: %w", err)
	}
	var layers []Layer
	commonRoot := filepath.Join(storeRoot, "profiles", "common")
	common, err := loadLayer("common", LayerCommon, "", "", commonRoot, len(layers))
	if err != nil {
		return nil, err
	}
	layers = append(layers, common)

	coreRoot := filepath.Join(storeRoot, "profiles", parent, "core")
	if _, err := os.Stat(filepath.Join(coreRoot, "manifest.yaml")); err != nil {
		return nil, fmt.Errorf("unknown parent profile %q: %w", parent, err)
	}
	core, err := loadLayer(parent+"-core", LayerCore, parent, "", coreRoot, len(layers))
	if err != nil {
		return nil, err
	}
	layers = append(layers, core)

	seen := map[string]bool{}
	for _, bucket := range buckets {
		bucket = strings.TrimSpace(bucket)
		if bucket == "" || seen[bucket] {
			continue
		}
		if err := validateName("bucket", bucket); err != nil {
			return nil, fmt.Errorf("resolve profile: %w", err)
		}
		seen[bucket] = true
		bucketRoot := filepath.Join(storeRoot, "profiles", parent, "buckets", bucket)
		if _, err := os.Stat(filepath.Join(bucketRoot, "manifest.yaml")); err != nil {
			return nil, fmt.Errorf("unknown bucket %q for parent profile %q: %w", bucket, parent, err)
		}
		layer, err := loadLayer(bucket, LayerBucket, parent, bucket, bucketRoot, len(layers))
		if err != nil {
			return nil, err
		}
		layers = append(layers, layer)
	}
	return layers, nil
}

func DiscoverParents(storeRoot string) ([]string, error) {
	profilesDir := filepath.Join(storeRoot, "profiles")
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return nil, fmt.Errorf("discover parent profiles: %w", err)
	}
	var parents []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "common" {
			continue
		}
		if err := validateName("parent profile", entry.Name()); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(profilesDir, entry.Name(), "core", "manifest.yaml")); err == nil {
			parents = append(parents, entry.Name())
		}
	}
	return parents, nil
}

func DiscoverBuckets(storeRoot, parent string) ([]string, error) {
	parent = strings.TrimSpace(parent)
	if err := validateName("parent profile", parent); err != nil {
		return nil, fmt.Errorf("discover buckets: %w", err)
	}
	bucketsDir := filepath.Join(storeRoot, "profiles", parent, "buckets")
	entries, err := os.ReadDir(bucketsDir)
	if err != nil {
		return nil, fmt.Errorf("discover buckets for %s: %w", parent, err)
	}
	var buckets []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := validateName("bucket", entry.Name()); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(bucketsDir, entry.Name(), "manifest.yaml")); err == nil {
			buckets = append(buckets, entry.Name())
		}
	}
	return buckets, nil
}

func LoadAllManifests(storeRoot string) ([]Layer, error) {
	var layers []Layer
	commonRoot := filepath.Join(storeRoot, "profiles", "common")
	common, err := loadLayer("common", LayerCommon, "", "", commonRoot, len(layers))
	if err != nil {
		return nil, err
	}
	layers = append(layers, common)
	parents, err := DiscoverParents(storeRoot)
	if err != nil {
		return nil, err
	}
	for _, parent := range parents {
		coreRoot := filepath.Join(storeRoot, "profiles", parent, "core")
		core, err := loadLayer(parent+"-core", LayerCore, parent, "", coreRoot, len(layers))
		if err != nil {
			return nil, err
		}
		layers = append(layers, core)
		buckets, err := DiscoverBuckets(storeRoot, parent)
		if err != nil {
			return nil, err
		}
		for _, bucket := range buckets {
			bucketRoot := filepath.Join(storeRoot, "profiles", parent, "buckets", bucket)
			layer, err := loadLayer(bucket, LayerBucket, parent, bucket, bucketRoot, len(layers))
			if err != nil {
				return nil, err
			}
			layers = append(layers, layer)
		}
	}
	return layers, nil
}

func loadLayer(name string, kind LayerKind, parent, bucket, root string, order int) (Layer, error) {
	manifestPath := filepath.Join(root, "manifest.yaml")
	parsed, err := manifest.ParseFile(manifestPath)
	if err != nil {
		return Layer{}, err
	}
	return Layer{Name: name, Kind: kind, Parent: parent, Bucket: bucket, RootDir: root, ManifestPath: manifestPath, Manifest: parsed, Order: order}, nil
}

func validateName(kind, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%s is required", kind)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("%s %q is not allowed", kind, name)
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) || len(name) >= 2 && name[1] == ':' {
		return fmt.Errorf("%s %q must be a simple name, not a path", kind, name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%s %q must not contain path separators", kind, name)
	}
	if filepath.Clean(name) != name {
		return fmt.Errorf("%s %q must be a clean path component", kind, name)
	}
	return nil
}
