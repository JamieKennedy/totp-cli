package ui

import "golang.org/x/sys/windows"

// EnableVirtualTerminal turns on ANSI escape processing for the console so
// colours render in cmd.exe and older PowerShell hosts as well as Windows
// Terminal. It is a no-op when output is redirected.
func EnableVirtualTerminal() {
	for _, h := range []windows.Handle{windows.Stdout, windows.Stderr} {
		var mode uint32
		if windows.GetConsoleMode(h, &mode) == nil {
			_ = windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
		}
	}
}
