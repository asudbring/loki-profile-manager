package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/allensu/loki-profile-manager/internal/app"
	"github.com/allensu/loki-profile-manager/internal/config"
	lokiui "github.com/allensu/loki-profile-manager/internal/tui"
)

type TUIRunner func(context.Context, lokiui.Client, lokiui.Options) error

func newTUICommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory, runner TUIRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive Loki terminal UI.",
		Long:  "Launch the interactive Loki terminal UI. The MVP provides guarded profile switch, sync cleanup, snapshots dry-runs, and read-only diagnostics. Requires an interactive terminal.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUIFromCommand(cmd, resolver, globals, factory, runner)
		},
	}
	return cmd
}

func runTUIFromCommand(cmd *cobra.Command, resolver config.PathResolver, globals *globalOptions, factory ServiceFactory, runner TUIRunner) error {
	if runner == nil {
		runner = lokiui.Run
	}
	ctx := cmd.Context()
	svc, err := factory(ctx, app.Options{
		Resolver:      resolver,
		StoreOverride: globals.store,
		Verbose:       globals.verbose,
		Stderr:        cmd.ErrOrStderr(),
	})
	if err != nil {
		return err
	}
	defer svc.Close()

	return runner(ctx, lokiui.NewServiceClient(svc, globals.store), lokiui.Options{
		StorePath: globals.store,
		Verbose:   globals.verbose,
		Input:     cmd.InOrStdin(),
		Output:    cmd.OutOrStdout(),
		Err:       cmd.ErrOrStderr(),
	})
}
