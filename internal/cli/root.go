package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
)

type ServiceFactory func(context.Context, app.Options) (*app.Service, error)

type Options struct {
	Resolver     config.PathResolver
	Out          io.Writer
	Err          io.Writer
	Factory      ServiceFactory
	TUIRunner    TUIRunner
	UpdateRunner app.UpdateCommandRunner
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
	factory = serviceFactoryWithUpdateRunner(factory, opts.UpdateRunner)

	cmd := &cobra.Command{
		Use:           "loki",
		Short:         "Manage local profiles, dotfiles, and AI tool skills.",
		Long:          "Loki Profile Manager manages local dotfile/profile stores and AI tool skills. Running `loki` with no arguments launches the terminal UI; commands and flags use CLI mode.",
		Version:       app.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return maybePrintUpdateNotice(cmd, opts.Resolver, globals, factory)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && globals.store == "" && !globals.verbose {
				return runTUIFromCommand(cmd, opts.Resolver, globals, factory, opts.TUIRunner)
			}
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
	cmd.AddCommand(newImportPluginCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newSecretsCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newSnapshotsCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newStoreCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newMigrateCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newAdoptCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newMachineCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newDoctorCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newUpdateCommand(opts.Resolver, globals, factory))
	cmd.AddCommand(newTUICommand(opts.Resolver, globals, factory, opts.TUIRunner))
	return cmd
}

func serviceFactoryWithUpdateRunner(factory ServiceFactory, runner app.UpdateCommandRunner) ServiceFactory {
	if runner == nil {
		return factory
	}
	return func(ctx context.Context, opts app.Options) (*app.Service, error) {
		if opts.UpdateRunner == nil {
			opts.UpdateRunner = runner
		}
		return factory(ctx, opts)
	}
}

func maybePrintUpdateNotice(cmd *cobra.Command, resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) error {
	if shouldSkipUpdateNotice(cmd) {
		return nil
	}
	svc, err := factory(cmd.Context(), app.Options{
		Resolver:      resolver,
		StoreOverride: globals.store,
		Verbose:       globals.verbose,
		Stderr:        cmd.ErrOrStderr(),
	})
	if err != nil {
		return nil
	}
	defer svc.Close()

	result, err := svc.CheckForUpdate(cmd.Context(), app.UpdateCheckRequest{})
	if err != nil || !result.Available || result.Message == "" {
		return nil
	}
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), result.Message)
	return nil
}

func shouldSkipUpdateNotice(cmd *cobra.Command) bool {
	if cmd == nil || cmd == cmd.Root() {
		return true
	}
	if cmd.CommandPath() == "loki update" || cmd.CommandPath() == "loki tui" || cmd.Name() == "help" {
		return true
	}
	if commandBoolFlag(cmd, "help") || commandBoolFlag(cmd, "version") || commandBoolFlag(cmd, "json") {
		return true
	}
	if truthyEnv(app.UpdateDisableEnvVar) || updateNoticeCIEnvSet() {
		return true
	}
	if updateNoticeWouldBeNonInteractive(cmd) {
		return true
	}
	return false
}

func commandBoolFlag(cmd *cobra.Command, name string) bool {
	for c := cmd; c != nil; c = c.Parent() {
		for _, flags := range []*pflag.FlagSet{c.Flags(), c.PersistentFlags(), c.InheritedFlags()} {
			flag := flags.Lookup(name)
			if flag == nil {
				continue
			}
			value, err := strconv.ParseBool(flag.Value.String())
			if err == nil && value {
				return true
			}
		}
	}
	return false
}

func updateNoticeCIEnvSet() bool {
	for _, name := range []string{"CI", "GITHUB_ACTIONS", "TF_BUILD", "BUILD_BUILDID"} {
		if truthyEnv(name) {
			return true
		}
	}
	return false
}

func truthyEnv(name string) bool {
	value := strings.TrimSpace(os.Getenv(name))
	return value != "" && value != "0" && !strings.EqualFold(value, "false")
}

func updateNoticeWouldBeNonInteractive(cmd *cobra.Command) bool {
	file, ok := cmd.ErrOrStderr().(*os.File)
	if !ok {
		return false
	}
	return !term.IsTerminal(file.Fd())
}
