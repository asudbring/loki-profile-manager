package app

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	importSkillZipMaxFiles             = 1000
	importSkillZipMaxUncompressedBytes = 50 * 1024 * 1024
)

type preparedImportSkillSource struct {
	OriginalPath string
	SkillDir     string
	DefaultName  string
	Kind         string
	Cleanup      func()
}

func prepareImportSkillSource(source string) (preparedImportSkillSource, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return preparedImportSkillSource{}, fmt.Errorf("import-skill: source is required")
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		return preparedImportSkillSource{}, fmt.Errorf("import-skill: resolve source %s: %w", source, err)
	}
	abs = filepath.Clean(abs)
	info, err := os.Lstat(abs)
	if err != nil {
		return preparedImportSkillSource{}, fmt.Errorf("import-skill: stat source %s: %w", abs, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return preparedImportSkillSource{}, fmt.Errorf("import-skill: source %s is a symlink; symlinks are not supported in skill imports", abs)
	}
	if info.IsDir() {
		return preparedImportSkillSource{OriginalPath: abs, SkillDir: abs, DefaultName: filepath.Base(abs), Kind: "folder", Cleanup: func() {}}, nil
	}
	if info.Mode().IsRegular() && strings.EqualFold(filepath.Ext(abs), ".md") {
		return prepareImportSkillMarkdownSource(abs)
	}
	if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(abs), ".zip") {
		return preparedImportSkillSource{}, fmt.Errorf("import-skill: source %s is not a directory, .md file, or .zip archive", abs)
	}
	return prepareImportSkillZipSource(abs)
}

func prepareImportSkillZipSource(zipPath string) (preparedImportSkillSource, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return preparedImportSkillSource{}, fmt.Errorf("import-skill: open zip %s: %w", zipPath, err)
	}
	defer reader.Close()

	tmp, err := os.MkdirTemp("", "loki-import-skill-zip-*")
	if err != nil {
		return preparedImportSkillSource{}, fmt.Errorf("import-skill: create zip staging directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	ok := false
	defer func() {
		if !ok {
			cleanup()
		}
	}()

	if err := extractImportSkillZip(reader.File, tmp); err != nil {
		return preparedImportSkillSource{}, err
	}
	skillDir, defaultName, err := findImportSkillZipRoot(tmp, zipPath)
	if err != nil {
		return preparedImportSkillSource{}, err
	}
	ok = true
	return preparedImportSkillSource{OriginalPath: zipPath, SkillDir: skillDir, DefaultName: defaultName, Kind: "zip", Cleanup: cleanup}, nil
}

func extractImportSkillZip(files []*zip.File, destRoot string) error {
	var total uint64
	for i, file := range files {
		if i >= importSkillZipMaxFiles {
			return fmt.Errorf("import-skill: zip archive has too many entries (max %d)", importSkillZipMaxFiles)
		}
		if file.UncompressedSize64 > importSkillZipMaxUncompressedBytes-total {
			return fmt.Errorf("import-skill: zip archive is too large after extraction (max %d bytes)", importSkillZipMaxUncompressedBytes)
		}
		cleanName, isDir, err := cleanImportSkillZipEntryName(file.Name)
		if err != nil {
			return err
		}
		mode := file.FileInfo().Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("import-skill: zip entry %q is a symlink; symlinks are not supported in skill imports", file.Name)
		}
		if mode.Type() != 0 && !mode.IsRegular() && !isDir {
			return fmt.Errorf("import-skill: zip entry %q is not a regular file or directory", file.Name)
		}
		destPath, err := importSkillZipDestination(destRoot, cleanName)
		if err != nil {
			return err
		}
		if isDir {
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return fmt.Errorf("import-skill: create zip directory %s: %w", destPath, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("import-skill: create zip parent directory %s: %w", filepath.Dir(destPath), err)
		}
		if _, err := os.Lstat(destPath); err == nil {
			return fmt.Errorf("import-skill: duplicate zip entry %q", file.Name)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("import-skill: stat zip destination %s: %w", destPath, err)
		}
		copied, err := extractImportSkillZipFile(file, destPath, importSkillZipMaxUncompressedBytes-total)
		if err != nil {
			return err
		}
		total += copied
	}
	return nil
}

func cleanImportSkillZipEntryName(name string) (string, bool, error) {
	if name == "" || len(name) >= 2 && name[1] == ':' {
		return "", false, fmt.Errorf("import-skill: unsafe zip entry %q", name)
	}
	normalized := strings.ReplaceAll(name, `\\`, "/")
	if strings.HasPrefix(normalized, "/") {
		return "", false, fmt.Errorf("import-skill: unsafe zip entry %q", name)
	}
	isDir := strings.HasSuffix(normalized, "/")
	raw := strings.TrimSuffix(normalized, "/")
	if raw == "" {
		return "", false, fmt.Errorf("import-skill: unsafe zip entry %q", name)
	}
	clean := path.Clean(raw)
	if clean != raw || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", false, fmt.Errorf("import-skill: unsafe zip entry %q", name)
	}
	return clean, isDir, nil
}

