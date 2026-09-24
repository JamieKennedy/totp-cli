// Package store persists TOTP accounts. Account metadata lives in a JSON file;
// secrets live in a SecretStore (the OS keychain in production).
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/JamieKennedy/totp-cli/internal/otp"
)

const (
	// KeyringService is the service name used for keychain entries.
	KeyringService = "totp-cli"
	// HomeEnv overrides the directory holding accounts.json.
	HomeEnv = "TOTP_CLI_HOME"

	fileName      = "accounts.json"
	schemaVersion = 1
	maxNameLen    = 64
)

// Account is the non-secret metadata for a registered TOTP.
type Account struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Issuer    string    `json:"issuer,omitempty"`
	Account   string    `json:"account,omitempty"`
	Algorithm string    `json:"algorithm"`
	Digits    int       `json:"digits"`
	Period    int       `json:"period"`
	CreatedAt time.Time `json:"created_at"`
}

// Params returns the code-generation parameters for the account.
func (a Account) Params() otp.Params {
	return otp.Params{Issuer: a.Issuer, Account: a.Account, Algorithm: a.Algorithm, Digits: a.Digits, Period: a.Period}
}

type file struct {
	Version  int       `json:"version"`
	Accounts []Account `json:"accounts"`
}

// ErrNotFound is returned when no account matches a name.
var ErrNotFound = errors.New("account not found")

// AmbiguousError is returned when a query matches more than one account.
type AmbiguousError struct {
	Query      string
	Candidates []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%q matches more than one account: %s", e.Query, strings.Join(e.Candidates, ", "))
}

// Store manages accounts.
type Store struct {
	path    string
	secrets SecretStore
}

// New returns a Store that keeps metadata in dir.
func New(dir string, secrets SecretStore) *Store {
	return &Store{path: filepath.Join(dir, fileName), secrets: secrets}
}

// DefaultDir returns the metadata directory: $TOTP_CLI_HOME, or
// %APPDATA%\totp-cli on Windows (the OS config dir elsewhere).
func DefaultDir() (string, error) {
	if dir := os.Getenv(HomeEnv); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "totp-cli"), nil
}

// Path returns the metadata file path.
func (s *Store) Path() string { return s.path }

// List returns all accounts sorted by name.
func (s *Store) List() ([]Account, error) {
	f, err := s.load()
	if err != nil {
		return nil, err
	}
	sort.Slice(f.Accounts, func(i, j int) bool {
		return strings.ToLower(f.Accounts[i].Name) < strings.ToLower(f.Accounts[j].Name)
	})
	return f.Accounts, nil
}

// Add registers a new account. The secret is written to the SecretStore and
// never to the metadata file.
func (s *Store) Add(name string, key otp.Key) (Account, error) {
	name = strings.TrimSpace(name)
	if err := ValidateName(name); err != nil {
		return Account{}, err
	}
	if err := key.Validate(); err != nil {
		return Account{}, err
	}
	f, err := s.load()
	if err != nil {
		return Account{}, err
	}
	for _, a := range f.Accounts {
		if strings.EqualFold(a.Name, name) {
			return Account{}, fmt.Errorf("an account named %q already exists", a.Name)
		}
	}

	id, err := newID()
	if err != nil {
		return Account{}, err
	}
	acct := Account{
		ID:        id,
		Name:      name,
		Issuer:    key.Issuer,
		Account:   key.Account,
		Algorithm: key.Algorithm,
		Digits:    key.Digits,
		Period:    key.Period,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	if err := s.secrets.Set(id, key.Secret); err != nil {
		return Account{}, fmt.Errorf("saving secret to keychain: %w", err)
	}
	f.Accounts = append(f.Accounts, acct)
	if err := s.save(f); err != nil {
		_ = s.secrets.Delete(id)
		return Account{}, err
	}
	return acct, nil
}

// Find resolves a name to an account: an exact match first, then a
// case-insensitive match, then a unique case-insensitive prefix.
func (s *Store) Find(query string) (Account, error) {
	accts, err := s.List()
	if err != nil {
		return Account{}, err
	}
	return Match(accts, query)
}

// Match resolves query against accts using the rules described on Find.
func Match(accts []Account, query string) (Account, error) {
	query = strings.TrimSpace(query)
	for _, a := range accts {
		if a.Name == query {
			return a, nil
		}
	}
	var fold, prefix []Account
	lq := strings.ToLower(query)
	for _, a := range accts {
		ln := strings.ToLower(a.Name)
		if ln == lq {
			fold = append(fold, a)
		} else if strings.HasPrefix(ln, lq) {
			prefix = append(prefix, a)
		}
	}
	for _, set := range [][]Account{fold, prefix} {
		switch len(set) {
		case 0:
			continue
		case 1:
			return set[0], nil
		default:
			names := make([]string, len(set))
			for i, a := range set {
				names[i] = a.Name
			}
			return Account{}, &AmbiguousError{Query: query, Candidates: names}
		}
	}
	return Account{}, fmt.Errorf("%w: %q", ErrNotFound, query)
}

// Secret returns the secret for an account.
func (s *Store) Secret(a Account) (string, error) {
	secret, err := s.secrets.Get(a.ID)
	if errors.Is(err, ErrSecretNotFound) {
		return "", fmt.Errorf("the keychain has no secret for %q; remove and re-add it", a.Name)
	}
	if err != nil {
		return "", fmt.Errorf("reading secret from keychain: %w", err)
	}
	return secret, nil
}

// Code generates the current code for an account.
func (s *Store) Code(a Account, now time.Time) (otp.Code, error) {
	secret, err := s.Secret(a)
	if err != nil {
		return otp.Code{}, err
	}
	return otp.Generate(secret, a.Params(), now)
}

// Remove deletes an account's metadata and its keychain secret.
func (s *Store) Remove(a Account) error {
	f, err := s.load()
	if err != nil {
		return err
	}
	kept := f.Accounts[:0]
	found := false
	for _, x := range f.Accounts {
		if x.ID == a.ID {
			found = true
			continue
		}
		kept = append(kept, x)
	}
	if !found {
		return fmt.Errorf("%w: %q", ErrNotFound, a.Name)
	}
	f.Accounts = kept
	if err := s.save(f); err != nil {
		return err
	}
	if err := s.secrets.Delete(a.ID); err != nil && !errors.Is(err, ErrSecretNotFound) {
		return fmt.Errorf("removed %q but could not delete its keychain entry: %w", a.Name, err)
	}
	return nil
}

// ValidateName checks that name is usable as an account name.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > maxNameLen {
		return fmt.Errorf("name must be at most %d characters", maxNameLen)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("name must not contain control characters")
		}
	}
	return nil
}

// SuggestName derives an account name from a key's issuer or account label.
func SuggestName(p otp.Params) string {
	src := p.Issuer
	if src == "" {
		src = p.Account
	}
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(src)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '@' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	name := strings.TrimRight(b.String(), "-")
	if len(name) > maxNameLen {
		name = name[:maxNameLen]
	}
	return name
}

func (s *Store) load() (*file, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return &file{Version: schemaVersion}, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.path, err)
	}
	if f.Version > schemaVersion {
		return nil, fmt.Errorf("%s was written by a newer version of totp-cli", s.path)
	}
	return &f, nil
}

// save writes the metadata atomically: write a temp file, then rename over.
func (s *Store) save(f *file) error {
	f.Version = schemaVersion
	if f.Accounts == nil {
		f.Accounts = []Account{}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// CreateTemp uses mode 0600; on Windows the user profile's ACLs apply.
	tmp, err := os.CreateTemp(dir, fileName+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
