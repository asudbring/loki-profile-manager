package migration

import "time"

type SourceKind string

const (
	SourceRepo  SourceKind = "repo"
	SourceLocal SourceKind = "local"
	SourceAdopt SourceKind = "adopt"
)

type CollisionStatus string

const (
	CollisionNone      CollisionStatus = "none"
	CollisionIdentical CollisionStatus = "identical"
	CollisionUpdate    CollisionStatus = "update"
	CollisionConflict  CollisionStatus = "conflict"
)

type Plan struct {
	StorePath   string     `json:"store_path"`
	SourceKind  SourceKind `json:"source_kind"`
	Profile     string     `json:"profile"`
	Bucket      string     `json:"bucket,omitempty"`
	LayerRoot   string     `json:"layer_root"`
	LayerKind   string     `json:"layer_kind"`
	LayerName   string     `json:"layer_name"`
	GeneratedAt string     `json:"generated_at"`
	Items       []Item     `json:"items"`
	Warnings    []string   `json:"warnings,omitempty"`
}

type Item struct {
	ID              string          `json:"id"`
	SourceKind      SourceKind      `json:"source_kind"`
	SourcePath      string          `json:"source_path"`
	StorePath       string          `json:"store_path"`
	ManifestPath    string          `json:"manifest_path"`
	ManifestSource  string          `json:"manifest_source"`
	Target          string          `json:"target"`
	TargetPath      string          `json:"target_path"`
	Profile         string          `json:"profile"`
	Bucket          string          `json:"bucket,omitempty"`
	LayerKind       string          `json:"layer_kind"`
	LayerName       string          `json:"layer_name"`
	Mode            string          `json:"mode"`
	Format          string          `json:"format,omitempty"`
	Secrets         []string        `json:"secrets,omitempty"`
	IsSkill         bool            `json:"is_skill,omitempty"`
	SkillSource     string          `json:"skill_source,omitempty"`
	Collision       CollisionStatus `json:"collision"`
	ExistingHash    string          `json:"existing_hash,omitempty"`
	ImportedHash    string          `json:"imported_hash,omitempty"`
	WillAdoptRecord bool            `json:"will_adopt_record"`
	Warning         string          `json:"warning,omitempty"`
}

type BuildRequest struct {
	StorePath string
	Profile   string
	Bucket    string
	Now       func() time.Time
}

func newPlan(kind SourceKind, layer layerInfo, now func() time.Time) Plan {
	if now == nil {
		now = time.Now
	}
	return Plan{
		StorePath:   layer.StorePath,
		SourceKind:  kind,
		Profile:     layer.Profile,
		Bucket:      layer.Bucket,
		LayerRoot:   layer.Root,
		LayerKind:   layer.Kind,
		LayerName:   layer.Name,
		GeneratedAt: now().UTC().Format(time.RFC3339),
		Items:       []Item{},
		Warnings:    []string{},
	}
}
