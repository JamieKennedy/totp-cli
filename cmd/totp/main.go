// Command totp manages TOTP codes for development and test accounts.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"charm.land/fang/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/colorprofile"

	"github.com/JamieKennedy/totp-cli/internal/cli"
	"github.com/JamieKennedy/totp-cli/internal/store"
	"github.com/JamieKennedy/totp-cli/internal/ui"
)

// Set by GoReleaser via -ldflags.
var (
	version = ""
	commit  = ""
)

type systemClipboard struct{}

func (systemClipboard) ReadAll() (string, error)   { return clipboard.ReadAll() }
func (systemClipboard) WriteAll(text string) error { return clipboard.WriteAll(text) }

func main() {
	ui.EnableVirtualTerminal()

	dir, err := store.DefaultDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "totp: cannot locate config directory:", err)
		os.Exit(1)
	}

	outTTY := ui.IsTerminal(os.Stdout)
	interactive := outTTY && ui.IsTerminal(os.Stdin)
	app := &cli.App{
		Store:          store.New(dir, store.KeyringSecrets{Service: store.KeyringService}),
		In:             os.Stdin,
		Out:            colorprofile.NewWriter(os.Stdout, os.Environ()),
		Err:            colorprofile.NewWriter(os.Stderr, os.Environ()),
		Clipboard:      systemClipboard{},
		Now:            time.Now,
		OutIsTerminal:  outTTY,
		Interactive:    interactive,
		TermIn:         os.Stdin,
		TermOut:        os.Stdout,
		DarkBackground: !interactive || ui.HasDarkBackground(os.Stdin, os.Stdout),
	}

	opts := []fang.Option{fang.WithoutManpage()}
	if version != "" {
		opts = append(opts, fang.WithVersion(version), fang.WithCommit(commit))
	}
	if err := fang.Execute(context.Background(), cli.NewRootCommand(app), opts...); err != nil {
		os.Exit(1)
	}
}
