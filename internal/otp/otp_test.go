package otp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

func b32(s string) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(s))
}

// Test vectors from RFC 6238 Appendix B.
func TestGenerateRFC6238(t *testing.T) {
	seeds := map[string]string{
		SHA1:   b32("12345678901234567890"),
		SHA256: b32("12345678901234567890123456789012"),
		SHA512: b32("1234567890123456789012345678901234567890123456789012345678901234"),
	}
	vectors := []struct {
		unix int64
		want map[string]string
	}{
		{59, map[string]string{SHA1: "94287082", SHA256: "46119246", SHA512: "90693936"}},
		{1111111109, map[string]string{SHA1: "07081804", SHA256: "68084774", SHA512: "25091201"}},
		{1111111111, map[string]string{SHA1: "14050471", SHA256: "67062674", SHA512: "99943326"}},
		{1234567890, map[string]string{SHA1: "89005924", SHA256: "91819424", SHA512: "93441116"}},
		{2000000000, map[string]string{SHA1: "69279037", SHA256: "90698825", SHA512: "38618901"}},
		{20000000000, map[string]string{SHA1: "65353130", SHA256: "77737706", SHA512: "47863826"}},
	}
	for _, v := range vectors {
		for alg, want := range v.want {
			p := Params{Algorithm: alg, Digits: 8, Period: 30}
			got, err := Generate(seeds[alg], p, time.Unix(v.unix, 0))
			if err != nil {
				t.Fatalf("%s @%d: %v", alg, v.unix, err)
			}
			if got.Value != want {
				t.Errorf("%s @%d: got %s, want %s", alg, v.unix, got.Value, want)
			}
		}
	}
}

func TestGenerateRemaining(t *testing.T) {
	p := DefaultParams()
	got, err := Generate("JBSWY3DPEHPK3PXP", p, time.Unix(65, 0))
	if err != nil {
		t.Fatal(err)
	}
	if got.Remaining != 25*time.Second {
		t.Errorf("remaining = %v, want 25s", got.Remaining)
	}
	if len(got.Value) != 6 {
		t.Errorf("code %q should have 6 digits", got.Value)
	}
}

func TestParseURL(t *testing.T) {
	k, err := ParseURL("otpauth://totp/ACME%20Co:alice@example.com?secret=jbsw%20y3dp-ehpk3pxp&issuer=ACME%20Co&algorithm=SHA256&digits=8&period=60")
	if err != nil {
		t.Fatal(err)
	}
	want := Key{
		Params: Params{Issuer: "ACME Co", Account: "alice@example.com", Algorithm: SHA256, Digits: 8, Period: 60},
		Secret: "JBSWY3DPEHPK3PXP",
	}
	if k != want {
		t.Errorf("got %+v, want %+v", k, want)
	}
}

func TestParseURLDefaults(t *testing.T) {
	k, err := ParseURL("otpauth://totp/dev?secret=JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if k.Params != (Params{Account: "dev", Algorithm: SHA1, Digits: 6, Period: 30}) {
		t.Errorf("unexpected params %+v", k.Params)
	}
}

func TestParseURLErrors(t *testing.T) {
	cases := map[string]string{
		"https://example.com":                                    "not an otpauth",
		"otpauth://hotp/x?secret=JBSWY3DPEHPK3PXP&counter=1":     "only totp",
		"otpauth://totp/x?issuer=a":                              "secret is empty",
		"otpauth://totp/x?secret=not*base32":                     "not valid base32",
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&algorithm=MD5": "unsupported algorithm",
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&digits=4":      "digits must be",
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&period=0":      "period must be",
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&encoder=steam": "unsupported encoder",
	}
	for raw, want := range cases {
		_, err := ParseURL(raw)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseURL(%q) error = %v, want containing %q", raw, err, want)
		}
		if err != nil && strings.Contains(err.Error(), "JBSWY3DPEHPK3PXP") {
			t.Errorf("error for %q leaks the secret: %v", raw, err)
		}
	}
}

func TestNormalizeSecret(t *testing.T) {
	got, err := NormalizeSecret(" jbsw y3dp-ehpk 3pxp== ")
	if err != nil || got != "JBSWY3DPEHPK3PXP" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := NormalizeSecret("   "); err == nil {
		t.Error("expected error for empty secret")
	}
	if _, err := NormalizeSecret("0189"); err == nil {
		t.Error("expected error for invalid base32")
	}
}

func TestParseAlgorithm(t *testing.T) {
	for in, want := range map[string]string{"": SHA1, "sha1": SHA1, "SHA-256": SHA256, "sha512": SHA512} {
		got, err := ParseAlgorithm(in)
		if err != nil || got != want {
			t.Errorf("ParseAlgorithm(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
