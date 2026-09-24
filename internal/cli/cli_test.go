package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"

	"github.com/JamieKennedy/totp-cli/internal/otp"
	"github.com/JamieKennedy/totp-cli/internal/store"
)

const (
	testURL    = "otpauth://totp/Test:dev@example.com?secret=JBSWY3DPEHPK3PXP&issuer=Test"
	testSecret = "JBSWY3DPEHPK3PXP"
)

var fixedNow = time.Unix(1_700_000_005, 0)

type fakeClipboard struct{ text string }

func (c *fakeClipboard) ReadAll() (string, error)   { return c.text, nil }
func (c *fakeClipboard) WriteAll(text string) error { c.text = text; return nil }

type harness struct {
	app       *App
	secrets   *store.MemorySecrets
	clipboard *fakeClipboard
	out, err  *bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		secrets:   store.NewMemorySecrets(),
		clipboard: &fakeClipboard{},
		out:       &bytes.Buffer{},
		err:       &bytes.Buffer{},
	}
	h.app = &App{
		Store:     store.New(t.TempDir(), h.secrets),
		Out:       &colorprofile.Writer{Forward: h.out, Profile: colorprofile.NoTTY},
		Err:       &colorprofile.Writer{Forward: h.err, Profile: colorprofile.NoTTY},
		Clipboard: h.clipboard,
		Now:       func() time.Time { return fixedNow },
	}
	return h
}

func (h *harness) run(t *testing.T, stdin string, args ...string) error {
	t.Helper()
	h.out.Reset()
	h.err.Reset()
	h.app.In = strings.NewReader(stdin)
	cmd := NewRootCommand(h.app)
	cmd.SetArgs(args)
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)
	return cmd.Execute()
}

func expectedCode(t *testing.T, p otp.Params) string {
	t.Helper()
	c, err := otp.Generate(testSecret, p, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return c.Value
}

func TestAddFromStdinURLAndGet(t *testing.T) {
	h := newHarness(t)
	if err := h.run(t, testURL+"\n", "add", "--url", "-"); err != nil {
		t.Fatal(err)
	}
	if got := h.out.String(); got != "added test\n" {
		t.Errorf("add output = %q", got)
	}
	if strings.Contains(h.err.String(), "shell history") {
		t.Error("reading from stdin should not warn about shell history")
	}

	if err := h.run(t, "", "get", "TE"); err != nil {
		t.Fatal(err)
	}
	want := expectedCode(t, otp.DefaultParams())
	if got := h.out.String(); got != want+"\n" {
		t.Errorf("get output = %q, want %q", got, want)
	}
}

func TestAddFromSettingsWarns(t *testing.T) {
	h := newHarness(t)
	if err := h.run(t, "", "add", "staging", "--secret", testSecret, "--digits", "8", "--period", "60", "--algorithm", "sha256"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.err.String(), "shell history") {
		t.Error("expected shell history warning")
	}
	accts, _ := h.app.Store.List()
	if len(accts) != 1 || accts[0].Digits != 8 || accts[0].Period != 60 || accts[0].Algorithm != otp.SHA256 {
		t.Errorf("unexpected accounts %+v", accts)
	}
}

func TestAddRejectsBadInput(t *testing.T) {
	cases := [][]string{
		{"add"},                                 // no secret, not interactive
		{"add", "--secret", "-"},                // no name derivable
		{"add", "x", "--secret", "not base32!"}, // invalid secret
		{"add", "x", "--url", testURL, "--digits", "8"},            // url + settings
		{"add", "x", "--url", testURL, "--secret", testSecret},     // url + secret
		{"add", "x", "--secret", testSecret, "--algorithm", "md5"}, // bad algorithm
	}
	for _, args := range cases {
		h := newHarness(t)
		if err := h.run(t, testSecret+"\n", args...); err == nil {
			t.Errorf("%v: expected error", args)
		}
		if h.secrets.Len() != 0 {
			t.Errorf("%v: no secret should be stored", args)
		}
	}
}

func TestListJSON(t *testing.T) {
	h := newHarness(t)
	_ = h.run(t, testURL, "add", "--url", "-")
	if err := h.run(t, "", "list", "--json"); err != nil {
		t.Fatal(err)
	}
	var entries []listEntry
	if err := json.Unmarshal(h.out.Bytes(), &entries); err != nil {
		t.Fatalf("invalid JSON %q: %v", h.out.String(), err)
	}
	if len(entries) != 1 || entries[0].Code != expectedCode(t, otp.DefaultParams()) || *entries[0].ExpiresIn != 5 {
		t.Errorf("unexpected entries %+v", entries)
	}
	if strings.Contains(h.out.String(), testSecret) {
		t.Error("list output must not contain the secret")
	}
}

func TestListMissingSecret(t *testing.T) {
	h := newHarness(t)
	_ = h.run(t, testURL, "add", "--url", "-")
	accts, _ := h.app.Store.List()
	_ = h.secrets.Delete(accts[0].ID)
	if err := h.run(t, "", "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.out.String(), "error") {
		t.Errorf("expected error marker in %q", h.out.String())
	}
}

func TestGetCopyAndClear(t *testing.T) {
	h := newHarness(t)
	_ = h.run(t, testURL, "add", "--url", "-")
	if err := h.run(t, "", "get", "test", "--copy", "--clear-after", "10ms"); err != nil {
		t.Fatal(err)
	}
	if h.clipboard.text != "" {
		t.Errorf("clipboard should be cleared, has %q", h.clipboard.text)
	}
	if !strings.Contains(h.err.String(), "Clipboard cleared") {
		t.Errorf("stderr = %q", h.err.String())
	}

	if err := h.run(t, "", "get", "test", "-c"); err != nil {
		t.Fatal(err)
	}
	if want := expectedCode(t, otp.DefaultParams()); h.clipboard.text != want {
		t.Errorf("clipboard = %q, want %q", h.clipboard.text, want)
	}
}

func TestRemove(t *testing.T) {
	h := newHarness(t)
	_ = h.run(t, testURL, "add", "--url", "-")
	if err := h.run(t, "", "rm", "test"); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("expected --yes error, got %v", err)
	}
	if err := h.run(t, "", "rm", "test", "--yes"); err != nil {
		t.Fatal(err)
	}
	if h.secrets.Len() != 0 {
		t.Error("secret should be deleted")
	}
	if err := h.run(t, "", "get", "test"); err == nil {
		t.Error("get after remove should fail")
	}
}

func TestWatchRequiresTerminal(t *testing.T) {
	h := newHarness(t)
	if err := h.run(t, "", "watch"); err == nil {
		t.Error("watch should fail without a terminal")
	}
}
