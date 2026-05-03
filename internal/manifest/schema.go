package manifest

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

const Version = 1

const (
	ModeSymlink = "symlink"
	ModeCopy    = "copy"
	ModeMerge   = "merge"
	ModeRender  = "render"
)

const (
	FormatJSON = "json"
	FormatYAML = "yaml"
	FormatTOML = "toml"
	FormatText = "text"
)

type Manifest struct {
	Version    int                    `yaml:"version" json:"version"`
	Name       string                 `yaml:"name" json:"name"`
	Files      []FileEntry            `yaml:"files" json:"files"`
	Skills     []SkillEntry           `yaml:"skills" json:"skills"`
	Ignore     []string               `yaml:"ignore" json:"ignore"`
	MergeRules map[string]string      `yaml:"merge_rules" json:"merge_rules"`
	Targets    map[string]TargetValue `yaml:"targets" json:"targets"`
}

type FileEntry struct {
	ID      string   `yaml:"id" json:"id"`
	Source  string   `yaml:"source" json:"source"`
	Target  string   `yaml:"target" json:"target"`
	Mode    string   `yaml:"mode" json:"mode"`
	Format  string   `yaml:"format" json:"format"`
	Capture bool     `yaml:"capture" json:"capture"`
	Secrets []string `yaml:"secrets" json:"secrets"`
}

type SkillEntry struct {
	Source  string   `yaml:"source" json:"source"`
	Targets []string `yaml:"targets" json:"targets"`
}

type TargetValue struct {
	Default string `yaml:"default" json:"default"`
	Windows string `yaml:"windows" json:"windows"`
	Darwin  string `yaml:"darwin" json:"darwin"`
}

func (v *TargetValue) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		v.Default = s
		return nil
	}
	type raw TargetValue
	var out raw
	if err := value.Decode(&out); err != nil {
		return err
	}
	*v = TargetValue(out)
	return nil
}

func (v TargetValue) ForOS(goos string) (string, bool) {
	switch goos {
	case "windows":
		if v.Windows != "" {
			return v.Windows, true
		}
	case "darwin":
		if v.Darwin != "" {
			return v.Darwin, true
		}
	}
	if v.Default != "" {
		return v.Default, true
	}
	return "", false
}

type Severity string

const (
	SeverityBlocking Severity = "blocking"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

type Problem struct {
	Severity    Severity `json:"severity"`
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Path        string   `json:"path,omitempty"`
	Layer       string   `json:"layer,omitempty"`
	Target      string   `json:"target,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
}

func blocking(code, message, path string) Problem {
	return Problem{Severity: SeverityBlocking, Code: code, Message: message, Path: path}
}

func warning(code, message, path string) Problem {
	return Problem{Severity: SeverityWarning, Code: code, Message: message, Path: path}
}

func (p Problem) Error() string {
	if p.Path == "" {
		return fmt.Sprintf("%s: %s", p.Code, p.Message)
	}
	return fmt.Sprintf("%s: %s: %s", p.Code, p.Path, p.Message)
}
