# totp-cli

A small command-line tool for managing the TOTP (2FA) codes you use during development. Register the dev and staging accounts you log in to, then get their codes from the terminal instead of your phone.

- Register a TOTP from an `otpauth://` URL or from its individual settings
- List every registered TOTP with its current code and a countdown
- Get one code, and optionally copy it to the clipboard
- Secrets are stored in the **Windows Credential Manager**, not in a plain file

> **Scope:** totp-cli is meant for development and test accounts. It is not a replacement for a hardened authenticator on production or personal accounts.

## Install (Windows)

1. Download `totp-cli_<version>_windows_amd64.zip` (or `arm64`) from the [latest release](https://github.com/JamieKennedy/totp-cli/releases/latest).
2. Check it against `checksums.txt`:
   ```powershell
   (Get-FileHash .\totp-cli_*_windows_amd64.zip -Algorithm SHA256).Hash
   ```
   You can also check the build provenance with the [GitHub CLI](https://cli.github.com/):
   ```powershell
   gh attestation verify .\totp-cli_<version>_windows_amd64.zip --repo JamieKennedy/totp-cli
   ```
3. Extract `totp.exe` to a folder on your `PATH`. For example:
   ```powershell
   $dir = "$env:LOCALAPPDATA\Programs\totp"
   New-Item -ItemType Directory -Force $dir | Out-Null
   Expand-Archive .\totp-cli_*_windows_amd64.zip -DestinationPath $dir -Force
   [Environment]::SetEnvironmentVariable("Path", "$([Environment]::GetEnvironmentVariable('Path','User'));$dir", "User")
   ```
   Open a new terminal and run `totp --version`.

The binaries aren't code-signed yet, so Windows SmartScreen may warn the first time you run `totp.exe`.

To build from source instead (requires Go): `go install github.com/JamieKennedy/totp-cli/cmd/totp@latest`.

## Usage

### Register a TOTP

```powershell
# Interactive: prompts for the secret (hidden) or an otpauth:// URL, then the settings and a name
totp add

# From an otpauth:// URL on the clipboard, read through stdin so it stays out of your shell history
Get-Clipboard | totp add github --url -

# From individual settings, with the secret read from stdin
Get-Clipboard | totp add staging --secret - --issuer ACME --digits 8 --period 60 --algorithm SHA256
```

If you don't give a name, one is suggested from the issuer or account label. Secrets passed directly as `--url <value>` or `--secret <value>` work, but they print a warning because the value ends up in your shell history.

### List TOTPs

```powershell
totp list             # table of names, issuers, current codes and time remaining
totp list --no-codes  # settings only
totp list --json      # machine-readable
```

### Get a code

```powershell
totp get github                          # names are case-insensitive, and a unique prefix works too
totp get git -c                          # also copy the code to the clipboard
totp get github -c --clear-after 30s     # clear the clipboard again after 30 seconds
totp get github | clip                   # piped output is just the code
```

### Watch codes live

```powershell
totp watch            # all TOTPs; ↑/↓ to select, Enter to copy, q to quit
totp watch github aws # only these
```

### Remove a TOTP

```powershell
totp rm github        # asks for confirmation
totp rm github --yes
```

Run `totp <command> --help` for all flags. Shell completions are available with `totp completion powershell`.

## How secrets are stored

| What | Where |
|---|---|
| TOTP secrets | Windows Credential Manager, as generic credentials named `totp-cli:<id>`. Windows protects them with DPAPI, tied to your Windows login. |
| Names and settings (issuer, digits, period, algorithm) | `%APPDATA%\totp-cli\accounts.json`. This file never contains secrets. |

Set `TOTP_CLI_HOME` to use a different directory for `accounts.json`.

Other safeguards:
- Secrets are never printed or logged, and the tool makes no network calls.
- Secrets and URLs are validated before anything is saved, so a bad paste can't leave a half-registered TOTP behind.
- `--clear-after` only clears the clipboard if it still contains the code it copied.

To see the stored credentials: `cmdkey /list | findstr totp-cli`.

## Development

```powershell
go test ./...
go run ./cmd/totp list
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

| Path | Contents |
|---|---|
| `cmd/totp` | Entry point |
| `internal/cli` | Commands (Cobra + [Fang](https://github.com/charmbracelet/fang), [Huh](https://github.com/charmbracelet/huh) forms, [Bubble Tea](https://github.com/charmbracelet/bubbletea) watch view) |
| `internal/otp` | `otpauth://` parsing and code generation ([pquerna/otp](https://github.com/pquerna/otp)) |
| `internal/store` | Account metadata file and keychain secrets ([zalando/go-keyring](https://github.com/zalando/go-keyring)) |
| `internal/ui` | [Lip Gloss](https://github.com/charmbracelet/lipgloss) styles |

## Releasing

Releases are built by [GoReleaser](https://goreleaser.com) in GitHub Actions. To publish one, push a version tag:

```powershell
git tag v0.1.0
git push origin v0.1.0
```

The release workflow then:
- builds `windows/amd64` and `windows/arm64` zips
- publishes them to GitHub Releases with `checksums.txt` and a changelog
- attests their build provenance

To test packaging locally: `go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean`.
