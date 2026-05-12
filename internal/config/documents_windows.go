//go:build windows

package config

import "golang.org/x/sys/windows"

func defaultDocumentsDir(home string) string {
	path, err := windows.KnownFolderPath(windows.FOLDERID_Documents, windows.KF_FLAG_DEFAULT)
	if err == nil && path != "" {
		return path
	}
	if home == "" {
		return ""
	}
	return joinForOS("windows", home, "Documents")
}
