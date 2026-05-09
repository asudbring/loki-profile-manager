package doctor

import (
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/store"
)

type Severity string

const (
	SeverityBlocking Severity = "blocking"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

type RuntimeInfo struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

type Summary struct {
	Blocking int `json:"blocking"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
}

type Check struct {
	Severity    Severity          `json:"severity"`
	Code        string            `json:"code"`
	Category    string            `json:"category"`
	Message     string            `json:"message"`
	Path        string            `json:"path,omitempty"`
	Remediation string            `json:"remediation,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
}

type Report struct {
	Healthy            bool                      `json:"healthy"`
	Version            string                    `json:"version"`
	Runtime            RuntimeInfo               `json:"runtime"`
	StorePath          string                    `json:"store_path,omitempty"`
	StoreOverride      string                    `json:"store_override,omitempty"`
	LocalPaths         config.LocalPaths         `json:"local_paths"`
	ProviderCandidates []store.ProviderCandidate `json:"provider_candidates,omitempty"`
	Summary            Summary                   `json:"summary"`
	Checks             []Check                   `json:"checks"`
}

func (r *Report) add(check Check) {
	if check.Severity == "" {
		check.Severity = SeverityInfo
	}
	r.Checks = append(r.Checks, check)
	switch check.Severity {
	case SeverityBlocking:
		r.Summary.Blocking++
	case SeverityWarning:
		r.Summary.Warnings++
	default:
		r.Summary.Info++
	}
	r.Healthy = r.Summary.Blocking == 0
}
