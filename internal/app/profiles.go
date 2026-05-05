package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/allensu/loki-profile-manager/internal/profile"
	"github.com/allensu/loki-profile-manager/internal/store"
)

type ProfileCatalogRequest struct {
	StorePath string
}

type ProfileCatalogResult struct {
	StorePath string           `json:"store_path"`
	Profiles  []ProfileSummary `json:"profiles"`
}

type ProfileSummary struct {
	Name    string          `json:"name"`
	Buckets []BucketSummary `json:"buckets"`
}

type BucketSummary struct {
	Name string `json:"name"`
}

func (s *Service) ProfileCatalog(ctx context.Context, req ProfileCatalogRequest) (ProfileCatalogResult, error) {
	if s == nil {
		return ProfileCatalogResult{}, fmt.Errorf("profile catalog: service is nil")
	}
	storePath, err := s.effectiveStorePath(ctx, req.StorePath)
	if err != nil {
		return ProfileCatalogResult{}, err
	}
	if validation := store.ValidateLayout(storePath); !validation.Valid {
		return ProfileCatalogResult{}, fmt.Errorf("profile catalog: invalid store layout: missing %v", validation.Missing)
	}

	parents, err := profile.DiscoverParents(storePath)
	if err != nil {
		return ProfileCatalogResult{}, err
	}
	sort.Strings(parents)

	result := ProfileCatalogResult{StorePath: storePath, Profiles: []ProfileSummary{}}
	for _, parent := range parents {
		buckets, err := profile.DiscoverBuckets(storePath, parent)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return ProfileCatalogResult{}, err
		}
		sort.Strings(buckets)
		summary := ProfileSummary{Name: parent, Buckets: []BucketSummary{}}
		for _, bucket := range buckets {
			summary.Buckets = append(summary.Buckets, BucketSummary{Name: bucket})
		}
		result.Profiles = append(result.Profiles, summary)
	}
	return result, nil
}
