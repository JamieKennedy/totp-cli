// Package cli implements the totp command-line interface.
package cli

import (
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/JamieKennedy/totp-cli/internal/store"
	"github.com/JamieKennedy/totp-cli/internal/ui"
)

// Clipboard is the subset of clipboard access the CLI needs.
type Clipboard interface {
	ReadAll() (string, error)
	WriteAll(text string) error
}

// App holds the dependencies shared by all commands.
type App struct {
	Store *store.Store
	In    io.Reader
	// Out and Err should strip or downsample ANSI colour to suit their
	// destination (see colorprofile.NewWriter).
	Out       io.Writer
	Err       io.Writer
	Clipboard Clipboard
	Now       func() time.Time

	// OutIsTerminal is true when stdout is a terminal; false means output is
	// piped and only plain values should be printed.
	OutIsTerminal bool
	// Interactive is true when stdin and stdout are terminals, so prompts,
	// forms and the watch view can be shown. TermIn/TermOut are then the raw
	// terminal files those programs run on.
	Interactive bool
	TermIn      *os.File
	TermOut     *os.File
	// DarkBackground selects the colour palette.
	DarkBackground bool
}

func (a *App) styles() ui.Styles { return ui.NewStyles(a.DarkBackground) }

// NewRootCommand builds the totp command tree.
func NewRootCommand(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "totp",
		Short: "Manage TOTP codes for development and test accounts",
		Long: `totp stores TOTP secrets in your OS keychain (Windows Credential Manager)
and generates codes on demand, so you don't need your phone to log in to
dev and staging accounts.`,
		Example: `  # Register from an otpauth:// URL read from stdin
  totp add github --url -

  # Register interactively
  totp add

  # Show all codes, or copy one
  totp list
  totp get github --copy`,
		SilenceUsage: true,
	}
	root.AddCommand(
		newAddCommand(app),
		newListCommand(app),
		newGetCommand(app),
		newRemoveCommand(app),
		newWatchCommand(app),
	)
	return root
}

// completeNames offers account names for shell completion.
func completeNames(app *App) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		accts, err := app.Store.List()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		names := make([]string, 0, len(accts))
		for _, a := range accts {
			names = append(names, a.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	}
}
