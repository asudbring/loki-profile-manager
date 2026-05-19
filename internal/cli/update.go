package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
)

func newUpdateCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Install the latest Loki npm build.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newCLIService(cmd, resolver, globals, factory)
			if err != nil {
				return err
			}
			defer svc.Close()

			result, err := svc.Update(cmd.Context(), app.UpdateRequest{})
			if err != nil {
				return err
			}
			printUpdateResult(cmd, result)
			return nil
		},
	}
	return cmd
}

func printUpdateResult(cmd *cobra.Command, result app.UpdateResult) {
	out := cmd.OutOrStdout()
	if result.Updated {
		fmt.Fprintln(out, "Loki update complete")
		if result.LatestVersion != "" {
			fmt.Fprintf(out, "Version: %s\n", result.LatestVersion)
		}
		if result.Message != "" {
			fmt.Fprintln(out, result.Message)
		}
		return
	}
	if result.Message != "" {
		fmt.Fprintln(out, result.Message)
	}
}
