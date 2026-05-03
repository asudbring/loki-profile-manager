package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/manifest"
)

type AdoptRequest struct {
	BuildRequest
	Resolver   config.PathResolver
	Target     string
	Mode       string
	SourceName string
}

func BuildAdoptPlan(req AdoptRequest) (Plan, error) {
	layer, err := resolveLayer(req.StorePath, req.Profile, req.Bucket)
	if err != nil {
		return Plan{}, err
	}
	resolver := req.Resolver.WithDefaults()
	plan := newPlan(SourceAdopt, layer, req.Now)
	targetPath, err := expandTarget(resolver, req.Target)
	if err != nil {
		return plan, fmt.Errorf("adopt target: %w", err)
	}
	info, err := os.Lstat(targetPath)
	if err != nil {
		return plan, fmt.Errorf("adopt target %s: %w", targetPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if _, err := os.Stat(targetPath); err != nil {
			return plan, fmt.Errorf("adopt target %s: broken symlink: %w", targetPath, err)
		}
	}
	targetSpec, homeRel, err := homeRelativeTargetSpec(resolver, targetPath)
	if err != nil {
		return plan, err
	}
	sourceRel := homeRel
	if strings.TrimSpace(req.SourceName) != "" {
		sourceRel, err = cleanRelative(req.SourceName)
		if err != nil {
			return plan, fmt.Errorf("adopt source-name: %w", err)
		}
	}
	mode, format, secrets, err := classifyMode(sourceRel, targetPath, SourceAdopt, strings.TrimSpace(req.Mode))
	if err != nil {
		return plan, err
	}
	if mode == manifest.ModeRender && info.IsDir() {
		return plan, fmt.Errorf("adopt target %s: render mode requires a file", targetPath)
	}
	storeRelRoot := "files"
	if mode == manifest.ModeRender {
		storeRelRoot = "templates"
	}
	item, err := itemFromCandidate(SourceAdopt, layer, candidate{
		SourcePath: targetPath,
		SourceRel:  sourceRel,
		Target:     targetSpec,
		TargetPath: targetPath,
		Mode:       mode,
		Format:     format,
		Secrets:    secrets,
		StoreRel:   joinSlash(storeRelRoot, sourceRel),
	})
	if err != nil {
		return plan, err
	}
	item.WillAdoptRecord = true
	plan.Items = uniquifyIDs([]Item{item})
	plan.GeneratedAt = generatedAt(req.Now)
	return plan, nil
}

func generatedAt(now func() time.Time) string {
	if now == nil {
		now = time.Now
	}
	return now().UTC().Format(time.RFC3339)
}

func sourceNameFromPath(path string) string {
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "target"
	}
	return base
}
