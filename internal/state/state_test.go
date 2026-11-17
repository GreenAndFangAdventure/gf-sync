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
	if !s2.Done("a") || s2.Done("b") || !s2.Paused || s2.GameDir != s.GameDir {
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
	if len(s.Uploaded) != maxRecords || s.Done("0") || !s.Done(fmt.Sprint(maxRecords+9)) {
		t.Fatal("prune should drop the oldest")
	}
}
