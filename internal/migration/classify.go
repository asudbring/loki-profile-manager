package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/allensu/loki-profile-manager/internal/activation"
	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/infisical"
	"github.com/allensu/loki-profile-manager/internal/manifest"
)

type layerInfo struct {
	StorePath    string
	Profile      string
	Bucket       string
	Root         string
	ManifestPath string
	Kind         string
	Name         string
}

type candidate struct {
	SourcePath string
	SourceRel  string
	Target     string
	TargetPath string
	Mode       string
	Format     string
	Secrets    []string
	IsSkill    bool
	SkillRoot  string
	StoreRel   string
}

var secretLikeRE = regexp.MustCompile(`(?i)(^|/)(\.env(\..*)?|.*\.(pem|key|p12|pfx|kdbx)|\.ssh/(id_rsa|id_ed25519|id_ecdsa|id_dsa))$`)

func resolveLayer(storePath, profileName, bucket string) (layerInfo, error) {
	storePath = strings.TrimSpace(storePath)
	if storePath == "" {
		return layerInfo{}, fmt.Errorf("migration: store path is required")
	}
	profileName = strings.TrimSpace(profileName)
	if err := validateSimpleName("profile", profileName); err != nil {
		return layerInfo{}, err
	}
	bucket = strings.TrimSpace(bucket)
	info := layerInfo{StorePath: filepath.Clean(storePath), Profile: profileName, Bucket: bucket}
	if bucket == "" {
		info.Kind = "core"
		info.Name = profileName + "-core"
		info.Root = filepath.Join(info.StorePath, "profiles", profileName, "core")
	} else {
		if err := validateSimpleName("bucket", bucket); err != nil {
			return layerInfo{}, err
		}
		info.Kind = "bucket"
		info.Name = bucket
		info.Root = filepath.Join(info.StorePath, "profiles", profileName, "buckets", bucket)
	}
	info.ManifestPath = filepath.Join(info.Root, "manifest.yaml")
	return info, nil
}

