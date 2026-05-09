package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/app"
)

type switchFingerprint struct {
	Profile    string                 `json:"profile"`
	Buckets    []string               `json:"buckets"`
	Operations []operationFingerprint `json:"operations"`
	Warnings   []string               `json:"warnings,omitempty"`
}

type operationFingerprint struct {
	ID           string                   `json:"id"`
	Type         activation.OperationType `json:"type"`
	Format       string                   `json:"format,omitempty"`
	TargetPath   string                   `json:"target_path"`
	SourcePath   string                   `json:"source_path,omitempty"`
	Sources      []sourceFingerprint      `json:"sources,omitempty"`
	LayerName    string                   `json:"layer_name,omitempty"`
	LayerKind    string                   `json:"layer_kind,omitempty"`
	Capture      bool                     `json:"capture"`
	Secrets      []string                 `json:"secrets,omitempty"`
	ExpectedHash string                   `json:"expected_hash,omitempty"`
	Safety       activation.SafetyStatus  `json:"safety"`
}

type sourceFingerprint struct {
	Path      string `json:"path"`
	LayerName string `json:"layer_name"`
	LayerKind string `json:"layer_kind"`
	FileID    string `json:"file_id"`
	Order     int    `json:"order"`
}

func cloneStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func fingerprintSwitchResult(result app.SwitchResult) string {
	fp := switchFingerprint{
		Profile:  result.Plan.Profile,
		Buckets:  cloneStrings(result.Plan.Buckets),
		Warnings: cloneStrings(result.Warnings),
	}
	for _, op := range result.Plan.Operations {
		out := operationFingerprint{
			ID:           op.ID,
			Type:         op.Type,
			Format:       op.Format,
			TargetPath:   op.TargetPath,
			SourcePath:   op.SourcePath,
			LayerName:    op.LayerName,
			LayerKind:    op.LayerKind,
			Capture:      op.Capture,
			Secrets:      cloneStrings(op.Secrets),
			ExpectedHash: op.ExpectedHash,
			Safety:       op.Safety,
		}
		for _, source := range op.Sources {
			out.Sources = append(out.Sources, sourceFingerprint{
				Path:      source.Path,
				LayerName: source.LayerName,
				LayerKind: source.LayerKind,
				FileID:    source.FileID,
				Order:     source.Order,
			})
		}
		fp.Operations = append(fp.Operations, out)
	}
	content, _ := json.Marshal(fp)
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
