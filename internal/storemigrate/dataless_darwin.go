//go:build darwin

package storemigrate

import (
	"io/fs"
	"syscall"
)

func fileInfoDatalessForPlatform(info fs.FileInfo) bool {
	if info == nil || info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && datalessFlagSet(stat.Flags)
}
