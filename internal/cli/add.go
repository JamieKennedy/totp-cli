package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/JamieKennedy/totp-cli/internal/otp"
	"github.com/JamieKennedy/totp-cli/internal/store"
	"github.com/JamieKennedy/totp-cli/internal/ui"
)

type addOptions struct {
	url       string
	secret    string
	issuer    string
	account   string
	algorithm string
	digits    int
	period    int
}

func newAddCommand(app *App) *cobra.Command {
	var o addOptions
	cmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Register a TOTP from an otpauth:// URL or from settings",
		Long: `Register a TOTP. The secret is stored in your OS keychain; only the name and
settings are written to disk.

Provide the secret in one of these ways (safest first):
  - run with no --url/--secret to be prompted (input is hidden)
  - pipe it in: --url - or --secret - reads from stdin
  - pass it as a value (it will be saved in your shell history)`,
		Example: `  totp add
  Get-Clipboard | totp add github --url -
  totp add staging --secret - --issuer ACME --digits 8 --period 60`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runAdd(cmd, app, name, &o)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&o.url, "url", "u", "", `otpauth://totp/... URL, or "-" to read it from stdin`)
	f.StringVarP(&o.secret, "secret", "s", "", `base32 secret, or "-" to read it from stdin`)
	f.StringVar(&o.issuer, "issuer", "", "issuer, e.g. GitHub")
	f.StringVar(&o.account, "account", "", "account label, e.g. you@example.com")
	f.StringVar(&o.algorithm, "algorithm", otp.DefaultAlgorithm, "hash algorithm: SHA1, SHA256 or SHA512")
	f.IntVar(&o.digits, "digits", otp.DefaultDigits, "code length (6-8)")
	f.IntVar(&o.period, "period", otp.DefaultPeriod, "seconds each code is valid for")
	cmd.MarkFlagsMutuallyExclusive("url", "secret")
	for _, name := range []string{"algorithm", "digits", "period"} {
		cmd.MarkFlagsMutuallyExclusive("url", name)
	}
	_ = cmd.RegisterFlagCompletionFunc("algorithm", cobra.FixedCompletions(
		[]string{otp.SHA1, otp.SHA256, otp.SHA512}, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

func runAdd(cmd *cobra.Command, app *App, name string, o *addOptions) error {
	errS := app.styles()
	var key otp.Key

	switch {
	case o.url != "":
		raw, err := valueOrStdin(app, o.url, "URL")
		if err != nil {
			return err
		}
		if key, err = otp.ParseURL(raw); err != nil {
			return err
		}
		if o.issuer != "" {
			key.Issuer = o.issuer
		}
		if o.account != "" {
			key.Account = o.account
		}

	case o.secret != "":
		raw, err := valueOrStdin(app, o.secret, "secret")
		if err != nil {
			return err
		}
		if key, err = keyFromSettings(raw, o); err != nil {
			return err
		}

	case app.Interactive:
		var err error
		if key, err = promptKey(cmd, app, o); err != nil {
			return err
		}

	default:
		return errors.New("no secret given: use --url or --secret (\"-\" reads from stdin), or run in a terminal to be prompted")
	}

	if o.url != "" && o.url != "-" || o.secret != "" && o.secret != "-" {
		fmt.Fprintln(app.Err, errS.Warn.Render("! The secret was passed as an argument and may be saved in your shell history. Prefer \"-\" to read from stdin."))
	}

	if name == "" {
		var err error
		if name, err = chooseName(cmd, app, key.Params); err != nil {
			return err
		}
	}

	// Prove the secret works before saving it.
	code, err := otp.Generate(key.Secret, key.Params, app.Now())
	if err != nil {
		return err
	}
	acct, err := app.Store.Add(name, key)
	if err != nil {
		return err
	}

	s := app.styles()
	if !app.OutIsTerminal {
		fmt.Fprintf(app.Out, "added %s\n", acct.Name)
		return nil
	}
	fmt.Fprintf(app.Out, "%s Added %s %s\n", s.Success.Render("✓"), s.Name.Render(acct.Name), s.Muted.Render(describe(acct.Issuer, acct.Account)))
	fmt.Fprintf(app.Out, "  Current code: %s  %s\n", s.Code.Render(ui.FormatCode(code.Value)), s.Countdown(code, 12))
	return nil
}

// valueOrStdin returns v, or a line read from stdin when v is "-".
func valueOrStdin(app *App, v, what string) (string, error) {
	if v != "-" {
		return v, nil
	}
	line, err := bufio.NewReader(app.In).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading %s from stdin: %w", what, err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", fmt.Errorf("no %s on stdin", what)
	}
	return line, nil
}

func keyFromSettings(secret string, o *addOptions) (otp.Key, error) {
	s, err := otp.NormalizeSecret(secret)
	if err != nil {
		return otp.Key{}, err
	}
	alg, err := otp.ParseAlgorithm(o.algorithm)
	if err != nil {
		return otp.Key{}, err
	}
	p := otp.Params{Issuer: o.issuer, Account: o.account, Algorithm: alg, Digits: o.digits, Period: o.period}
	if err := p.Validate(); err != nil {
		return otp.Key{}, err
	}
	return otp.Key{Params: p, Secret: s}, nil
}

// promptKey asks for the secret (or a URL) with hidden input, then for any
// settings not implied by a URL. Flag values are used as defaults.
func promptKey(cmd *cobra.Command, app *App, o *addOptions) (otp.Key, error) {
	var raw string
	err := newForm(app, huh.NewGroup(
		huh.NewInput().
			Title("Secret or otpauth:// URL").
			Description("Input is hidden. Paste the base32 setup key or the full otpauth:// link.").
			EchoMode(huh.EchoModePassword).
			Value(&raw).
			Validate(func(s string) error {
				if otp.IsURL(s) {
					_, err := otp.ParseURL(s)
					return err
				}
				_, err := otp.NormalizeSecret(s)
				return err
			}),
	)).RunWithContext(cmd.Context())
	if err != nil {
		return otp.Key{}, err
	}
	if otp.IsURL(raw) {
		return otp.ParseURL(raw)
	}

	alg, err := otp.ParseAlgorithm(o.algorithm)
	if err != nil {
		return otp.Key{}, err
	}
	issuer, account, digits, period := o.issuer, o.account, o.digits, strconv.Itoa(o.period)
	err = newForm(app, huh.NewGroup(
		huh.NewInput().Title("Issuer").Placeholder("e.g. GitHub").Value(&issuer),
		huh.NewInput().Title("Account").Placeholder("e.g. you@example.com").Value(&account),
		huh.NewSelect[string]().Title("Algorithm").
			Options(huh.NewOptions(otp.SHA1, otp.SHA256, otp.SHA512)...).Value(&alg),
		huh.NewSelect[int]().Title("Digits").
			Options(huh.NewOptions(6, 7, 8)...).Value(&digits),
		huh.NewInput().Title("Period (seconds)").Value(&period).
			Validate(func(s string) error {
				n, err := strconv.Atoi(strings.TrimSpace(s))
				if err != nil || n < 1 || n > 3600 {
					return errors.New("enter a number of seconds between 1 and 3600")
				}
				return nil
			}),
	)).RunWithContext(cmd.Context())
	if err != nil {
		return otp.Key{}, err
	}
	n, _ := strconv.Atoi(strings.TrimSpace(period))
	return keyFromSettings(raw, &addOptions{
		issuer: strings.TrimSpace(issuer), account: strings.TrimSpace(account),
		algorithm: alg, digits: digits, period: n,
	})
}

// chooseName suggests a name from the issuer/account and, when interactive,
// lets the user edit it.
func chooseName(cmd *cobra.Command, app *App, p otp.Params) (string, error) {
	name := store.SuggestName(p)
	if !app.Interactive {
		if name == "" {
			return "", errors.New("a name is required: run totp add <name>")
		}
		return name, nil
	}
	accts, err := app.Store.List()
	if err != nil {
		return "", err
	}
	err = newForm(app, huh.NewGroup(
		huh.NewInput().Title("Name").
			Description("Used with totp get <name>.").
			Value(&name).
			Validate(func(s string) error {
				s = strings.TrimSpace(s)
				if err := store.ValidateName(s); err != nil {
					return err
				}
				for _, a := range accts {
					if strings.EqualFold(a.Name, s) {
						return fmt.Errorf("%q already exists", a.Name)
					}
				}
				return nil
			}),
	)).RunWithContext(cmd.Context())
	return strings.TrimSpace(name), err
}

func newForm(app *App, groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).
		WithTheme(huh.ThemeFunc(huh.ThemeCharm)).
		WithInput(app.TermIn).
		WithOutput(app.TermOut).
		WithShowHelp(true)
}

func describe(issuer, account string) string {
	switch {
	case issuer != "" && account != "":
		return issuer + " · " + account
	default:
		return issuer + account
	}
}
