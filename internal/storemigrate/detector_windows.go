//go:build windows

package storemigrate

import (
	"io/fs"
	"syscall"
)

const (
	windowsFileAttributeOffline            = 0x00001000
	windowsFileAttributeRecallOnOpen       = 0x00040000
	windowsFileAttributeRecallOnDataAccess = 0x00400000
)

type platformCloudPlaceholderDetector struct{}

func (platformCloudPlaceholderDetector) IsCloudOnly(_ string, info fs.FileInfo) bool {
	if info == nil || info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return false
	}
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok || data == nil {
		return false
	}
	attrs := data.FileAttributes
	return attrs&windowsFileAttributeOffline != 0 || attrs&windowsFileAttributeRecallOnOpen != 0 || attrs&windowsFileAttributeRecallOnDataAccess != 0
}