func validateSimpleName(kind, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("migration: %s is required", kind)
	}
	if value == "." || value == ".." || filepath.IsAbs(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\`) || len(value) >= 2 && value[1] == ':' {
		return fmt.Errorf("migration: %s %q must be a simple name", kind, value)
	}
	if strings.ContainsAny(value, `/\`) || filepath.Clean(value) != value {
		return fmt.Errorf("migration: %s %q must be a clean path component", kind, value)
	}
	return nil
}

func ensureLayerDirs(layer layerInfo) error {
	for _, dir := range []string{"files", "skills", "templates"} {
		if err := os.MkdirAll(filepath.Join(layer.Root, dir), 0o755); err != nil {
			return fmt.Errorf("create layer directory %s: %w", filepath.Join(layer.Root, dir), err)
		}
	}
	return nil
}

func itemFromCandidate(kind SourceKind, layer layerInfo, c candidate) (Item, error) {
	if info, err := os.Lstat(c.SourcePath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return Item{}, fmt.Errorf("migration source %s is a symlink; adopt the resolved file or directory explicitly", c.SourcePath)
	}
	storeRel := c.StoreRel
	if storeRel == "" {
		root := "files"
		if c.Mode == manifest.ModeRender {
			root = "templates"
		}
		if c.IsSkill {
			root = "skills"
		}
		storeRel = joinSlash(root, c.SourceRel)
	}
	storeRel, err := cleanRelative(storeRel)
	if err != nil {
		return Item{}, err
	}
	storePath := filepath.Join(layer.Root, filepath.FromSlash(storeRel))
	manifestSource := filepath.ToSlash(storeRel)
	if c.TargetPath == "" {
		targetPath, err := expandTarget(config.PathResolver{}, c.Target)
		if err != nil {
			return Item{}, err
		}
		c.TargetPath = targetPath
	}
	importedHash := ""
	if hash, err := activation.HashPath(c.SourcePath); err == nil {
		importedHash = hash
	}
	collision, existingHash := collisionFor(storePath, importedHash)
	item := Item{
		ID:             makeID(c.SourceRel),
		SourceKind:     kind,
		SourcePath:     c.SourcePath,
		StorePath:      storePath,
		ManifestPath:   layer.ManifestPath,
		ManifestSource: manifestSource,
		Target:         c.Target,
		TargetPath:     c.TargetPath,
		Profile:        layer.Profile,
		Bucket:         layer.Bucket,
		LayerKind:      layer.Kind,
		LayerName:      layer.Name,
		Mode:           c.Mode,
		Format:         c.Format,
		Secrets:        cloneStrings(c.Secrets),
		IsSkill:        c.IsSkill,
		Collision:      collision,
		ExistingHash:   existingHash,
		ImportedHash:   importedHash,
	}
	if c.IsSkill {
		item.SkillSource = manifestSource
	}
	return item, nil
}

func collisionFor(storePath, importedHash string) (CollisionStatus, string) {
	if _, err := os.Lstat(storePath); os.IsNotExist(err) {
		return CollisionNone, ""
	}
	existingHash := ""
	if hash, err := activation.HashPath(storePath); err == nil {
		existingHash = hash
	}
	if existingHash != "" && importedHash != "" && existingHash == importedHash {
		return CollisionIdentical, existingHash
	}
	return CollisionUpdate, existingHash
}

func sourcePathForAdoptedTarget(path string) (sourcePath string, adoptedTargetHash string, symlink bool, err error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", "", false, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, "", false, nil
	}
	if _, err := os.Stat(path); err != nil {
		return "", "", true, fmt.Errorf("broken symlink: %w", err)
	}
	hash, err := activation.HashPath(path)
	if err != nil {
		return "", "", true, err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", "", true, err
	}
	return resolved, hash, true, nil
}

func validateSymlinkAdoptionMode(targetPath, mode string) error {
	switch mode {
	case manifest.ModeMerge, manifest.ModeRender:
		return fmt.Errorf("adopt target %s: %s mode requires a regular file, not a symlink", targetPath, mode)
	default:
		return nil
	}
}

func targetExistsWithHash(targetPath, sourceHash string) bool {
	if sourceHash == "" {
		return false
	}
	hash, err := activation.HashPath(targetPath)
	return err == nil && hash == sourceHash
}

func classifyMode(rel, path string, sourceKind SourceKind, explicit string) (string, string, []string, error) {
	if explicit != "" {
		if !validMode(explicit) {
			return "", "", nil, fmt.Errorf("unsupported migration mode %q", explicit)
		}
		format := detectFormat(path)
		if explicit == manifest.ModeMerge {
			info, err := os.Lstat(path)
			if err != nil {
				return "", "", nil, err
			}
			if info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !structuredFormat(format) {
				return "", "", nil, fmt.Errorf("merge mode requires a regular json, yaml, or toml file")
			}
		}
		secrets, err := detectSecretsIfNeeded(path, explicit)
		return explicit, format, secrets, err
	}
	format := detectFormat(path)
	secrets, err := detectAutoSecrets(path)
	if err == nil && len(secrets) > 0 {
		return manifest.ModeRender, format, secrets, nil
	}
	if sourceKind == SourceAdopt {
		return manifest.ModeCopy, format, nil, nil
	}
	if isAppMutated(rel) {
		return manifest.ModeCopy, format, nil, nil
	}
	if isStableDotfile(rel) {
		return manifest.ModeSymlink, format, nil, nil
	}
	return manifest.ModeCopy, format, nil, nil
}

func validMode(mode string) bool {
	switch mode {
	case manifest.ModeCopy, manifest.ModeSymlink, manifest.ModeMerge, manifest.ModeRender:
		return true
	default:
		return false
	}
}

func detectFormat(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return manifest.FormatJSON
	case ".yaml", ".yml":
		return manifest.FormatYAML
	case ".toml":
		return manifest.FormatTOML
	default:
		return manifest.FormatText
	}
}

func structuredFormat(format string) bool {
	switch format {
	case manifest.FormatJSON, manifest.FormatYAML, manifest.FormatTOML:
		return true
	default:
		return false
	}
}

func detectSecretsIfNeeded(path, mode string) ([]string, error) {
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scan template placeholders %s: %w", path, err)
	}
	secrets := infisical.RequiredSecrets(content, nil)
	if len(secrets) == 0 {
		return nil, nil
	}
	if err := infisical.ValidateSecretNames(secrets); err != nil {
		if mode == manifest.ModeRender {
			return nil, err
		}
		return nil, nil
	}
	return secrets, nil
}

func detectAutoSecrets(path string) ([]string, error) {
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil
	}
	name := strings.ToLower(filepath.Base(path))
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scan template placeholders %s: %w", path, err)
	}
	text := string(content)
	if !strings.Contains(text, "{{") && !strings.Contains(name, "tmpl") && !strings.Contains(name, "template") {
		return nil, nil
	}
	secrets := infisical.RequiredSecrets(content, nil)
	if len(secrets) == 0 {
		return nil, nil
	}
	if err := infisical.ValidateSecretNames(secrets); err != nil {
		return nil, nil
	}
	return secrets, nil
}

func isAppMutated(rel string) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	return strings.HasSuffix(rel, "/settings.json") || strings.HasSuffix(rel, "/mcp.json") || rel == "settings.json" || rel == "mcp.json"
}

func isStableDotfile(rel string) bool {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(rel)))
	switch base {
	case ".gitconfig", ".zshrc", ".bashrc", ".profile", ".bash_profile", ".vimrc", ".tmux.conf", "config", "ssh_config":
		return true
	default:
		return strings.HasPrefix(base, ".") && !strings.Contains(base, "history")
	}
}

func expandTarget(resolver config.PathResolver, target string) (string, error) {
	resolver = resolver.WithDefaults()
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("target is required")
	}
	if !strings.HasPrefix(target, "~") && !isAbsForRuntime(target) {
		abs, err := filepath.Abs(target)
		if err != nil {
			return "", err
		}
		target = abs
	}
	expander := manifest.Expander{Resolver: resolver}
	expanded, err := expander.ExpandTarget(target)
	if err != nil {
		return "", err
	}
	if err := expander.ValidateTargetPath(expanded); err != nil {
		return "", err
	}
	return expanded, nil
}

func isAbsForRuntime(value string) bool {
	return filepath.IsAbs(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\`) || len(value) >= 3 && value[1] == ':'
}

func homeRelativeTargetSpec(resolver config.PathResolver, path string) (string, string, error) {
	resolver = resolver.WithDefaults()
	home := filepath.Clean(resolver.HomeDir)
	cleaned := filepath.Clean(path)
	if home == "" {
		return "", "", fmt.Errorf("home directory is required")
	}
	rel, err := filepath.Rel(home, cleaned)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("target %s is outside home %s", path, home)
	}
	rel = filepath.ToSlash(rel)
	return "~/" + rel, rel, nil
}

func cleanRelative(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, `\`, "/"))
	value = strings.TrimPrefix(value, "~/")
	value = strings.TrimPrefix(value, "./")
	if value == "" {
		return "", fmt.Errorf("relative path is required")
	}
	if strings.HasPrefix(value, "/") || len(value) >= 2 && value[1] == ':' {
		return "", fmt.Errorf("relative path %q must not be absolute", value)
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("relative path %q escapes root", value)
	}
	return cleaned, nil
}

func repoTargetRel(rel string) (string, error) {
	rel, err := cleanRelative(rel)
	if err != nil {
		return "", err
	}
	for _, prefix := range []string{"home/", "HOME/"} {
		if strings.HasPrefix(rel, prefix) {
			return cleanRelative(strings.TrimPrefix(rel, prefix))
		}
	}
	return rel, nil
}

func joinSlash(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.ReplaceAll(part, `\`, "/"), "/")
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return strings.Join(cleaned, "/")
}

func makeID(rel string) string {
	rel = strings.Trim(strings.ToLower(filepath.ToSlash(rel)), "/")
	if rel == "" {
		return "item"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range rel {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "item"
	}
	return out
}

func uniquifyIDs(items []Item) []Item {
	used := map[string]bool{}
	next := map[string]int{}
	for i := range items {
		base := items[i].ID
		id := base
		if used[id] {
			n := next[base]
			if n < 2 {
				n = 2
			}
			for {
				id = fmt.Sprintf("%s-%d", base, n)
				n++
				if !used[id] {
					break
				}
			}
			next[base] = n
		}
		items[i].ID = id
		used[id] = true
	}
	return items
}

func promoteDuplicateStructuredTargets(items []Item) []Item {
	counts := map[string]int{}
	allStructured := map[string]bool{}
	for _, item := range items {
		counts[item.Target]++
		if _, ok := allStructured[item.Target]; !ok {
			allStructured[item.Target] = true
		}
		if !structuredFormat(item.Format) {
			allStructured[item.Target] = false
		}
	}
	for i := range items {
		if counts[items[i].Target] > 1 && allStructured[items[i].Target] {
			items[i].Mode = manifest.ModeMerge
		}
	}
	return items
}

func shouldSkipRepoPath(rel string, isDir bool) bool {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" {
		return false
	}
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		switch part {
		case ".git", ".hg", ".svn", "node_modules", "vendor", "dist", "build", ".DS_Store":
			return true
		}
	}
	if !isDir && secretLikeRE.MatchString(rel) {
		return true
	}
	return false
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	sort.Strings(out)
	return out
}
