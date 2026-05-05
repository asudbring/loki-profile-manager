package cli

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/allensu/loki-profile-manager/internal/app"
	"github.com/allensu/loki-profile-manager/internal/config"
)

type ServiceFactory func(context.Context, app.Options) (*app.Service, error)

type Options struct {
	Resolver  config.PathResolver
	Out       io.Writer
	Err       io.Writer
	Factory   ServiceFactory
	TUIRunner TUIRunner
}

type globalOptions struct {
	store   string
	verbose bool
}

func Execute() error {
	return NewRootCommand(Options{}).Execute()
}

func NewRootCommand(opts Options) *cobra.Command {
	globals := &globalOptions{}
	factory := opts.Factory
	if factory == nil {
		factory = app.NewService
	}

	cmd := &cobra.Command{
		Use:           "loki",
		Short:         "Manage local profiles, dotfiles, and AI tool skills.",
		Long:          "Loki Profile Manager manages local dotfile/profile stores and AI tool skills. Current commands provide status, verification, and manifest-driven profile switching.",
		Version:       app.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	if opts.Out != nil {
		cmd.SetOut(opts.Out)
	}
	if opts.Err != nil {
		cmd.SetErr(opts.Err)
	}
	cmd.SetVersionTemplate("{{.Version}}\n")

	cmd.PersistentFlags().StringVar(&globals.store, "store", "", "override Loki store root path")
	cmd.PersistentFlags().BoolVar(&globals.verbose, "verbose", false, "print verbose diagnostics to stderr")

	cmd.AddCommand(newStatusCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newVerifyCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newSwitchCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newSyncCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newImportSkillCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newSecretsCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newSnapshotsCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newMigrateCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newAdoptCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newMachineCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newDoctorCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newTUICommand(opts.Resolver, globals, factory, opts.TUIRunner))
	return cmd
}
