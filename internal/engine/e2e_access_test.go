package engine_test

// Security software (or permissions) restricting GF Sync: a folder or
// file that exists but cannot be read must be reported as "blocked", not
// as "not found", and GF Sync's own files must never be reset or deleted
// because of it.

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gfsync/internal/engine"
	"gfsync/internal/gamedir"
	"gfsync/internal/secret"
	"gfsync/internal/state"
)

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestE2EGameFolderDeniedIsNotNotFound(t *testing.T) {
	h := newHarness(t)
	h.writeSV("A", sv(2, true, "a:1:character.self"))
	var allowed atomic.Bool
	h.cfg.DetectEvery = 50 * time.Millisecond
	h.cfg.Detect = func() (string, error) {
		if allowed.Load() {
			return h.forever, nil
		}
		return "", fmt.Errorf("%w: %s", gamedir.ErrAccessDenied, h.forever)
	}
	h.start()
	h.waitFor("denied", func() bool {
		s := h.eng.Status()
		return s.Ready && s.GameDirDenied
	})
	for i := 0; i < 5; i++ {
		if s := h.eng.Status(); s.NeedGameDir || !s.GameDirDenied {
			t.Fatalf("blocked folder must not ask the player to pick one: %+v", s)
		}
		time.Sleep(60 * time.Millisecond)
	}
	h.waitForLog(`game folder: access denied (not "not found")`)

	// Security software lets GF Sync in: it carries on by itself.
	allowed.Store(true)
	h.waitFor("upload", func() bool { return h.receivedIDs()["a:1:character.self"] })
	if s := h.eng.Status(); s.GameDirDenied || s.NeedGameDir || s.GameDir != h.forever {
		t.Fatalf("after access: %+v", s)
	}
}

func TestE2ENoGameFolderStillAsks(t *testing.T) {
	h := newHarness(t)
	h.cfg.Detect = func() (string, error) { return "", nil }
	h.start()
	h.waitFor("need folder", func() bool { s := h.eng.Status(); return s.NeedGameDir && !s.GameDirDenied })
}

func TestE2ESavedFolderUsedWithoutDetect(t *testing.T) {
	h := newHarness(t)
	h.writeSV("A", sv(2, true, "a:1:character.self"))
	st := state.New()
	st.GameDir = h.forever
	if err := st.Save(filepath.Join(h.data, state.FileName)); err != nil {
		t.Fatal(err)
	}
	h.cfg.Detect = func() (string, error) { t.Error("must not detect when the saved folder works"); return "", nil }
	h.start()
	h.waitFor("upload", func() bool { return h.receivedIDs()["a:1:character.self"] })
}

func TestE2ETokenUnreadableIsKept(t *testing.T) {
	h := newHarness(t)
	h.cfg.DetectEvery = 50 * time.Millisecond
	// token.dat exists but cannot be read (a directory stands in for a
	// file the security software will not let GF Sync open).
	tok := filepath.Join(h.data, secret.FileName)
	if err := os.MkdirAll(tok, 0o700); err != nil {
		t.Fatal(err)
	}
	h.start()
	h.waitFor("token unreadable", func() bool {
		s := h.eng.Status()
		return s.Ready && s.TokenUnreadable && has(s.Storage, engine.StorageToken)
	})
	h.waitForLog("token: cannot read")
	time.Sleep(300 * time.Millisecond)
	if st, err := os.Stat(tok); err != nil || !st.IsDir() {
		t.Fatalf("token.dat must not be deleted: %v", err)
	}
	h.logMu.Lock()
	opened := len(h.opened)
	h.logMu.Unlock()
	if opened != 0 || h.eng.Status().Pairing {
		t.Fatal("must not start pairing over a token it cannot read")
	}

	// Readable again (here: really absent): the warning goes away.
	if err := os.Remove(tok); err != nil {
		t.Fatal(err)
	}
	h.waitFor("token readable", func() bool {
		s := h.eng.Status()
		return !s.TokenUnreadable && !has(s.Storage, engine.StorageToken)
	})
}

func TestE2EStateUnreadableIsNotOverwritten(t *testing.T) {
	h := newHarness(t)
	h.cfg.DetectEvery = 50 * time.Millisecond
	h.writeSV("A", sv(2, true, "a:1:character.self"))
	sp := filepath.Join(h.data, state.FileName)
	if err := os.MkdirAll(sp, 0o700); err != nil {
		t.Fatal(err)
	}
	h.start()
	h.waitFor("state unreadable", func() bool {
		s := h.eng.Status()
		return s.Ready && has(s.Storage, engine.StorageState) && s.GameDir == h.forever
	})
	h.waitForLog("leaving it untouched")
	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(sp + ".tmp"); err == nil {
		t.Fatal("must not try to save over a state file it could not read")
	}

	// Readable again: the state is loaded, merged and saved.
	if err := os.Remove(sp); err != nil {
		t.Fatal(err)
	}
	h.waitFor("state saved", func() bool {
		s, err := state.Load(sp)
		return err == nil && s.GameDir == h.forever && !has(h.eng.Status().Storage, engine.StorageState)
	})
}

func TestE2ELogProblemShown(t *testing.T) {
	h := newHarness(t)
	var bad atomic.Bool
	bad.Store(true)
	h.cfg.LogErr = func() error {
		if bad.Load() {
			return os.ErrPermission
		}
		return nil
	}
	h.start()
	h.waitFor("log problem", func() bool { return has(h.eng.Status().Storage, engine.StorageLog) })
	bad.Store(false)
	h.eng.SyncNow()
	h.waitFor("log ok", func() bool { return !has(h.eng.Status().Storage, engine.StorageLog) })
}

func TestE2EPickedEmptyFolderIsNotFound(t *testing.T) {
	h := newHarness(t)
	h.cfg.Detect = func() (string, error) { return "", nil }
	h.start()
	h.waitFor("need folder", func() bool { return h.eng.Status().NeedGameDir })
	if err := h.eng.SetGameDir(t.TempDir()); err == nil || err.Error() == engine.MsgFolderDenied {
		t.Fatalf("an empty folder is not found, not denied: %v", err)
	}
}
