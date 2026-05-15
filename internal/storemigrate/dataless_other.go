//go:build !darwin

package storemigrate

import "io/fs"

func fileInfoDatalessForPlatform(info fs.FileInfo) bool {
	return false
}
