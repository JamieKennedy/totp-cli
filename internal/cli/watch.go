package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/JamieKennedy/totp-cli/internal/otp"
	"github.com/JamieKennedy/totp-cli/internal/store"
	"github.com/JamieKennedy/totp-cli/internal/ui"
)

func newWatchCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:               "watch [name...]",
		Short:             "Show live-updating codes; press Enter to copy",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeNames(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !app.Interactive {
				return errors.New("watch needs an interactive terminal; use totp list instead")
			}
			all, err := app.Store.List()
			if err != nil {
				return err
			}
			accts := all
			if len(args) > 0 {
				accts = nil
				for _, q := range args {
					a, err := store.Match(all, q)
					if err != nil {
						return err
					}
					accts = append(accts, a)
				}
			}
			if len(accts) == 0 {
				return errors.New("no TOTPs registered yet; add one with: totp add")
			}

			m := &watchModel{app: app, styles: app.styles()}
			for _, a := range accts {
				row := watchRow{acct: a}
				row.secret, row.err = app.Store.Secret(a)
				m.rows = append(m.rows, row)
			}
			_, err = tea.NewProgram(m,
				tea.WithContext(cmd.Context()),
				tea.WithInput(app.TermIn),
				tea.WithOutput(app.TermOut),
			).Run()
			if errors.Is(err, tea.ErrProgramKilled) {
				return nil
			}
			return err
		},
	}
}

type watchRow struct {
	acct   store.Account
	secret string
	err    error
}

type tickMsg time.Time

type watchModel struct {
	app      *App
	styles   ui.Styles
	rows     []watchRow
	cursor   int
	status   string
	statusAt time.Time
}

func tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *watchModel) Init() tea.Cmd { return tick() }

func (m *watchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		return m, tick()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.cursor = (m.cursor - 1 + len(m.rows)) % len(m.rows)
		case "down", "j":
			m.cursor = (m.cursor + 1) % len(m.rows)
		case "enter", "c", " ":
			m.copySelected()
		}
	}
	return m, nil
}

func (m *watchModel) copySelected() {
	row := m.rows[m.cursor]
	m.statusAt = m.app.Now()
	if row.err != nil {
		m.status = m.styles.Error.Render("✗ " + row.err.Error())
		return
	}
	code, err := otp.Generate(row.secret, row.acct.Params(), m.app.Now())
	if err == nil {
		err = m.app.Clipboard.WriteAll(code.Value)
	}
	if err != nil {
		m.status = m.styles.Error.Render("✗ " + err.Error())
		return
	}
	m.status = m.styles.Success.Render(fmt.Sprintf("✓ Copied %s code", row.acct.Name))
}

func (m *watchModel) View() tea.View {
	s := m.styles
	now := m.app.Now()

	nameW := 0
	for _, r := range m.rows {
		nameW = max(nameW, len([]rune(r.acct.Name)))
	}

	var b strings.Builder
	b.WriteString("\n  " + s.Title.Render("totp") + s.Muted.Render(" · live codes") + "\n\n")
	for i, r := range m.rows {
		pointer, name := "  ", s.Name.Render(pad(r.acct.Name, nameW))
		if i == m.cursor {
			pointer, name = s.Cursor.Render("▸ "), s.Cursor.Render(pad(r.acct.Name, nameW))
		}
		line := "  " + pointer + name + "  "
		if r.err != nil {
			line += s.Error.Render("unavailable")
		} else if code, err := otp.Generate(r.secret, r.acct.Params(), now); err != nil {
			line += s.Error.Render("invalid secret")
		} else {
			line += s.Code.Render(pad(ui.FormatCode(code.Value), 9)) + "  " + s.Countdown(code, 20)
		}
		if d := describe(r.acct.Issuer, r.acct.Account); d != "" {
			line += "  " + s.Muted.Render(d)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	if m.status != "" && now.Sub(m.statusAt) < 3*time.Second {
		b.WriteString("  " + m.status + "\n")
	} else {
		b.WriteString("\n")
	}
	b.WriteString("  " + s.Muted.Render("↑/↓ select • enter copy • q quit") + "\n")
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func pad(s string, w int) string {
	if n := len([]rune(s)); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}
