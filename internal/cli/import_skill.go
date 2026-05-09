package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
)

func newImportSkillCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var common bool
	var profile string
	var bucket string
	var name string
	var dryRun bool
	var yes bool
	var overwrite bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "import-skill <source>",
		Short: "Import a skill folder or .zip archive into a Loki store layer.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := factory(cmd.Context(), app.Options{Resolver: resolver, StoreOverride: globals.store, Verbose: globals.verbose, Stderr: cmd.ErrOrStderr()})
			if err != nil {
				return err
			}
			defer svc.Close()

			result, err := svc.ImportSkill(cmd.Context(), app.ImportSkillRequest{SourceFolder: args[0], Common: common, Profile: profile, Bucket: bucket, Name: name, DryRun: dryRun, Yes: yes, Overwrite: overwrite})
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				if encodeErr := encoder.Encode(result); encodeErr != nil {
					return encodeErr
				}
			} else if err == nil || result.SourcePath != "" || result.DestinationPath != "" {
				printImportSkillResult(cmd, result)
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&common, "common", false, "import into the common layer")
	cmd.Flags().StringVar(&profile, "profile", "", "profile core layer to import into")
	cmd.Flags().StringVar(&bucket, "bucket", "", "optional profile bucket layer to import into")
	cmd.Flags().StringVar(&name, "name", "", "store folder name under skills/ (defaults to source name)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate and show planned import without writing files")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm store writes")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "replace an existing skills/<name> folder")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func printImportSkillResult(cmd *cobra.Command, result app.ImportSkillResult) {
	out := cmd.OutOrStdout()
	if result.DryRun {
		fmt.Fprintln(out, "Loki skill import dry-run")
	} else {
		fmt.Fprintln(out, "Loki skill import complete")
	}
	if result.SourcePath != "" {
		fmt.Fprintf(out, "Source: %s\n", result.SourcePath)
	}
	if result.SourceKind != "" {
		fmt.Fprintf(out, "Source kind: %s\n", result.SourceKind)
	}
	if result.Name != "" {
		fmt.Fprintf(out, "Name: %s\n", result.Name)
	}
	if result.Layer.Kind != "" {
		fmt.Fprintf(out, "Layer: %s", result.Layer.Kind)
		if result.Layer.Profile != "" {
			fmt.Fprintf(out, " profile=%s", result.Layer.Profile)
		}
		if result.Layer.Bucket != "" {
			fmt.Fprintf(out, " bucket=%s", result.Layer.Bucket)
		}
		fmt.Fprintln(out)
	}
	if result.DestinationPath != "" {
		fmt.Fprintf(out, "Destination: %s\n", result.DestinationPath)
	}
	if result.ManifestPath != "" {
		fmt.Fprintf(out, "Manifest: %s\n", result.ManifestPath)
	}
	if result.ManifestSource != "" {
		fmt.Fprintf(out, "Manifest source: %s\n", result.ManifestSource)
	}
	if result.DryRun {
		fmt.Fprintf(out, "Would copy: %t\n", result.WouldCopy)
		fmt.Fprintf(out, "Would overwrite: %t\n", result.WouldOverwrite)
		fmt.Fprintf(out, "Manifest update: %t\n", result.ManifestChanged)
	} else {
		fmt.Fprintf(out, "Changed: %d\n", result.Changed)
	}
	if !result.Validation.Valid && len(result.Validation.Issues) > 0 {
		fmt.Fprintln(out, "Validation issues:")
		for _, issue := range result.Validation.Issues {
			fmt.Fprintf(out, "- %s: %s", issue.Code, issue.Message)
			if issue.Path != "" {
				fmt.Fprintf(out, " (%s)", issue.Path)
			}
			fmt.Fprintln(out)
		}
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "Warning: %s\n", warning)
	}
}
