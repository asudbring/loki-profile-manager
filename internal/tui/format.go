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

func formatBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func formatSectionStatus(err error, value string) string {
	if err != nil {
		return "error: " + err.Error()
	}
	return value
}

func formatList(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func formatDoctorSummary(doctor app.DoctorResult) string {
	if doctor.Summary.Blocking == 0 && doctor.Summary.Warnings == 0 {
		return "healthy"
	}
	return fmt.Sprintf("%d blocking, %d warning, %d info", doctor.Summary.Blocking, doctor.Summary.Warnings, doctor.Summary.Info)
}

func formatSecretsReady(status app.SecretsStatusResult) string {
	if status.Provider == "" {
		return "unknown"
	}
	if status.Ready {
		return string(status.Provider) + " ready"
	}
	return string(status.Provider) + " not ready"
}

func formatMachineFromStatus(status app.StatusResult, machine app.MachineStatusResult) string {
	if machine.Message != "" || machine.MachineID != "" {
		if machine.Registered {
			if machine.Record != nil && machine.Record.DisplayName != "" {
				return fmt.Sprintf("registered (%s)", machine.Record.DisplayName)
			}
			return "registered"
		}
		if machine.MachineID != "" {
			return "unregistered"
		}
		return "not registered"
	}
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
