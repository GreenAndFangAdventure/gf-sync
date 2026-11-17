package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSettle(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "GreenFangToolkit.lua")
	os.WriteFile(p, []byte("a"), 0o644)
	now := time.Unix(1000, 0)
	w := New()
	w.Now = func() time.Time { return now }

	if r := w.Scan([]string{p}); len(r) != 0 {
		t.Fatal("must wait for settle on first sight")
	}
	now = now.Add(4 * time.Second)
	if r := w.Scan([]string{p}); len(r) != 0 {
		t.Fatal("4s is not enough")
	}
	// The game writes again: timer restarts.
	os.WriteFile(p, []byte("ab"), 0o644)
	now = now.Add(2 * time.Second)
	if r := w.Scan([]string{p}); len(r) != 0 {
		t.Fatal("changed file must restart settle")
	}
	now = now.Add(5 * time.Second)
	if r := w.Scan([]string{p}); len(r) != 1 {
		t.Fatal("stable 5s should be ready")
	}
	now = now.Add(time.Minute)
	if r := w.Scan([]string{p}); len(r) != 0 {
		t.Fatal("unchanged file is reported once")
	}
	w.ForceAll()
	if r := w.Scan([]string{p}); len(r) != 1 {
		t.Fatal("ForceAll should make it ready")
	}
	w.Touch(p)
	if r := w.Scan([]string{p}); len(r) != 0 {
		t.Fatal("Touch waits for settle")
	}
	now = now.Add(Settle)
	if r := w.Scan([]string{p}); len(r) != 1 {
		t.Fatal("Touch then settle")
	}
	// Mtime change with same size.
	later := now.Add(time.Hour)
	os.Chtimes(p, later, later)
	now = now.Add(Settle)
	w.Scan([]string{p})
	now = now.Add(Settle)
	if r := w.Scan([]string{p}); len(r) != 1 {
		t.Fatal("mtime change should be detected")
	}
	os.Remove(p)
	if r := w.Scan([]string{p}); len(r) != 0 || len(w.files) != 0 {
		t.Fatal("removed file forgotten")
	}
}
