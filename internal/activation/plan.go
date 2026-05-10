package activation

import "time"

type OperationType string

const (
	OperationSymlink OperationType = "symlink"
	OperationCopy    OperationType = "copy"
	OperationMerge   OperationType = "merge"
	OperationRender  OperationType = "render"
	OperationMirror  OperationType = "mirror"
)

type SafetyClass string

const (
	SafetyUnknown             SafetyClass = "unknown"
	SafetyMissing             SafetyClass = "missing"
	SafetyManagedSymlink      SafetyClass = "loki_managed_symlink"
	SafetyManagedFileHash     SafetyClass = "loki_managed_file_hash"
	SafetyManagedGenerated    SafetyClass = "loki_managed_generated"
	SafetyUnmanagedFile       SafetyClass = "unmanaged_file"
	SafetyUnmanagedDirectory  SafetyClass = "unmanaged_directory"
	SafetyBrokenSymlink       SafetyClass = "broken_symlink"
	SafetyManagedHashMismatch SafetyClass = "loki_managed_hash_mismatch"
)

type SafetyStatus struct {
	Class        SafetyClass `json:"class"`
	Safe         bool        `json:"safe"`
	Message      string      `json:"message,omitempty"`
	ExistingHash string      `json:"existing_hash,omitempty"`
	Managed      bool        `json:"managed"`
}

type Source struct {
	Path      string `json:"path"`
	LayerName string `json:"layer_name"`
	LayerKind string `json:"layer_kind"`
	FileID    string `json:"file_id"`
	Order     int    `json:"order"`
}

type Operation struct {
	ID           string        `json:"id"`
	Type         OperationType `json:"type"`
	Format       string        `json:"format,omitempty"`
	TargetPath   string        `json:"target_path"`
	SourcePath   string        `json:"source_path,omitempty"`
	Sources      []Source      `json:"sources,omitempty"`
	LayerName    string        `json:"layer_name,omitempty"`
	LayerKind    string        `json:"layer_kind,omitempty"`
	Capture      bool          `json:"capture"`
	Secrets      []string      `json:"secrets,omitempty"`
	ExpectedHash string        `json:"expected_hash,omitempty"`
	Safety       SafetyStatus  `json:"safety"`
}

type Plan struct {
	StorePath   string      `json:"store_path"`
	Profile     string      `json:"profile"`
	Buckets     []string    `json:"buckets"`
	GeneratedAt string      `json:"generated_at"`
	Operations  []Operation `json:"operations"`
	Warnings    []string    `json:"warnings,omitempty"`
}

func NewPlan(storePath, profile string, buckets []string, now time.Time) Plan {
	return Plan{
		StorePath:   storePath,
		Profile:     profile,
		Buckets:     cloneStrings(buckets),
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Operations:  []Operation{},
		Warnings:    []string{},
	}
}

func (p Plan) OperationCount() int {
	return len(p.Operations)
}

func cloneStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
