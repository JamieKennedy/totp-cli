// Package otp parses TOTP definitions and generates codes.
package otp

import (
	"encoding/base32"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	potp "github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// Supported hash algorithms.
const (
	SHA1   = "SHA1"
	SHA256 = "SHA256"
	SHA512 = "SHA512"
)

// Defaults per RFC 6238 and common authenticator apps.
const (
	DefaultDigits    = 6
	DefaultPeriod    = 30
	DefaultAlgorithm = SHA1
)

// Params describes how codes are generated for a key. It contains no secret
// material and is safe to persist in plain text.
type Params struct {
	Issuer    string
	Account   string
	Algorithm string
	Digits    int
	Period    int
}

// Key is a TOTP definition together with its base32-encoded secret.
type Key struct {
	Params
	Secret string
}

// DefaultParams returns Params populated with the standard defaults.
func DefaultParams() Params {
	return Params{Algorithm: DefaultAlgorithm, Digits: DefaultDigits, Period: DefaultPeriod}
}

// IsURL reports whether s looks like an otpauth:// URL.
func IsURL(s string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "otpauth://")
}

// ParseURL parses an otpauth://totp/... URL.
func ParseURL(raw string) (Key, error) {
	raw = strings.TrimSpace(raw)
	if !IsURL(raw) {
		return Key{}, errors.New("not an otpauth:// URL")
	}
	k, err := potp.NewKeyFromURL(raw)
	if err != nil {
		return Key{}, errors.New("invalid otpauth URL")
	}
	if !strings.EqualFold(k.Type(), "totp") {
		return Key{}, fmt.Errorf("unsupported OTP type %q: only totp is supported", k.Type())
	}
	if k.Encoder() != potp.EncoderDefault {
		return Key{}, errors.New("unsupported encoder: only standard numeric codes are supported")
	}

	secret, err := NormalizeSecret(k.Secret())
	if err != nil {
		return Key{}, err
	}

	// Read the algorithm directly: the library silently maps unknown values to SHA1.
	u, err := url.Parse(raw)
	if err != nil {
		return Key{}, errors.New("invalid otpauth URL")
	}
	alg, err := ParseAlgorithm(u.Query().Get("algorithm"))
	if err != nil {
		return Key{}, err
	}

	key := Key{
		Params: Params{
			Issuer:    k.Issuer(),
			Account:   k.AccountName(),
			Algorithm: alg,
			Digits:    int(k.Digits()),
			Period:    int(min(k.Period(), 3601)), // clamp so Validate rejects huge values
		},
		Secret: secret,
	}
	if err := key.Validate(); err != nil {
		return Key{}, err
	}
	return key, nil
}

// ParseAlgorithm normalises an algorithm name such as "sha256" or "SHA-256".
func ParseAlgorithm(s string) (string, error) {
	switch strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), "-", "")) {
	case "", SHA1:
		return SHA1, nil
	case SHA256:
		return SHA256, nil
	case SHA512:
		return SHA512, nil
	default:
		return "", fmt.Errorf("unsupported algorithm %q: use SHA1, SHA256 or SHA512", s)
	}
}

// NormalizeSecret strips whitespace, dashes and padding, upper-cases the
// secret and checks that it is valid base32.
func NormalizeSecret(s string) (string, error) {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n', '-':
			return -1
		}
		return r
	}, s)
	s = strings.ToUpper(strings.TrimRight(s, "="))
	if s == "" {
		return "", errors.New("secret is empty")
	}
	if _, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s); err != nil {
		return "", errors.New("secret is not valid base32")
	}
	return s, nil
}

// Validate checks that the parameters are within supported ranges.
func (p Params) Validate() error {
	if _, err := ParseAlgorithm(p.Algorithm); err != nil {
		return err
	}
	if p.Digits < 6 || p.Digits > 8 {
		return fmt.Errorf("digits must be between 6 and 8, got %d", p.Digits)
	}
	if p.Period < 1 || p.Period > 3600 {
		return fmt.Errorf("period must be between 1 and 3600 seconds, got %d", p.Period)
	}
	return nil
}

// Code is a generated TOTP code and how long it remains valid.
type Code struct {
	Value     string
	Remaining time.Duration
	Period    time.Duration
}

// Generate produces the code for secret at time t.
func Generate(secret string, p Params, t time.Time) (Code, error) {
	if err := p.Validate(); err != nil {
		return Code{}, err
	}
	alg, _ := ParseAlgorithm(p.Algorithm)
	value, err := totp.GenerateCodeCustom(secret, t, totp.ValidateOpts{
		Period:    uint(p.Period),
		Digits:    potp.Digits(p.Digits),
		Algorithm: toLibAlgorithm(alg),
	})
	if err != nil {
		return Code{}, errors.New("could not generate code: secret is not valid base32")
	}
	period := time.Duration(p.Period) * time.Second
	elapsed := time.Duration(t.UnixNano() % int64(period))
	return Code{Value: value, Remaining: period - elapsed, Period: period}, nil
}

func toLibAlgorithm(alg string) potp.Algorithm {
	switch alg {
	case SHA256:
		return potp.AlgorithmSHA256
	case SHA512:
		return potp.AlgorithmSHA512
	default:
		return potp.AlgorithmSHA1
	}
}
