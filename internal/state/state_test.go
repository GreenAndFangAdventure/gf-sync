package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "GFSync", FileName)
	s, err := Load(p)
	if err != nil || len(s.Uploaded) != 0 {
		t.Fatal(err)
	}
	s.Mark("a", Record{At: 1, Status: "accepted"})
	s.GameDir = `D:\World of Warcraft\_forever_`
	s.Paused = true
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Has("a") || s2.Has("b") || !s2.Paused || s2.GameDir != s.GameDir {
		t.Fatalf("%+v", s2)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file left behind")
	}
}

func TestCorruptedIsReset(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, FileName)
	os.WriteFile(p, []byte("{not json"), 0o600)
	s, err := Load(p)
	if err == nil || s == nil || len(s.Uploaded) != 0 {
		t.Fatalf("want warning and empty state, got %v %+v", err, s)
	}
	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), FileName+".bad-") {
			found = true
		}
	}
	if !found {
		t.Fatal("corrupted file should be kept aside")
	}
}

func TestPrune(t *testing.T) {
	s := New()
	for i := 0; i < maxRecords+10; i++ {
		s.Mark(fmt.Sprint(i), Record{At: int64(i)})
	}
	s.prune()
	if len(s.Uploaded) != maxRecords || s.Has("0") || !s.Has(fmt.Sprint(maxRecords+9)) {
		t.Fatal("prune should drop the oldest")
	}
}

// Answers belong to one site and device (LOG #058): after switching the
// API base or pairing as a new device, old answers no longer count.
func TestScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	// A file from before scopes existed.
	if err := os.WriteFile(path, []byte(`{"version":1,"uploaded":{"a":{"at":1,"status":"accepted"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Scope != "" || !s.Has("a") {
		t.Fatalf("legacy load: scope %q done %v", s.Scope, s.Has("a"))
	}
	if !s.SetScope("https://staging.example device:1") || s.Has("a") {
		t.Fatal("unscoped answers must not count for a known site")
	}
	s.Mark("b", Record{At: 2, Status: "accepted"})
	if s.SetScope("https://staging.example device:1") || !s.Has("b") {
		t.Fatal("the same scope keeps its answers")
	}
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	s, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Scope != "https://staging.example device:1" || !s.Has("b") {
		t.Fatalf("scope not saved: %q", s.Scope)
	}
	if !s.SetScope("https://www.example device:1") || s.Has("b") {
		t.Fatal("another API base starts empty")
	}
	s.Mark("c", Record{At: 3, Status: "accepted"})
	if !s.SetScope("https://www.example device:2") || s.Has("c") {
		t.Fatal("another device starts empty")
	}
	if New().SetScope("x") {
		t.Fatal("nothing dropped from an empty state")
	}
}

// upload-api v1.1 §2: "unknown_kind" is not an answer; retry later.
func TestRetryAt(t *testing.T) {
	s := New()
	s.Mark("a", Record{At: 100, Status: "rejected", Reason: "unknown_kind", RetryAt: 100 + 86400})
	if !s.Done("a", 200) {
		t.Fatal("waits until RetryAt")
	}
	if s.Done("a", 100+86400) {
		t.Fatal("due again at RetryAt")
	}
	s.Mark("b", Record{At: 100, Status: "rejected", Reason: "wrong_guild"})
	if !s.Done("b", 1<<40) {
		t.Fatal("other answers are final")
	}
	if s.Done("c", 0) || s.Has("c") || !s.Has("a") {
		t.Fatal("unknown id")
	}
}