func importSkillZipDestination(root, cleanName string) (string, error) {
	dest := filepath.Join(root, filepath.FromSlash(cleanName))
	rel, err := filepath.Rel(root, dest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("import-skill: zip entry %q escapes extraction root", cleanName)
	}
	return dest, nil
}

func extractImportSkillZipFile(file *zip.File, destPath string, remainingBytes uint64) (uint64, error) {
	reader, err := file.Open()
	if err != nil {
		return 0, fmt.Errorf("import-skill: open zip entry %q: %w", file.Name, err)
	}
	defer reader.Close()
	writer, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, fmt.Errorf("import-skill: create zip entry %s: %w", destPath, err)
	}
	if remainingBytes > uint64(1<<63-2) {
		_ = writer.Close()
		_ = os.Remove(destPath)
		return 0, fmt.Errorf("import-skill: zip archive is too large after extraction (max %d bytes)", importSkillZipMaxUncompressedBytes)
	}
	limited := &io.LimitedReader{R: reader, N: int64(remainingBytes) + 1}
	copied, copyErr := io.Copy(writer, limited)
	closeErr := writer.Close()
	if copied < 0 {
		_ = os.Remove(destPath)
		return 0, fmt.Errorf("import-skill: extract zip entry %q: negative byte count", file.Name)
	}
	copiedBytes := uint64(copied)
	if copiedBytes > remainingBytes {
		_ = os.Remove(destPath)
		return 0, fmt.Errorf("import-skill: zip archive is too large after extraction (max %d bytes)", importSkillZipMaxUncompressedBytes)
	}
	if copyErr != nil {
		_ = os.Remove(destPath)
		return 0, fmt.Errorf("import-skill: extract zip entry %q: %w", file.Name, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(destPath)
		return 0, fmt.Errorf("import-skill: close zip entry %s: %w", destPath, closeErr)
	}
	return copiedBytes, nil
}

func findImportSkillZipRoot(extractRoot, zipPath string) (string, string, error) {
	if info, err := os.Stat(filepath.Join(extractRoot, "SKILL.md")); err == nil && !info.IsDir() {
		return extractRoot, strings.TrimSuffix(filepath.Base(zipPath), filepath.Ext(zipPath)), nil
	}
	entries, err := os.ReadDir(extractRoot)
	if err != nil {
		return "", "", fmt.Errorf("import-skill: read zip root: %w", err)
	}
	var candidates []fs.DirEntry
	for _, entry := range entries {
		if entry.Name() == "__MACOSX" || entry.Name() == ".DS_Store" {
			continue
		}
		candidates = append(candidates, entry)
	}
	if len(candidates) != 1 || !candidates[0].IsDir() {
		return "", "", fmt.Errorf("import-skill: zip archive must contain SKILL.md at the archive root or exactly one top-level skill folder")
	}
	skillDir := filepath.Join(extractRoot, candidates[0].Name())
	if info, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil || info.IsDir() {
		return "", "", fmt.Errorf("import-skill: zip archive top-level folder %q does not contain SKILL.md", candidates[0].Name())
	}
	return skillDir, candidates[0].Name(), nil
}

