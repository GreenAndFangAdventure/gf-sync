// Package secret stores the device token encrypted with Windows DPAPI
// (current-user scope). The token never appears in logs or state.json.
package secret

import (
	"errors"
	"os"
	"path/filepath"
)

// Store keeps one secret value.
type Store interface {
	Load() (string, error) // returns "", nil when absent
	Save(token string) error
	Clear() error
}

// FileName is the encrypted token file inside the data directory.
const FileName = "token.dat"

// entropy is mixed into DPAPI so other programs calling CryptUnprotectData
// on this blob without it fail.
var entropy = []byte("GFSync device token v1")

// DPAPIStore saves to <dir>/token.dat.
type DPAPIStore struct{ Path string }

// NewDPAPI returns a store in dir.
func NewDPAPI(dir string) *DPAPIStore { return &DPAPIStore{Path: filepath.Join(dir, FileName)} }

func (s *DPAPIStore) Load() (string, error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	plain, err := unprotect(b)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *DPAPIStore) Save(token string) error {
	blob, err := protect([]byte(token))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

func (s *DPAPIStore) Clear() error {
	err := os.Remove(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// MemStore is an in-memory Store for tests.
type MemStore struct{ Token string }

func (m *MemStore) Load() (string, error) { return m.Token, nil }
func (m *MemStore) Save(t string) error   { m.Token = t; return nil }
func (m *MemStore) Clear() error          { m.Token = ""; return nil }
