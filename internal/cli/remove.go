package cli

import (
	"errors"
	"fmt"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
)

func newRemoveCommand(app *App) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:               "remove <name>",
		Aliases:           []string{"rm"},
		Short:             "Remove a TOTP and delete its secret from the keychain",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeNames(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			acct, err := app.Store.Find(args[0])
			if err != nil {
				return err
			}
			if !yes {
				if !app.Interactive {
					return errors.New("refusing to remove without confirmation: pass --yes")
				}
				confirmed := false
				err := newForm(app, huh.NewGroup(
					huh.NewConfirm().
						Title(fmt.Sprintf("Remove %q?", acct.Name)).
						Description("The secret is deleted from the keychain and cannot be recovered.").
						Affirmative("Remove").
						Negative("Cancel").
						Value(&confirmed),
				)).RunWithContext(cmd.Context())
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintln(app.Err, app.styles().Muted.Render("Cancelled."))
					return nil
				}
			}
			if err := app.Store.Remove(acct); err != nil {
				return err
			}
			s := app.styles()
			if app.OutIsTerminal {
				fmt.Fprintf(app.Out, "%s Removed %s\n", s.Success.Render("✓"), s.Name.Render(acct.Name))
			} else {
				fmt.Fprintf(app.Out, "removed %s\n", acct.Name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}
