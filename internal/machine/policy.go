package machine

import (
	"fmt"
	"strings"
)

type PolicyError struct {
	MachineID   string
	Kind        string
	Requested   string
	Allowed     []string
	Remediation string
}

func (e PolicyError) Error() string {
	return fmt.Sprintf("machine policy blocks %s %q for machine %s. %s", e.Kind, e.Requested, e.MachineID, e.Remediation)
}

func ValidatePolicy(record Record, parentProfile string, buckets []string) error {
	if !contains(record.AllowedParentProfiles, parentProfile) {
		return PolicyError{
			MachineID:   record.MachineID,
			Kind:        "parent_profile",
			Requested:   parentProfile,
			Allowed:     cloneStrings(record.AllowedParentProfiles),
			Remediation: fmt.Sprintf("Add %q to allowed_parent_profiles in registry/machines.json or choose one of: %s.", parentProfile, strings.Join(record.AllowedParentProfiles, ", ")),
		}
	}
	seen := map[string]bool{}
	for _, bucket := range buckets {
		bucket = strings.TrimSpace(bucket)
		if bucket == "" || seen[bucket] {
			continue
		}
		seen[bucket] = true
		if !contains(record.AllowedBuckets, bucket) {
			return PolicyError{
				MachineID:   record.MachineID,
				Kind:        "bucket",
				Requested:   bucket,
				Allowed:     cloneStrings(record.AllowedBuckets),
				Remediation: fmt.Sprintf("Add %q to allowed_buckets in registry/machines.json or choose one of: %s.", bucket, strings.Join(record.AllowedBuckets, ", ")),
			}
		}
	}
	return nil
}

func contains(values []string, requested string) bool {
	requested = strings.TrimSpace(requested)
	for _, value := range values {
		if strings.TrimSpace(value) == requested {
			return true
		}
	}
	return false
}
