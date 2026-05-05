package tui

import (
	"context"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

type Options struct {
	StorePath      string
	Verbose        bool
	Input          io.Reader
	Output         io.Writer
	Err            io.Writer
	AllowNonTTY    bool
	AltScreen      bool
	ProgramOptions []tea.ProgramOption
}

func Run(ctx context.Context, client Client, opts Options) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		return fmt.Errorf("tui: client is nil")
	}
	input := opts.Input
	if input == nil {
		input = os.Stdin
	}
	output := opts.Output
	if output == nil {
		output = os.Stdout
	}
	if !opts.AllowNonTTY && (!isReaderTerminal(input) || !isWriterTerminal(output)) {
		return fmt.Errorf("tui requires an interactive terminal; run `loki tui` from a TTY")
	}

	programOptions := []tea.ProgramOption{
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithOutput(output),
	}
	if opts.AltScreen {
		programOptions = append(programOptions, tea.WithAltScreen())
	}
	programOptions = append(programOptions, opts.ProgramOptions...)

	_, err := tea.NewProgram(NewModel(ctx, client), programOptions...).Run()
	return err
}

func isReaderTerminal(input io.Reader) bool {
	file, ok := input.(*os.File)
	if !ok {
		return false
	}
	return isFileTerminal(file)
}

func isWriterTerminal(output io.Writer) bool {
	file, ok := output.(*os.File)
	if !ok {
		return false
	}
	return isFileTerminal(file)
}

func isFileTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
