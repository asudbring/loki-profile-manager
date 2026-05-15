//go:build darwin

package storemigrate

import "io/fs"

type platformCloudPlaceholderDetector struct{}

func (platformCloudPlaceholderDetector) IsCloudOnly(_ string, info fs.FileInfo) bool {
	return fileInfoDataless(info)
}
