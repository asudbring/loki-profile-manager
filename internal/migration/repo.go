package migration

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/allensu/loki-profile-manager/internal/config"
)

type RepoRequest struct {
	BuildRequest
	Resolver config.PathResolver
	RepoPath string
}

func BuildRepoPlan(req RepoRequest) (Plan, error) {
	layer, err := resolveLayer(req.StorePath, req.Profile, req.Bucket)
	if err != nil {
		return Plan{}, err
	}
	resolver := req.Resolver.WithDefaults()
	plan := newPlan(SourceRepo, layer, req.Now)
	rawRepoPath := strings.TrimSpace(req.RepoPath)
	if rawRepoPath == "" {
		return plan, fmt.Errorf("migrate repo: repo path is required")
	}
	repoRoot, err := filepath.Abs(rawRepoPath)
	if err != nil {
		return plan, fmt.Errorf("migrate repo %s: %w", rawRepoPath, err)
	}
	info, err := os.Stat(repoRoot)
	if err != nil {
		return plan, fmt.Errorf("migrate repo %s: %w", repoRoot, err)
	}
	if !info.IsDir() {
		return plan, fmt.Errorf("migrate repo %s: not a directory", repoRoot)
	}
	skillHashes := map[string]string{}
	err = filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == repoRoot {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if shouldSkipRepoPath(rel, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("skipped sensitive or generated path %s", rel))
			return nil
		}
		if entry.IsDir() {
			if isSkillDir(path) {
				item, err := repoSkillItem(layer, resolver, repoRoot, path, rel, skillHashes)
				if err != nil {
					return err
				}
				if item.Warning != "" {
					plan.Warnings = append(plan.Warnings, item.Warning)
					if item.Collision == CollisionConflict {
						plan.Items = append(plan.Items, item)
					}
					return filepath.SkipDir
				}
				plan.Items = append(plan.Items, item)
				return filepath.SkipDir
			}
			return nil
		}
		item, err := repoFileItem(layer, resolver, path, rel)
		if err != nil {
			return err
		}
		if item.Warning != "" {
			plan.Warnings = append(plan.Warnings, item.Warning)
		}
		plan.Items = append(plan.Items, item)
		return nil
	})
	if err != nil {
		return plan, err
	}
	plan.Items = promoteDuplicateStructuredTargets(plan.Items)
	markRepoAdoptionRecords(plan.Items)
	plan.Items = uniquifyIDs(plan.Items)
	return plan, nil
}

func repoFileItem(layer layerInfo, resolver config.PathResolver, path, rel string) (Item, error) {
	sourceRel, err := cleanRelative(rel)
	if err != nil {
		return Item{}, err
	}
	targetRel, err := repoTargetRel(rel)
	if err != nil {
		return Item{}, err
	}
	targetSpec := "~/" + targetRel
	targetPath, err := expandTarget(resolver, targetSpec)
	if err != nil {
		return Item{}, err
	}
	mode, format, secrets, err := classifyMode(targetRel, path, SourceRepo, "")
	if err != nil {
		return Item{}, err
	}
	storeRoot := "files"
	if mode == "render" {
		storeRoot = "templates"
	}
	item, err := itemFromCandidate(SourceRepo, layer, candidate{
		SourcePath: path,
		SourceRel:  sourceRel,
		Target:     targetSpec,
		TargetPath: targetPath,
		Mode:       mode,
		Format:     format,
		Secrets:    secrets,
		StoreRel:   joinSlash(storeRoot, sourceRel),
	})
	if err != nil {
		return Item{}, err
	}
	if item.Collision == CollisionUpdate {
		item.Collision = CollisionConflict
		item.Warning = fmt.Sprintf("store destination already exists with different content: %s", item.StorePath)
	}
	return item, nil
}

func repoSkillItem(layer layerInfo, resolver config.PathResolver, repoRoot, path, rel string, skillHashes map[string]string) (Item, error) {
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
	targetRel := joinSlash(".pi/agent/skills", name)
	targetSpec := "~/" + targetRel
	targetPath, err := expandTarget(resolver, targetSpec)
	if err != nil {
		return Item{}, err
	}
	item, err := itemFromCandidate(SourceRepo, layer, candidate{
		SourcePath: path,
		SourceRel:  joinSlash("pi", name),
		Target:     targetSpec,
		TargetPath: targetPath,
		Mode:       "copy",
		Format:     "text",
		IsSkill:    true,
		StoreRel:   joinSlash("skills", "pi", name),
	})
	if err != nil {
		return Item{}, err
	}
	if item.Collision == CollisionUpdate {
		item.Collision = CollisionConflict
		item.Warning = fmt.Sprintf("store destination already exists with different content: %s", item.StorePath)
	}
	return item, nil
}

func markRepoAdoptionRecords(items []Item) {
	for i := range items {
		if items[i].Collision == CollisionConflict || items[i].ImportedHash == "" {
			continue
		}
		items[i].WillAdoptRecord = targetExistsWithHash(items[i].TargetPath, items[i].ImportedHash)
	}
}
