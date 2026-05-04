package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/manifest"
)

type LocalRequest struct {
	BuildRequest
	Resolver config.PathResolver
}

func BuildLocalPlan(req LocalRequest) (Plan, error) {
	layer, err := resolveLayer(req.StorePath, req.Profile, req.Bucket)
	if err != nil {
		return Plan{}, err
	}
	resolver := req.Resolver.WithDefaults()
	plan := newPlan(SourceLocal, layer, req.Now)
	if resolver.HomeDir == "" {
		return plan, fmt.Errorf("migrate local: home directory is required")
	}
	skillHashes := map[string]string{}
	for _, path := range localFileCandidates(resolver) {
		if _, err := os.Lstat(path); err != nil {
			continue
		}
		item, err := localPathItem(layer, resolver, path, false, skillHashes)
		if err != nil {
			return plan, err
		}
		plan.Items = append(plan.Items, item)
	}
	for _, root := range localSkillRoots(resolver) {
		items, warnings, err := localSkillItems(layer, resolver, root, skillHashes)
		if err != nil {
			return plan, err
		}
		plan.Warnings = append(plan.Warnings, warnings...)
		plan.Items = append(plan.Items, items...)
	}
	plan.Items = promoteDuplicateStructuredTargets(plan.Items)
	for i := range plan.Items {
		plan.Items[i].WillAdoptRecord = true
	}
	plan.Items = uniquifyIDs(plan.Items)
	return plan, nil
}

func localFileCandidates(resolver config.PathResolver) []string {
	home := resolver.HomeDir
	candidates := []string{
		filepath.Join(home, ".gitconfig"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".profile"),
		filepath.Join(home, ".bash_profile"),
		filepath.Join(home, ".github", "instructions"),
		filepath.Join(home, ".github", "prompts"),
		filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"),
		filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1"),
	}
	for _, path := range vscodeUserPaths(resolver) {
		candidates = append(candidates, filepath.Join(path, "settings.json"), filepath.Join(path, "mcp.json"))
	}
	return candidates
}

func vscodeUserPaths(resolver config.PathResolver) []string {
	home := resolver.HomeDir
	switch resolver.GOOS {
	case "darwin":
		return []string{filepath.Join(home, "Library", "Application Support", "Code", "User")}
	case "windows":
		base := ""
		if resolver.Env != nil {
			base = resolver.Env("APPDATA")
		}
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return []string{filepath.Join(base, "Code", "User")}
	default:
		return []string{filepath.Join(home, ".config", "Code", "User")}
	}
}

func localSkillRoots(resolver config.PathResolver) []string {
	home := resolver.HomeDir
	return []string{
		filepath.Join(home, ".pi", "agent", "skills"),
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".claude", "skills"),
	}
}

func localSkillItems(layer layerInfo, resolver config.PathResolver, root string, skillHashes map[string]string) ([]Item, []string, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("scan skills %s: %w", root, err)
	}
	var items []Item
	var warnings []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if !isSkillDir(path) {
			continue
		}
		item, err := localPathItem(layer, resolver, path, true, skillHashes)
		if err != nil {
			return nil, nil, err
		}
		if item.Warning != "" {
			warnings = append(warnings, item.Warning)
			if item.Collision == CollisionConflict {
				items = append(items, item)
			}
			continue
		}
		items = append(items, item)
	}
	return items, warnings, nil
}

func localPathItem(layer layerInfo, resolver config.PathResolver, path string, isSkill bool, skillHashes map[string]string) (Item, error) {
	targetSpec, rel, err := homeRelativeTargetSpec(resolver, path)
	if err != nil {
		return Item{}, err
	}
	targetPath, err := expandTarget(resolver, targetSpec)
	if err != nil {
		return Item{}, err
	}
	sourcePath, adoptedTargetHash, symlinkTarget, err := sourcePathForAdoptedTarget(path)
	if err != nil {
		return Item{}, fmt.Errorf("migrate local target %s: %w", path, err)
	}
	mode, format, secrets, err := classifyMode(rel, path, SourceLocal, "")
	if err != nil {
		return Item{}, err
	}
	if symlinkTarget {
		if err := validateSymlinkAdoptionMode(path, mode); err != nil {
			return Item{}, err
		}
	}
	storeRel := joinSlash("files", rel)
	if mode == manifest.ModeRender {
		storeRel = joinSlash("templates", rel)
	}
	sourceRel := rel
	if isSkill {
		name := filepath.Base(path)
		hash, err := hashPath(path)
		if err != nil {
			return Item{}, err
		}
		if existing, ok := skillHashes[name]; ok {
			item := Item{Warning: fmt.Sprintf("duplicate skill %s skipped", name)}
			if existing != hash {
				item.Collision = CollisionConflict
				item.Warning = fmt.Sprintf("conflicting skill %s has different content", name)
			}
			return item, nil
		}
		skillHashes[name] = hash
		sourceRel = joinSlash(skillToolName(rel), name)
		storeRel = joinSlash("skills", sourceRel)
		mode = manifest.ModeCopy
		format = manifest.FormatText
	}
	item, err := itemFromCandidate(SourceLocal, layer, candidate{
		SourcePath: sourcePath,
		SourceRel:  sourceRel,
		Target:     targetSpec,
		TargetPath: targetPath,
		Mode:       mode,
		Format:     format,
		Secrets:    secrets,
		IsSkill:    isSkill,
		StoreRel:   storeRel,
	})
	if err != nil {
		return Item{}, err
	}
	item.AdoptedTargetHash = adoptedTargetHash
	return item, nil
}

func skillToolName(rel string) string {
	rel = filepath.ToSlash(rel)
	switch {
	case strings.HasPrefix(rel, ".pi/agent/skills/"):
		return "pi"
	case strings.HasPrefix(rel, ".agents/skills/"):
		return "agents"
	case strings.HasPrefix(rel, ".claude/skills/"):
		return "claude"
	default:
		return "skills"
	}
}
