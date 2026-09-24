package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"

	"github.com/JamieKennedy/totp-cli/internal/otp"
	"github.com/JamieKennedy/totp-cli/internal/store"
	"github.com/JamieKennedy/totp-cli/internal/ui"
)

type listEntry struct {
	Name      string `json:"name"`
	Issuer    string `json:"issuer,omitempty"`
	Account   string `json:"account,omitempty"`
	Algorithm string `json:"algorithm"`
	Digits    int    `json:"digits"`
	Period    int    `json:"period"`
	Code      string `json:"code,omitempty"`
	ExpiresIn *int   `json:"expires_in,omitempty"`
	Error     string `json:"error,omitempty"`

	code otp.Code
}

func newListCommand(app *App) *cobra.Command {
	var asJSON, noCodes bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered TOTPs and their current codes",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			accts, err := app.Store.List()
			if err != nil {
				return err
			}
			entries := buildEntries(app, accts, !noCodes)
			switch {
			case asJSON:
				enc := json.NewEncoder(app.Out)
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			case len(accts) == 0:
				s := app.styles()
				fmt.Fprintln(app.Err, s.Muted.Render("No TOTPs registered yet. Add one with: totp add"))
				return nil
			case !app.OutIsTerminal:
				return renderPlain(app, entries, !noCodes)
			default:
				renderTable(app, entries, !noCodes)
				return nil
			}
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	cmd.Flags().BoolVar(&noCodes, "no-codes", false, "show settings only, without generating codes")
	return cmd
}

func buildEntries(app *App, accts []store.Account, withCodes bool) []listEntry {
	now := app.Now()
	entries := make([]listEntry, 0, len(accts))
	for _, a := range accts {
		e := listEntry{Name: a.Name, Issuer: a.Issuer, Account: a.Account, Algorithm: a.Algorithm, Digits: a.Digits, Period: a.Period}
		if withCodes {
			if c, err := app.Store.Code(a, now); err != nil {
				e.Error = err.Error()
			} else {
				secs := ui.Seconds(c)
				e.Code, e.ExpiresIn, e.code = c.Value, &secs, c
			}
		}
		entries = append(entries, e)
	}
	return entries
}

func renderPlain(app *App, entries []listEntry, withCodes bool) error {
	w := tabwriter.NewWriter(app.Out, 0, 0, 2, ' ', 0)
	if withCodes {
		fmt.Fprintln(w, "NAME\tISSUER\tACCOUNT\tCODE\tEXPIRES")
	} else {
		fmt.Fprintln(w, "NAME\tISSUER\tACCOUNT\tALGORITHM\tDIGITS\tPERIOD")
	}
	for _, e := range entries {
		switch {
		case !withCodes:
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%ds\n", e.Name, dash(e.Issuer), dash(e.Account), e.Algorithm, e.Digits, e.Period)
		case e.Error != "":
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t-\n", e.Name, dash(e.Issuer), dash(e.Account), "error")
		default:
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%ds\n", e.Name, dash(e.Issuer), dash(e.Account), e.Code, *e.ExpiresIn)
		}
	}
	return w.Flush()
}

func renderTable(app *App, entries []listEntry, withCodes bool) {
	s := app.styles()

	headers := []string{"NAME", "ISSUER", "ACCOUNT", "CODE", "EXPIRES"}
	if !withCodes {
		headers = []string{"NAME", "ISSUER", "ACCOUNT", "ALGORITHM", "DIGITS", "PERIOD"}
	}
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		row := []string{s.Name.Render(e.Name), dash(e.Issuer), s.Muted.Render(dash(e.Account))}
		switch {
		case !withCodes:
			row = append(row, e.Algorithm, fmt.Sprint(e.Digits), fmt.Sprintf("%ds", e.Period))
		case e.Error != "":
			row = append(row, s.Error.Render("unavailable"), "")
		default:
			row = append(row, s.Code.Render(ui.FormatCode(e.Code)), s.Countdown(e.code, 10))
		}
		rows = append(rows, row)
	}

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(s.Border).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return s.Header
			}
			return s.Cell
		})
	fmt.Fprintln(app.Out, t.Render())

	errS := app.styles()
	for _, e := range entries {
		if e.Error != "" {
			fmt.Fprintln(app.Err, errS.Error.Render("✗ "+e.Error))
		}
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
