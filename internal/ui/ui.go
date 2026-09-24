// Package ui holds shared terminal styling for totp-cli.
package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"

	"github.com/JamieKennedy/totp-cli/internal/otp"
)

// Styles are the lipgloss styles used across commands. Colour is stripped or
// downsampled by the colorprofile writer the output is wrapped in, so these
// can be used unconditionally.
type Styles struct {
	Title   lipgloss.Style
	Name    lipgloss.Style
	Code    lipgloss.Style
	Muted   lipgloss.Style
	Success lipgloss.Style
	Warn    lipgloss.Style
	Error   lipgloss.Style
	Header  lipgloss.Style
	Cell    lipgloss.Style
	Border  lipgloss.Style
	Cursor  lipgloss.Style
}

// NewStyles returns styles tuned for a dark or light terminal background.
func NewStyles(dark bool) Styles {
	c := lipgloss.LightDark(dark)
	accent := c(lipgloss.Color("#5A3FD0"), lipgloss.Color("#A08CFF"))
	return Styles{
		Title:   lipgloss.NewStyle().Bold(true).Foreground(accent),
		Name:    lipgloss.NewStyle().Bold(true),
		Code:    lipgloss.NewStyle().Bold(true).Foreground(c(lipgloss.Color("#0B6E4F"), lipgloss.Color("#5EEAD4"))),
		Muted:   lipgloss.NewStyle().Foreground(c(lipgloss.Color("#6B7280"), lipgloss.Color("#9CA3AF"))),
		Success: lipgloss.NewStyle().Foreground(c(lipgloss.Color("#15803D"), lipgloss.Color("#4ADE80"))),
		Warn:    lipgloss.NewStyle().Foreground(c(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24"))),
		Error:   lipgloss.NewStyle().Foreground(c(lipgloss.Color("#B91C1C"), lipgloss.Color("#F87171"))),
		Header:  lipgloss.NewStyle().Bold(true).Foreground(accent).Padding(0, 1),
		Cell:    lipgloss.NewStyle().Padding(0, 1),
		Border:  lipgloss.NewStyle().Foreground(c(lipgloss.Color("#D1D5DB"), lipgloss.Color("#4B5563"))),
		Cursor:  lipgloss.NewStyle().Bold(true).Foreground(accent),
	}
}

// IsTerminal reports whether f is an interactive terminal.
func IsTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// HasDarkBackground queries the terminal background, assuming dark when the
// terminal doesn't answer.
func HasDarkBackground(in, out *os.File) bool {
	return lipgloss.HasDarkBackground(in, out)
}

// FormatCode groups digits for readability: "123 456", "1234 5678".
func FormatCode(code string) string {
	if len(code) < 6 {
		return code
	}
	mid := len(code) / 2
	return code[:mid] + " " + code[mid:]
}

// Seconds returns the remaining validity of c in whole seconds, rounded up.
func Seconds(c otp.Code) int {
	return int((c.Remaining + time.Second - 1) / time.Second)
}

// Countdown renders a bar showing how much of the period remains, coloured
// green, then amber, then red as the code nears expiry.
func (s Styles) Countdown(c otp.Code, width int) string {
	if c.Period <= 0 || width <= 0 {
		return ""
	}
	frac := float64(c.Remaining) / float64(c.Period)
	filled := max(0, min(width, int(frac*float64(width)+0.5)))

	style := s.Success
	switch {
	case c.Remaining <= 5*time.Second:
		style = s.Error
	case frac <= 1.0/3:
		style = s.Warn
	}
	bar := style.Render(strings.Repeat("━", filled)) + s.Border.Render(strings.Repeat("━", width-filled))
	return bar + " " + style.Render(fmt.Sprintf("%2ds", Seconds(c)))
}
