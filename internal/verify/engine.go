package verify

import (
	"context"
	"fmt"

	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/machine"
	"github.com/allensu/loki-profile-manager/internal/manifest"
	"github.com/allensu/loki-profile-manager/internal/profile"
	"github.com/allensu/loki-profile-manager/internal/skills"
	"github.com/allensu/loki-profile-manager/internal/store"
)

type Request struct {
	StorePath     string
	ParentProfile string
	Buckets       []string
	MachineID     string
	MachineIDPath string
	Resolver      config.PathResolver
}

func Run(ctx context.Context, req Request) Report {
	report := Report{Valid: true, StorePath: req.StorePath, Profile: req.ParentProfile, Buckets: clone(req.Buckets), Issues: []Issue{}}
	if req.StorePath == "" {
		report.Add(Issue{Severity: SeverityBlocking, Code: "store.not_configured", Message: "store path is required", Remediation: "Provide --store or initialize a Loki store."})
		return report
	}
	validation := store.ValidateLayout(req.StorePath)
	if !validation.Valid {
		for _, missing := range validation.Missing {
			report.Add(Issue{Severity: SeverityBlocking, Code: "store.layout_missing", Message: "required store path is missing", Path: missing, Remediation: "Run store initialization or repair the Loki store layout."})
		}
		return report
	}

	parent := req.ParentProfile
	buckets := clone(req.Buckets)
	machineID := req.MachineID
	if machineID == "" && req.MachineIDPath != "" {
		if id, ok, err := machine.ReadID(req.MachineIDPath); err == nil && ok {
			machineID = id
		} else if err != nil {
			report.Add(Issue{Severity: SeverityWarning, Code: "machine.id_unreadable", Message: err.Error(), Path: req.MachineIDPath})
		}
	}
	if parent == "" && machineID != "" {
		if record, ok, err := machine.GetMachine(req.StorePath, machineID); err == nil && ok && record.ActiveProfile != "" {
			parent = record.ActiveProfile
			buckets = clone(record.ActiveBuckets)
			report.Profile = parent
			report.Buckets = buckets
		}
	}

	var layers []profile.Layer
	if parent != "" {
		resolved, err := profile.Resolve(req.StorePath, parent, buckets)
		if err != nil {
			report.Add(Issue{Severity: SeverityBlocking, Code: "profile.resolve_failed", Message: err.Error(), Remediation: "Check parent profile and bucket names."})
			return report
		}
		layers = resolved
		checkMachinePolicy(&report, req.StorePath, machineID, parent, buckets)
	} else {
		report.Add(Issue{Severity: SeverityInfo, Code: "profile.no_active", Message: "no active or requested profile; merge dry-run skipped"})
		all, err := profile.LoadAllManifests(req.StorePath)
		if err != nil {
			report.Add(Issue{Severity: SeverityBlocking, Code: "profile.load_all_failed", Message: err.Error()})
			return report
		}
		layers = all
	}

	operations := []manifest.FileOperation{}
	for _, layer := range layers {
		report.Layers = append(report.Layers, LayerSummary{Name: layer.Name, Kind: string(layer.Kind), ManifestPath: layer.ManifestPath})
		expander := manifest.Expander{Resolver: req.Resolver, Targets: layer.Manifest.Targets}
		result := manifest.ValidateLayer(manifest.ValidationInput{LayerName: layer.Name, LayerRoot: layer.RootDir, Manifest: layer.Manifest, Expander: expander})
		for _, problem := range result.Problems {
			issue := FromManifestProblem(problem)
			if issue.Layer == "" {
				issue.Layer = layer.Name
			}
			report.Add(issue)
		}
		operations = append(operations, result.Operations...)
		validateLayerSkills(&report, layer)
	}
	if parent != "" {
		for _, problem := range manifest.MergeDryRun(operations) {
			report.Add(FromManifestProblem(problem))
		}
	}
	report.Valid = report.Summary.Blocking == 0
	return report
}

func checkMachinePolicy(report *Report, storeRoot, machineID, parent string, buckets []string) {
	if machineID == "" {
		report.Add(Issue{Severity: SeverityWarning, Code: "machine.id_missing", Message: "machine id not found; policy check skipped"})
		return
	}
	record, ok, err := machine.GetMachine(storeRoot, machineID)
	if err != nil {
		report.Add(Issue{Severity: SeverityWarning, Code: "machine.registry_unreadable", Message: err.Error()})
		return
	}
	if !ok {
		report.Add(Issue{Severity: SeverityWarning, Code: "machine.record_missing", Message: fmt.Sprintf("machine %s is not registered; policy check skipped", machineID)})
		return
	}
	if err := machine.ValidatePolicy(record, parent, buckets); err != nil {
		report.Add(Issue{Severity: SeverityBlocking, Code: "machine.policy_blocked", Message: err.Error(), Remediation: "Update registry/machines.json allowed_parent_profiles or allowed_buckets for this machine."})
	}
}

func validateLayerSkills(report *Report, layer profile.Layer) {
	for _, skillEntry := range layer.Manifest.Skills {
		dirs, err := manifest.SkillSourceDirs(layer.RootDir, skillEntry)
		if err != nil {
			report.Add(Issue{Severity: SeverityBlocking, Code: "skill.source_invalid", Message: err.Error(), Layer: layer.Name})
			continue
		}
		if len(dirs) == 0 {
			report.Add(Issue{Severity: SeverityInfo, Code: "skill.source_empty", Message: "skill source has no SKILL.md folders", Layer: layer.Name})
			continue
		}
		for _, dir := range dirs {
			result := skills.ValidateFolder(dir)
			for _, skillIssue := range result.Issues {
				severity := SeverityBlocking
				if result.Valid {
					severity = SeverityInfo
				}
				report.Add(Issue{Severity: severity, Code: skillIssue.Code, Message: skillIssue.Message, Path: skillIssue.Path, Layer: layer.Name})
			}
		}
	}
}

func clone(values []string) []string {
	if values == nil {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