func prepareImportSkillMarkdownSource(mdPath string) (preparedImportSkillSource, error) {
	content, err := os.ReadFile(mdPath)
	if err != nil {
		return preparedImportSkillSource{}, fmt.Errorf("import-skill: read markdown source %s: %w", mdPath, err)
	}

	baseName := strings.TrimSuffix(filepath.Base(mdPath), filepath.Ext(mdPath))
	defaultName := toSkillName(baseName)

	skillContent, err := ensureSkillFrontmatter(content, defaultName)
	if err != nil {
		return preparedImportSkillSource{}, fmt.Errorf("import-skill: convert markdown %s: %w", mdPath, err)
	}

	tmp, err := os.MkdirTemp("", "loki-import-skill-md-*")
	if err != nil {
		return preparedImportSkillSource{}, fmt.Errorf("import-skill: create markdown staging directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	ok := false
	defer func() {
		if !ok {
			cleanup()
		}
	}()

	if err := os.WriteFile(filepath.Join(tmp, "SKILL.md"), skillContent, 0o644); err != nil {
		return preparedImportSkillSource{}, fmt.Errorf("import-skill: write SKILL.md: %w", err)
	}

	ok = true
	return preparedImportSkillSource{OriginalPath: mdPath, SkillDir: tmp, DefaultName: defaultName, Kind: "markdown", Cleanup: cleanup}, nil
}

func ensureSkillFrontmatter(content []byte, defaultName string) ([]byte, error) {
	text := strings.TrimPrefix(strings.ReplaceAll(string(content), "\r\n", "\n"), "\ufeff")

	if strings.HasPrefix(text, "---\n") {
		end := strings.Index(text[4:], "\n---\n")
		if end >= 0 {
			frontBlock := text[4 : 4+end]
			body := text[4+end+5:]
			var fm struct {
				Name        string `yaml:"name"`
				Description string `yaml:"description"`
			}
			if err := yaml.Unmarshal([]byte(frontBlock), &fm); err == nil {
				if strings.TrimSpace(fm.Name) != "" && strings.TrimSpace(fm.Description) != "" {
					return content, nil
				}
				if strings.TrimSpace(fm.Name) == "" {
					fm.Name = defaultName
				}
				if strings.TrimSpace(fm.Description) == "" {
					fm.Description = deriveSkillDescription(body)
				}
				return buildSkillContent(fm.Name, fm.Description, body), nil
			}
		}
	}

	description := deriveSkillDescription(text)
	return buildSkillContent(defaultName, description, text), nil
}

func buildSkillContent(name, description, body string) []byte {
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString("name: " + name + "\n")
	buf.WriteString("description: " + yamlQuote(description) + "\n")
	buf.WriteString("---\n")
	if body != "" && !strings.HasPrefix(body, "\n") {
		buf.WriteString("\n")
	}
	buf.WriteString(body)
	if !strings.HasSuffix(buf.String(), "\n") {
		buf.WriteString("\n")
	}
	return buf.Bytes()
}

var headingRE = regexp.MustCompile(`(?m)^#+\s+(.+)$`)

func deriveSkillDescription(body string) string {
	if m := headingRE.FindStringSubmatch(body); len(m) > 1 {
		desc := strings.TrimSpace(m[1])
		if len(desc) > 200 {
			desc = desc[:200]
		}
		return desc
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > 200 {
			trimmed = trimmed[:200]
		}
		return trimmed
	}
	return "Imported skill"
}

func toSkillName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		if r == ' ' || r == '_' || r == '.' {
			return '-'
		}
		return -1
	}, name)
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	name = strings.Trim(name, "-")
	if name == "" {
		name = "imported-skill"
	}
	return name
}

func yamlQuote(s string) string {
	if strings.ContainsAny(s, ":\n\"'#{}[]|>&*!%@`") || strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") {
		escaped := strings.ReplaceAll(s, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return s
}
