package activation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/manifest"
	"github.com/allensu/loki-profile-manager/internal/profile"
)

type PlanRequest struct {
	StorePath string
	Profile   string
	Buckets   []string
	Resolver  config.PathResolver
	Now       func() time.Time
}

func BuildPlan(ctx context.Context, req PlanRequest) (Plan, error) {
	_ = ctx
	if strings.TrimSpace(req.StorePath) == "" {
		return Plan{}, fmt.Errorf("build activation plan: store path is required")
	}
	if strings.TrimSpace(req.Profile) == "" {
		return Plan{}, fmt.Errorf("build activation plan: profile is required")
	}
	now := time.Now
	if req.Now != nil {
		now = req.Now
	}
	plan := NewPlan(req.StorePath, req.Profile, req.Buckets, now())
	layers, err := profile.Resolve(req.StorePath, req.Profile, req.Buckets)
	if err != nil {
		return plan, fmt.Errorf("build activation plan: %w", err)
	}

	var fileOps []manifest.FileOperation
	var problems []manifest.Problem
	layerKinds := map[string]string{}
	for _, layer := range layers {
		layerKinds[layer.Name] = string(layer.Kind)
		expander := manifest.Expander{Resolver: req.Resolver, Targets: layer.Manifest.Targets}
		result := manifest.ValidateLayer(manifest.ValidationInput{LayerName: layer.Name, LayerRoot: layer.RootDir, Manifest: layer.Manifest, Expander: expander})
		problems = append(problems, result.Problems...)
		fileOps = append(fileOps, result.Operations...)
	}
	if err := blockingProblems("validate manifests", problems); err != nil {
		return plan, err
	}
	mergeProblems := manifest.MergeDryRun(fileOps)
	if err := blockingProblems("validate mergeability", mergeProblems); err != nil {
		return plan, err
	}

	groups := groupFileOperations(fileOps)
	for _, group := range groups {
		op, err := operationFromGroup(group, layerKinds)
		if err != nil {
			return plan, err
		}
		plan.Operations = append(plan.Operations, op)
	}
	return plan, nil
}

func groupFileOperations(ops []manifest.FileOperation) [][]manifest.FileOperation {
	seen := map[string]int{}
	var groups [][]manifest.FileOperation
	for _, op := range ops {
		if idx, ok := seen[op.TargetPath]; ok {
			groups[idx] = append(groups[idx], op)
			continue
		}
		seen[op.TargetPath] = len(groups)
		groups = append(groups, []manifest.FileOperation{op})
	}
	return groups
}

func operationFromGroup(group []manifest.FileOperation, layerKinds map[string]string) (Operation, error) {
	if len(group) == 0 {
		return Operation{}, fmt.Errorf("activation plan: empty operation group")
	}
	if len(group) > 1 && isMergeGroup(group) {
		first := group[0]
		sources := make([]Source, 0, len(group))
		paths := make([]string, 0, len(group))
		for i, op := range group {
			sources = append(sources, sourceFromOperation(op, layerKinds, i))
			paths = append(paths, op.SourcePath)
		}
		content, err := MergeBytes(first.Entry.Format, paths)
		if err != nil {
			return Operation{}, err
		}
		return Operation{
			ID:           first.Entry.ID,
			Type:         OperationMerge,
			Format:       first.Entry.Format,
			TargetPath:   first.TargetPath,
			Sources:      sources,
			LayerName:    "merged",
			LayerKind:    "merged",
			Capture:      anyCapture(group),
			ExpectedHash: HashBytes(content),
			Safety:       SafetyStatus{Class: SafetyUnknown},
		}, nil
	}
	selected := group[len(group)-1]
	typeName := OperationType(selected.Entry.Mode)
	expectedHash := ""
	if typeName == OperationCopy || typeName == OperationSymlink {
		if hash, err := HashPath(selected.SourcePath); err == nil {
			expectedHash = hash
		}
	}
	return Operation{
		ID:           selected.Entry.ID,
		Type:         typeName,
		Format:       selected.Entry.Format,
		TargetPath:   selected.TargetPath,
		SourcePath:   selected.SourcePath,
		Sources:      []Source{sourceFromOperation(selected, layerKinds, 0)},
		LayerName:    selected.LayerName,
		LayerKind:    layerKinds[selected.LayerName],
		Capture:      selected.Entry.Capture,
		Secrets:      cloneStrings(selected.Entry.Secrets),
		ExpectedHash: expectedHash,
		Safety:       SafetyStatus{Class: SafetyUnknown},
	}, nil
}

func isMergeGroup(group []manifest.FileOperation) bool {
	if len(group) == 0 || group[0].Entry.Mode != manifest.ModeMerge {
		return false
	}
	format := group[0].Entry.Format
	for _, op := range group[1:] {
		if op.Entry.Mode != manifest.ModeMerge || op.Entry.Format != format {
			return false
		}
	}
	return true
}

func sourceFromOperation(op manifest.FileOperation, layerKinds map[string]string, order int) Source {
	return Source{Path: op.SourcePath, LayerName: op.LayerName, LayerKind: layerKinds[op.LayerName], FileID: op.Entry.ID, Order: order}
}

func anyCapture(group []manifest.FileOperation) bool {
	for _, op := range group {
		if op.Entry.Capture {
			return true
		}
	}
	return false
}

func blockingProblems(phase string, problems []manifest.Problem) error {
	messages := []string{}
	for _, problem := range problems {
		if problem.Severity == manifest.SeverityBlocking {
			messages = append(messages, problem.Error())
		}
	}
	if len(messages) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %s", phase, strings.Join(messages, "; "))
}
