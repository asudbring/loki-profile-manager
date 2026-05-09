package verify

import "github.com/asudbring/loki-profile-manager/internal/manifest"

type Severity string

const (
	SeverityBlocking Severity = "blocking"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

type Issue struct {
	Severity    Severity `json:"severity"`
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Path        string   `json:"path,omitempty"`
	Layer       string   `json:"layer,omitempty"`
	Target      string   `json:"target,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
}

type LayerSummary struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	ManifestPath string `json:"manifest_path"`
}

type Summary struct {
	Blocking int `json:"blocking"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
}

type Report struct {
	Valid     bool           `json:"valid"`
	StorePath string         `json:"store_path"`
	Profile   string         `json:"profile,omitempty"`
	Buckets   []string       `json:"buckets,omitempty"`
	Layers    []LayerSummary `json:"layers,omitempty"`
	Issues    []Issue        `json:"issues"`
	Summary   Summary        `json:"summary"`
}

func (r *Report) Add(issue Issue) {
	r.Issues = append(r.Issues, issue)
	switch issue.Severity {
	case SeverityBlocking:
		r.Summary.Blocking++
	case SeverityWarning:
		r.Summary.Warnings++
	case SeverityInfo:
		r.Summary.Info++
	}
	r.Valid = r.Summary.Blocking == 0
}

func FromManifestProblem(problem manifest.Problem) Issue {
	return Issue{Severity: Severity(problem.Severity), Code: problem.Code, Message: problem.Message, Path: problem.Path, Layer: problem.Layer, Target: problem.Target, Remediation: problem.Remediation}
}
