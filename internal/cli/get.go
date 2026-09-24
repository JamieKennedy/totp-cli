package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/JamieKennedy/totp-cli/internal/otp"
	"github.com/JamieKennedy/totp-cli/internal/store"
	"github.com/JamieKennedy/totp-cli/internal/ui"
)

func newGetCommand(app *App) *cobra.Command {
	var copyCode bool
	var clearAfter time.Duration
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Print the current code for a TOTP",
		Long: `Print the current code for a TOTP. The name can be a unique prefix and is
case-insensitive. When output is piped, only the code is printed.`,
		Example: `  totp get github
  totp get git --copy
  totp get github --copy --clear-after 30s`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeNames(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			acct, err := app.Store.Find(args[0])
			if err != nil {
				return err
			}
			code, err := app.Store.Code(acct, app.Now())
			if err != nil {
				return err
			}

			if !app.OutIsTerminal {
				fmt.Fprintln(app.Out, code.Value)
			}
			if copyCode {
				if err := app.Clipboard.WriteAll(code.Value); err != nil {
					return fmt.Errorf("copying to clipboard: %w", err)
				}
			}
			if app.OutIsTerminal {
				printCode(app, acct, code, copyCode)
			}
			if copyCode && clearAfter > 0 {
				return clearClipboardLater(cmd.Context(), app, code.Value, clearAfter)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&copyCode, "copy", "c", false, "copy the code to the clipboard")
	cmd.Flags().DurationVar(&clearAfter, "clear-after", 0, "with --copy, clear the clipboard after this long (e.g. 30s)")
	return cmd
}

func printCode(app *App, acct store.Account, code otp.Code, copied bool) {
	s := app.styles()
	label := s.Name.Render(acct.Name)
	if d := describe(acct.Issuer, acct.Account); d != "" {
		label += " " + s.Muted.Render(d)
	}
	fmt.Fprintln(app.Out, label)
	line := "  " + s.Code.Render(ui.FormatCode(code.Value)) + "  " + s.Countdown(code, 16)
	if copied {
		line += "  " + s.Success.Render("✓ copied")
	}
	fmt.Fprintln(app.Out, line)
}

// clearClipboardLater waits, then clears the clipboard if it still holds code.
func clearClipboardLater(ctx context.Context, app *App, code string, after time.Duration) error {
	s := app.styles()
	fmt.Fprintln(app.Err, s.Muted.Render(fmt.Sprintf("Clipboard will be cleared in %s (Ctrl+C to keep it).", after)))
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(after):
	}
	if current, err := app.Clipboard.ReadAll(); err == nil && current == code {
		if err := app.Clipboard.WriteAll(""); err != nil {
			return fmt.Errorf("clearing clipboard: %w", err)
		}
		fmt.Fprintln(app.Err, s.Muted.Render("Clipboard cleared."))
	}
	return nil
}
