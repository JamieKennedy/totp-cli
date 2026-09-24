package store

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JamieKennedy/totp-cli/internal/otp"
)

const testSecret = "JBSWY3DPEHPK3PXP"

func testKey() otp.Key {
	p := otp.DefaultParams()
	p.Issuer = "GitHub"
	p.Account = "dev@example.com"
	return otp.Key{Params: p, Secret: testSecret}
}

func newTestStore(t *testing.T) (*Store, *MemorySecrets) {
	t.Helper()
	secrets := NewMemorySecrets()
	return New(t.TempDir(), secrets), secrets
}

func TestAddListRemove(t *testing.T) {
	s, secrets := newTestStore(t)

	acct, err := s.Add("github", testKey())
	if err != nil {
		t.Fatal(err)
	}
	if acct.ID == "" || acct.Issuer != "GitHub" || acct.Digits != 6 {
		t.Errorf("unexpected account %+v", acct)
	}

	data, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), testSecret) {
		t.Fatal("metadata file must not contain the secret")
	}

	accts, err := s.List()
	if err != nil || len(accts) != 1 || accts[0].Name != "github" {
		t.Fatalf("List() = %+v, %v", accts, err)
	}

	code, err := s.Code(acct, time.Unix(59, 0))
	if err != nil || len(code.Value) != 6 {
		t.Fatalf("Code() = %+v, %v", code, err)
	}

	if err := s.Remove(acct); err != nil {
		t.Fatal(err)
	}
	if accts, _ := s.List(); len(accts) != 0 {
		t.Errorf("expected no accounts after remove, got %d", len(accts))
	}
	if secrets.Len() != 0 {
		t.Error("expected secret to be deleted")
	}
}

func TestAddDuplicateName(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.Add("github", testKey()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add("GitHub", testKey()); err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestAddInvalidName(t *testing.T) {
	s, secrets := newTestStore(t)
	for _, name := range []string{"", "  ", "bad\x00name", strings.Repeat("x", 65)} {
		if _, err := s.Add(name, testKey()); err == nil {
			t.Errorf("Add(%q) should fail", name)
		}
	}
	if secrets.Len() != 0 {
		t.Error("no secrets should be stored for rejected accounts")
	}
}

func TestMissingSecret(t *testing.T) {
	s, secrets := newTestStore(t)
	acct, _ := s.Add("github", testKey())
	_ = secrets.Delete(acct.ID)
	if _, err := s.Code(acct, time.Now()); err == nil || !strings.Contains(err.Error(), "re-add") {
		t.Errorf("expected helpful missing-secret error, got %v", err)
	}
	// Removing still works when the keychain entry is already gone.
	if err := s.Remove(acct); err != nil {
		t.Errorf("Remove() = %v", err)
	}
}

func TestMatch(t *testing.T) {
	accts := []Account{{Name: "github"}, {Name: "GitLab"}, {Name: "aws-dev"}, {Name: "aws-prod"}, {Name: "Stripe"}}
	cases := []struct {
		query, want, err string
	}{
		{"github", "github", ""},
		{"stripe", "Stripe", ""},
		{"gith", "github", ""},
		{"aws-p", "aws-prod", ""},
		{"git", "", "matches more than one"},
		{"aws", "", "matches more than one"},
		{"azure", "", "not found"},
	}
	for _, c := range cases {
		got, err := Match(accts, c.query)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("Match(%q) error = %v, want %q", c.query, err, c.err)
			}
			continue
		}
		if err != nil || got.Name != c.want {
			t.Errorf("Match(%q) = %q, %v; want %q", c.query, got.Name, err, c.want)
		}
	}

	var amb *AmbiguousError
	if _, err := Match(accts, "aws"); !errors.As(err, &amb) || len(amb.Candidates) != 2 {
		t.Errorf("expected AmbiguousError with 2 candidates, got %v", err)
	}
}

func TestSuggestName(t *testing.T) {
	cases := map[otp.Params]string{
		{Issuer: "ACME Co."}:             "acme-co.",
		{Account: "Alice@Example.com"}:   "alice@example.com",
		{Issuer: "  My  Service (dev) "}: "my-service-dev",
		{}:                               "",
	}
	for p, want := range cases {
		if got := SuggestName(p); got != want {
			t.Errorf("SuggestName(%+v) = %q, want %q", p, got, want)
		}
	}
}
