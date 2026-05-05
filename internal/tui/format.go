package tui

import (
	"fmt"
	"strings"

	"github.com/allensu/loki-profile-manager/internal/app"
)

func frame(lines ...string) string {
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func labelValue(label, value string) string {
	return fmt.Sprintf("%s %s", labelStyle.Render(label+":"), value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func formatMachine(status app.StatusResult) string {
	if status.MachineID == "" {
		return "not registered"
	}
	if status.MachineRegistered {
		if status.MachineDisplayName != "" {
			return fmt.Sprintf("registered (%s)", status.MachineDisplayName)
		}
		return "registered"
	}
	return "unregistered"
}

func profileCount(catalog app.ProfileCatalogResult) int {
	return len(catalog.Profiles)
}

func bucketCount(catalog app.ProfileCatalogResult) int {
	count := 0
	for _, profile := range catalog.Profiles {
		count += len(profile.Buckets)
	}
	return count
}
