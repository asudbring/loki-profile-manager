package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type ValidationInput struct {
	LayerName string
	LayerRoot string
	Manifest  Manifest
	Expander  Expander
}

type FileOperation struct {
	LayerName  string
	LayerRoot  string
	Entry      FileEntry
	SourcePath string
	TargetPath string
}

type ValidationResult struct {
	Problems   []Problem
	Operations []FileOperation
}

func ValidateLayer(input ValidationInput) ValidationResult {
	result := ValidationResult{Problems: []Problem{}, Operations: []FileOperation{}}
	if strings.TrimSpace(input.Manifest.Name) == "" {
		result.Problems = append(result.Problems, blocking("manifest.name_missing", "manifest name is required", input.LayerRoot))
	}
	for _, pattern := range input.Manifest.Ignore {
		if _, err := doublestar.Match(pattern, "validation-probe"); err != nil {
			result.Problems = append(result.Problems, blocking("manifest.ignore_invalid", fmt.Sprintf("ignore pattern %q is invalid: %v", pattern, err), input.LayerRoot))
		}
	}
	for _, entry := range input.Manifest.Files {
		problems, op, ok := validateFile(input, entry)
		result.Problems = append(result.Problems, problems...)
		if ok {
			result.Operations = append(result.Operations, op)
		}
	}
	for _, entry := range input.Manifest.Skills {
		result.Problems = append(result.Problems, validateSkillSource(input, entry)...)
	}
	return result
}

func validateFile(input ValidationInput, entry FileEntry) ([]Problem, FileOperation, bool) {
	problems := []Problem{}
	if strings.TrimSpace(entry.ID) == "" {
		problems = append(problems, blocking("manifest.file_id_missing", "file entry id is required", input.LayerRoot))
	}
	if strings.TrimSpace(entry.Source) == "" {
		problems = append(problems, blocking("manifest.source_missing", "file entry source is required", input.LayerRoot))
	}
	if strings.TrimSpace(entry.Target) == "" {
		problems = append(problems, blocking("manifest.target_missing", "file entry target is required", input.LayerRoot))
	}
	if !knownMode(entry.Mode) {
		problems = append(problems, blocking("manifest.mode_invalid", fmt.Sprintf("mode %q is not supported", entry.Mode), input.LayerRoot))
	}
	if !knownFormat(entry.Format) {
		problems = append(problems, blocking("manifest.format_invalid", fmt.Sprintf("format %q is not supported", entry.Format), input.LayerRoot))
	}
	if entry.Mode == ModeMerge && !structuredFormat(entry.Format) {
		problems = append(problems, blocking("manifest.merge_format_required", "merge mode requires json, yaml, or toml format", input.LayerRoot))
	}
	if entry.Capture && entry.Mode != ModeCopy && entry.Mode != ModeMerge {
		problems = append(problems, warning("manifest.capture_ignored", "capture is only supported for copy or merge modes", input.LayerRoot))
	}

	sourcePath := ""
	if entry.Source != "" {
		resolved, err := ResolveSource(input.LayerRoot, entry.Source)
		if err != nil {
			problems = append(problems, blocking("manifest.source_invalid", err.Error(), input.LayerRoot))
		} else {
			sourcePath = resolved
			if _, err := os.Stat(sourcePath); err != nil {
				problems = append(problems, blocking("manifest.source_missing", fmt.Sprintf("source %s does not exist", sourcePath), sourcePath))
			}
		}
	}
	targetPath := ""
	if entry.Target != "" {
		expanded, err := input.Expander.ExpandTarget(entry.Target)
		if err != nil {
			problems = append(problems, blocking("manifest.target_invalid", err.Error(), input.LayerRoot))
		} else {
			targetPath = expanded
		}
	}

	blockingCount := 0
	for _, problem := range problems {
		if problem.Severity == SeverityBlocking {
			blockingCount++
		}
	}
	return problems, FileOperation{LayerName: input.LayerName, LayerRoot: input.LayerRoot, Entry: entry, SourcePath: sourcePath, TargetPath: targetPath}, blockingCount == 0
}

func validateSkillSource(input ValidationInput, entry SkillEntry) []Problem {
	problems := []Problem{}
	if strings.TrimSpace(entry.Source) == "" {
		return append(problems, blocking("manifest.skill_source_missing", "skill source is required", input.LayerRoot))
	}
	resolved, err := ResolveSource(input.LayerRoot, entry.Source)
	if err != nil {
		return append(problems, blocking("manifest.skill_source_invalid", err.Error(), input.LayerRoot))
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return append(problems, blocking("manifest.skill_source_missing", fmt.Sprintf("skill source %s does not exist", resolved), resolved))
	}
	if !info.IsDir() {
		return append(problems, blocking("manifest.skill_source_invalid", fmt.Sprintf("skill source %s is not a directory", resolved), resolved))
	}
	return problems
}

func SkillSourceDirs(layerRoot string, entry SkillEntry) ([]string, error) {
	resolved, err := ResolveSource(layerRoot, entry.Source)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(resolved, "SKILL.md")); err == nil {
		return []string{resolved}, nil
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(resolved, entry.Name())
		if _, err := os.Stat(filepath.Join(candidate, "SKILL.md")); err == nil {
			dirs = append(dirs, candidate)
		}
	}
	return dirs, nil
}

func knownMode(mode string) bool {
	switch mode {
	case ModeSymlink, ModeCopy, ModeMerge, ModeRender:
		return true
	default:
		return false
	}
}

func knownFormat(format string) bool {
	switch format {
	case "", FormatJSON, FormatYAML, FormatTOML, FormatText:
		return true
	default:
		return false
	}
}

func structuredFormat(format string) bool {
	switch format {
	case FormatJSON, FormatYAML, FormatTOML:
		return true
	default:
		return false
	}
}
