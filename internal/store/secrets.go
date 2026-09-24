package store

import (
	"errors"
	"sync"

	"github.com/zalando/go-keyring"
)

// ErrSecretNotFound is returned when no secret exists for an account.
var ErrSecretNotFound = errors.New("secret not found")

// SecretStore holds TOTP secrets, keyed by account id.
type SecretStore interface {
	Set(id, secret string) error
	Get(id string) (string, error)
	Delete(id string) error
}

// KeyringSecrets stores secrets in the OS keychain (Windows Credential
// Manager, macOS Keychain, or the Secret Service on Linux).
type KeyringSecrets struct {
	Service string
}

func (k KeyringSecrets) Set(id, secret string) error {
	return keyring.Set(k.Service, id, secret)
}

func (k KeyringSecrets) Get(id string) (string, error) {
	s, err := keyring.Get(k.Service, id)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrSecretNotFound
	}
	return s, err
}

func (k KeyringSecrets) Delete(id string) error {
	err := keyring.Delete(k.Service, id)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrSecretNotFound
	}
	return err
}

// MemorySecrets is an in-memory SecretStore for tests.
type MemorySecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func NewMemorySecrets() *MemorySecrets {
	return &MemorySecrets{m: map[string]string{}}
}

func (m *MemorySecrets) Set(id, secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[id] = secret
	return nil
}

func (m *MemorySecrets) Get(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.m[id]
	if !ok {
		return "", ErrSecretNotFound
	}
	return s, nil
}

func (m *MemorySecrets) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.m[id]; !ok {
		return ErrSecretNotFound
	}
	delete(m.m, id)
	return nil
}

// Len returns the number of stored secrets.
func (m *MemorySecrets) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.m)
}
