package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
)

func newSecretsConfigureCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure secret providers interactively.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newSecretsConfigureInfisicalCommand(resolver, globals, factory))
	return cmd
}

func newSecretsConfigureInfisicalCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "infisical",
		Short: "Interactively configure Infisical machine authentication.",
		Long:  "Interactively configure Infisical machine authentication. Values are validated before writing; secret values are written only to the local Infisical env file and are never printed.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			prompter := newSecretsPrompter(cmd.InOrStdin(), cmd.ErrOrStderr())
			projectID, err := prompter.readLine("Infisical project ID", "", true)
			if err != nil {
				return err
			}
			environment, err := prompter.readLine("Infisical environment", "dev", true)
			if err != nil {
				return err
			}
			clientID, err := prompter.readLine("Infisical client ID", "", true)
			if err != nil {
				return err
			}
			clientSecret, err := prompter.readSecret("Infisical client secret/key")
			if err != nil {
				return err
			}
			hostURL, err := prompter.readLine("Infisical host/API URL", "", false)
			if err != nil {
				return err
			}
			confirmed, err := prompter.confirm("Write local Infisical config?", true)
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Fprintln(cmd.OutOrStdout(), "Infisical configuration cancelled.")
				return nil
			}

			svc, err := factory(cmd.Context(), app.Options{Resolver: resolver, StoreOverride: globals.store, Verbose: globals.verbose, Stderr: cmd.ErrOrStderr()})
			if err != nil {
				return err
			}
			defer svc.Close()
			result, err := svc.SecretsConfigureInfisical(cmd.Context(), app.SecretsConfigureInfisicalRequest{
				ProjectID:         projectID,
				Environment:       environment,
				ClientID:          clientID,
				ClientSecret:      clientSecret,
				HostURL:           hostURL,
				OverwriteExisting: true,
				SkipVerify:        true,
			})
			if outputErr := printSecretsConfigureInfisical(cmd.OutOrStdout(), result); outputErr != nil {
				return outputErr
			}
			if err != nil {
				return err
			}
			if len(result.Missing) > 0 {
				return fmt.Errorf("secrets configure infisical: configuration is incomplete")
			}
			return nil
		},
	}
	return cmd
}

type secretsPrompter struct {
	reader *bufio.Reader
	in     io.Reader
	out    io.Writer
}

func newSecretsPrompter(in io.Reader, out io.Writer) secretsPrompter {
	return secretsPrompter{reader: bufio.NewReader(in), in: in, out: out}
}

func (p secretsPrompter) readLine(label, defaultValue string, required bool) (string, error) {
	prompt := label
	if defaultValue != "" {
		prompt += " [" + defaultValue + "]"
	}
	prompt += ": "
	fmt.Fprint(p.out, prompt)
	value, err := p.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read %s: %w", label, err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = defaultValue
	}
	if required && value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	return value, nil
}

func (p secretsPrompter) readSecret(label string) (string, error) {
	if file, ok := p.in.(*os.File); ok && term.IsTerminal(file.Fd()) {
		fmt.Fprintf(p.out, "%s: ", label)
		data, err := term.ReadPassword(file.Fd())
		fmt.Fprintln(p.out)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", label, err)
		}
		value := strings.TrimSpace(string(data))
		if value == "" {
			return "", fmt.Errorf("%s is required", label)
		}
		return value, nil
	}
	return p.readLine(label, "", true)
}

func (p secretsPrompter) confirm(label string, defaultYes bool) (bool, error) {
	defaultValue := "y"
	if !defaultYes {
		defaultValue = "n"
	}
	value, err := p.readLine(label+" (y/n)", defaultValue, true)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("confirmation must be yes or no")
	}
}
