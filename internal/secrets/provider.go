package secrets

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type ProviderID string

const ProviderInfisical ProviderID = "infisical"

const (
	SeverityInfo    = "info"
	SeverityWarning = "warning"
)

type Provider interface {
	GetSecrets(ctx context.Context, names []string) (map[string]string, error)
}

type StatusChecker interface {
	CheckStatus(ctx context.Context) Status
}

type LoginRunner interface {
	Login(ctx context.Context, req LoginRequest) error
}

type LoginRequest struct {
	Domain string
}

type Status struct {
	Provider      ProviderID `json:"provider"`
	CLIInstalled  bool       `json:"cli_installed"`
	Authenticated bool       `json:"authenticated"`
	Ready         bool       `json:"ready"`
	Checks        []Check    `json:"checks"`
}

type Check struct {
	Severity    string `json:"severity"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

var secretNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func NormalizeNames(names []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		if !secretNameRE.MatchString(name) {
			return nil, fmt.Errorf("invalid secret name %q", name)
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}
