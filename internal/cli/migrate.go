package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/migration"
)

func newMigrateCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	cmd := &cobra.Command{Use: "migrate", Short: "Migrate existing dotfiles, settings, and skills into Loki."}
	cmd.AddCommand(newMigrateRepoCommand(resolver, globals, factory))
	cmd.AddCommand(newMigrateLocalCommand(resolver, globals, factory))
	return cmd
}

func newMigrateRepoCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var opts migrateFlags
	cmd := &cobra.Command{
		Use:   "repo <path>",
		Short: "Migrate a legacy dotfiles/settings repository.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := factory(cmd.Context(), app.Options{Resolver: resolver, StoreOverride: globals.store, Verbose: globals.verbose, Stderr: cmd.ErrOrStderr()})
			if err != nil {
				return err
			}
			defer svc.Close()
			result, err := svc.MigrateRepo(cmd.Context(), app.MigrateRepoRequest{RepoPath: args[0], Profile: opts.profile, Bucket: opts.bucket, DryRun: opts.dryRun, Yes: opts.yes})
			if outputErr := printMigrateResult(cmd.OutOrStdout(), result, opts.jsonOutput, "repo"); outputErr != nil {
				return outputErr
			}
			return err
		},
	}
	addMigrateFlags(cmd, &opts)
	return cmd
}

func newMigrateLocalCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var opts migrateFlags
	cmd := &cobra.Command{
		Use:   "local",
		Short: "Migrate known settings from the current local home.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := factory(cmd.Context(), app.Options{Resolver: resolver, StoreOverride: globals.store, Verbose: globals.verbose, Stderr: cmd.ErrOrStderr()})
			if err != nil {
				return err
			}
			defer svc.Close()
			result, err := svc.MigrateLocal(cmd.Context(), app.MigrateLocalRequest{Profile: opts.profile, Bucket: opts.bucket, DryRun: opts.dryRun, Yes: opts.yes})
			if outputErr := printMigrateResult(cmd.OutOrStdout(), result, opts.jsonOutput, "local"); outputErr != nil {
				return outputErr
			}
			return err
		},
	}
	addMigrateFlags(cmd, &opts)
	return cmd
}

type migrateFlags struct {
	profile    string
	bucket     string
	dryRun     bool
	yes        bool
	jsonOutput bool
}

func addMigrateFlags(cmd *cobra.Command, opts *migrateFlags) {
	cmd.Flags().StringVar(&opts.profile, "profile", "", "profile to migrate into")
	cmd.Flags().StringVar(&opts.bucket, "bucket", "", "optional bucket to migrate into")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "show migration plan without writing files")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "confirm store and local-state writes")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "emit machine-readable JSON")
	_ = cmd.MarkFlagRequired("profile")
}

func printMigrateResult(out io.Writer, result app.MigrateResult, jsonOutput bool, source string) error {
	if jsonOutput {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	if result.DryRun {
		fmt.Fprintf(out, "Loki %s migration dry-run\n", source)
	} else if result.Changed > 0 {
		fmt.Fprintf(out, "Loki %s migration complete\n", source)
	}
	printMigrationPlanSummary(out, result.Plan)
	return nil
}

func printMigrationPlanSummary(out io.Writer, plan migration.Plan) {
	if plan.Profile != "" {
		fmt.Fprintf(out, "Profile: %s\n", plan.Profile)
	}
	if plan.Bucket != "" {
		fmt.Fprintf(out, "Bucket: %s\n", plan.Bucket)
	}
	fmt.Fprintf(out, "Items: %d\n", len(plan.Items))
	for _, warning := range plan.Warnings {
		fmt.Fprintf(out, "Warning: %s\n", warning)
	}
	for _, item := range plan.Items {
		fmt.Fprintf(out, "- %s %s <- %s", item.Mode, item.Target, item.ManifestSource)
		if item.IsSkill {
			fmt.Fprint(out, " [skill]")
		}
		if len(item.Secrets) > 0 {
			fmt.Fprintf(out, " [secrets:%d]", len(item.Secrets))
		}
		if item.WillAdoptRecord {
			fmt.Fprint(out, " [adopt-record]")
		}
		if item.Collision != "" && item.Collision != "none" {
			fmt.Fprintf(out, " [%s]", item.Collision)
		}
		fmt.Fprintln(out)
	}
}
