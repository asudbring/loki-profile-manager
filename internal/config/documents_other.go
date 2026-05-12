//go:build !windows

package config

func defaultDocumentsDir(home string) string {
	if home == "" {
		return ""
	}
	return joinForOS("", home, "Documents")
}
