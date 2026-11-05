// Package command handles process-level options independently of terminal UI.
package command

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/charmbracelet/x/term"
	"github.com/mapherez/nox-cli/internal/buildinfo"
	"github.com/mapherez/nox-cli/internal/config"
	"github.com/mapherez/nox-cli/internal/install"
	"github.com/mapherez/nox-cli/internal/tui"
	"os"
)

const usage = `Usage: nox [--help | --version]

Start NoX CLI in an interactive terminal.

Options:
  --help       Show this help.
  --version    Show version and build information.

Configure a private Gateway in Settings. App navigation is not available yet.
`

// Run returns a process exit code. Help/version never require a terminal.
func Run(ctx context.Context, args []string, input io.Reader, output, diagnostics io.Writer) int {
	if len(args) == 1 && args[0] == "--complete-install" {
		root, rootErr := config.Root()
		source, sourceErr := os.Executable()
		if rootErr != nil || sourceErr != nil {
			fmt.Fprintln(diagnostics, install.ErrInstall)
			return 1
		}
		if err := install.Complete(root, source, buildinfo.Version); err != nil {
			fmt.Fprintln(diagnostics, err.Error())
			return 1
		}
		fmt.Fprintln(output, "NoX CLI installed. Open a new terminal and run nox.")
		return 0
	}
	if len(args) == 1 {
		switch args[0] {
		case "--help", "-h":
			fmt.Fprint(output, usage)
			return 0
		case "--version", "-v":
			fmt.Fprintln(output, buildinfo.String())
			return 0
		}
	}
	if len(args) != 0 {
		// Do not echo unexpected arguments: they may contain a pasted secret.
		fmt.Fprintln(diagnostics, "Unsupported arguments. Run nox --help.")
		return 2
	}
	if !isTerminal(input) || !isTerminal(output) {
		fmt.Fprintln(diagnostics, "An interactive terminal is required. Run nox --help or nox --version for non-interactive output.")
		return 2
	}
	if err := tui.Run(ctx, input, output, buildinfo.Version); err != nil {
		if errors.Is(err, context.Canceled) {
			return 130
		}
		// Raw framework/OS errors are not an application diagnostic surface.
		fmt.Fprintln(diagnostics, "Unable to run the terminal interface.")
		return 1
	}
	return 0
}

func isTerminal(stream any) bool {
	file, ok := stream.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(file.Fd())
}
