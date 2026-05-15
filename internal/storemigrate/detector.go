package storemigrate

import "io/fs"

// CloudPlaceholderDetector detects files that are visible in the filesystem but still cloud-only.
type CloudPlaceholderDetector interface {
	IsCloudOnly(path string, info fs.FileInfo) bool
}

// DefaultCloudPlaceholderDetector returns the platform-specific cloud placeholder detector.
func DefaultCloudPlaceholderDetector() CloudPlaceholderDetector {
	return platformCloudPlaceholderDetector{}
}
