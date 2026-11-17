package secret

import (
	"bytes"
	"os"
	"testing"
)

func TestDPAPIRoundTrip(t *testing.T) {
	s := NewDPAPI(t.TempDir())
	if tok, err := s.Load(); err != nil || tok != "" {
		t.Fatal("absent token should be empty", err)
	}
	const tok = "c2VjcmV0LXRva2VuLTMyLWJ5dGVzLWxvbmctcmFuZG9t"
	if err := s.Save(tok); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(s.Path)
	if bytes.Contains(raw, []byte(tok)) {
		t.Fatal("token stored in plain text")
	}
	got, err := s.Load()
	if err != nil || got != tok {
		t.Fatalf("got %q %v", got, err)
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Load(); got != "" {
		t.Fatal("clear")
	}
	if err := s.Clear(); err != nil {
		t.Fatal("clear twice", err)
	}
}

func TestTamperedBlobFails(t *testing.T) {
	s := NewDPAPI(t.TempDir())
	s.Save("abc")
	raw, _ := os.ReadFile(s.Path)
	raw[len(raw)/2] ^= 0xFF
	os.WriteFile(s.Path, raw, 0o600)
	if _, err := s.Load(); err == nil {
		t.Fatal("tampered blob must not decrypt")
	}
}
