package machine

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatePolicyAllowed(t *testing.T) {
	record := Record{MachineID: "machine-1", AllowedParentProfiles: []string{"work"}, AllowedBuckets: []string{"content-dev", "azure"}}
	if err := ValidatePolicy(record, "work", []string{"content-dev", "azure"}); err != nil {
		t.Fatalf("ValidatePolicy() error = %v", err)
	}
}

func TestValidatePolicyEmptyBucketsAllowed(t *testing.T) {
	record := Record{MachineID: "machine-1", AllowedParentProfiles: []string{"work"}, AllowedBuckets: []string{"content-dev"}}
	if err := ValidatePolicy(record, "work", nil); err != nil {
		t.Fatalf("ValidatePolicy() error = %v", err)
	}
}

func TestValidatePolicyDisallowedProfile(t *testing.T) {
	record := Record{MachineID: "machine-1", AllowedParentProfiles: []string{"work"}, AllowedBuckets: []string{"content-dev"}}
	err := ValidatePolicy(record, "dev", nil)
	var policyErr PolicyError
	if !errors.As(err, &policyErr) || policyErr.Kind != "parent_profile" || !strings.Contains(err.Error(), "allowed_parent_profiles") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePolicyDisallowedBucket(t *testing.T) {
	record := Record{MachineID: "machine-1", AllowedParentProfiles: []string{"work"}, AllowedBuckets: []string{"content-dev"}}
	err := ValidatePolicy(record, "work", []string{"writer"})
	var policyErr PolicyError
	if !errors.As(err, &policyErr) || policyErr.Kind != "bucket" || !strings.Contains(err.Error(), "allowed_buckets") {
		t.Fatalf("unexpected error: %v", err)
	}
}
