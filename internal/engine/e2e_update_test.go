package engine_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gfsync/internal/engine"
	"gfsync/internal/update"
)

// jumpClock is real time plus a jump the test controls (the watcher's
// settle delay needs the clock to keep moving).
type jumpClock struct {
	mu   sync.Mutex
	jump time.Duration
}

func (c *jumpClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return time.Now().Add(c.jump) }
func (c *jumpClock) set(d time.Duration) {
	c.mu.Lock()
	c.jump = d
	c.mu.Unlock()
}

// docs/AUTO_UPDATE.md §3: the site answers 426 client_too_old. Nothing is
// marked as answered, uploads stop (no retry churn), the player is told to
// update, an update check runs at once, and once the site takes this
// version again everything goes up.
func TestE2EClientTooOld426(t *testing.T) {
	clock := &jumpClock{}
	var checks atomic.Int32
	h := newHarness(t)
	h.cfg.Now = clock.now
	h.cfg.UpdateFirst = time.Hour // only the 426 should trigger a check
	h.cfg.CheckUpdate = func(context.Context) (*update.Result, error) {
		checks.Add(1)
		return nil, update.ErrNotFound
	}
	h.start()
	h.waitFor("paired", func() bool { return h.eng.Status().Paired })

	h.mock.SetMinClient("0.2.0") // the test client is 0.1.0-test
	h.writeSV("A", sv(2, true, "a:1:character.self", "a:2:character.self"))
	h.waitFor("refused with 426", func() bool { return h.eng.Status().ClientTooOld })
	s := h.eng.Status()
	if s.LastError != engine.MsgClientTooOld || s.MinVersion != "0.2.0" || s.Pending != 2 {
		t.Fatalf("status %+v", s)
	}
	if s.DownloadURL != h.srv.URL+"/pages/sync" {
		t.Errorf("download URL %q", s.DownloadURL)
	}
	h.waitFor("update check after 426", func() bool { return checks.Load() == 1 })
	calls := h.mock.Calls()
	time.Sleep(500 * time.Millisecond)
	if h.mock.Calls() != calls {
		t.Fatalf("kept sending after 426 (%d -> %d calls)", calls, h.mock.Calls())
	}
	if h.mock.Status("a:1:character.self") != "" {
		t.Fatal("nothing may be recorded for a refused request")
	}

	h.mock.SetMinClient("")
	clock.set(engine.TooOldRetry + time.Minute)
	h.waitFor("uploaded after the wait", func() bool {
		got := h.receivedIDs()
		return got["a:1:character.self"] && got["a:2:character.self"]
	})
	h.waitFor("notice cleared", func() bool {
		s := h.eng.Status()
		return !s.ClientTooOld && s.LastError == ""
	})
}

// A 426 on pairing ends the attempt with the same message instead of
// retrying or opening the browser.
func TestE2EPairingClientTooOld(t *testing.T) {
	h := newHarness(t)
	h.mock.SetMinClient("0.2.0")
	h.start()
	h.waitFor("pairing refused", func() bool {
		s := h.eng.Status()
		return s.PairEnded && !s.Pairing && s.ClientTooOld && s.LastError == engine.MsgClientTooOld
	})
	h.logMu.Lock()
	opened := len(h.opened)
	h.logMu.Unlock()
	if opened != 0 {
		t.Fatalf("browser opened %d times", opened)
	}
}

// addon_too_old is handled like unknown_kind: not an answer, retried after
// 24 hours, and the player is told to update the AddOn.
func TestE2EAddonTooOld(t *testing.T) {
	const sid = "Player-1-0000AAAA:1763370000:member.share:Player-1-0000BBBB"
	clock := &jumpClock{}
	h := newHarness(t)
	h.cfg.Now = clock.now
	h.mock.SetAddonTooOld("member.share")
	h.writeSV("A", outbox(entry("self:1:character.self", 1763370000), share(sid, 1763370000, "虛構乙", 10)))
	h.start()
	h.waitFor("own character accepted", func() bool { return h.receivedIDs()["self:1:character.self"] })
	h.waitFor("addon notice", func() bool {
		s := h.eng.Status()
		return s.AddonTooOld && len(s.Notices) > 0 && s.Notices[0] == engine.MsgAddonTooOld && s.Pending == 0
	})
	calls := h.mock.Calls()
	time.Sleep(500 * time.Millisecond)
	if h.mock.Calls() != calls {
		t.Fatalf("resent before 24 hours (%d -> %d calls)", calls, h.mock.Calls())
	}

	h.mock.SetAddonTooOld()
	clock.set(engine.UnknownKindRetry + time.Minute)
	h.waitFor("share accepted after 24 hours", func() bool { return h.receivedIDs()[sid] })
	h.waitFor("notice cleared", func() bool {
		s := h.eng.Status()
		return !s.AddonTooOld && len(s.Notices) == 0
	})
}

// A scheduled check that finds a newer version only reports it: the status
// carries the release page, never an installer, and nothing is fetched by
// the engine itself.
func TestE2EUpdateNotifyOnly(t *testing.T) {
	var checks atomic.Int32
	result := &update.Result{
		Manifest: update.Manifest{Version: "0.2.0", Notes: "修正\n很多\t東西", PrivacyChanged: true},
		Current:  "0.1.0-test", Newer: true, Required: true, Source: "site",
	}
	h := newHarness(t)
	h.cfg.UpdateFirst = 50 * time.Millisecond
	h.cfg.CheckUpdate = func(context.Context) (*update.Result, error) {
		checks.Add(1)
		return result, nil
	}
	h.start()
	h.waitFor("update found", func() bool { return h.eng.Status().Update != nil })
	u := h.eng.Status().Update
	if u.Version != "0.2.0" || !u.Required || !u.PrivacyChanged || u.Notes != "修正 很多 東西" ||
		u.Page != "https://github.com/GreenAndFangAdventure/gf-sync/releases/tag/v0.2.0" {
		t.Fatalf("update %+v", u)
	}
	time.Sleep(300 * time.Millisecond)
	if n := checks.Load(); n != 1 {
		t.Errorf("%d scheduled checks, want 1 (next one is a day later)", n)
	}

	// 「檢查更新」 in the menu.
	got, err := h.eng.CheckUpdateNow(context.Background())
	if err != nil || got == nil || got.Version != "0.2.0" || checks.Load() != 2 {
		t.Fatalf("CheckUpdateNow = %+v, %v (checks %d)", got, err, checks.Load())
	}
	result = &update.Result{Manifest: update.Manifest{Version: "0.1.0"}, Current: "0.1.0-test"}
	if got, err := h.eng.CheckUpdateNow(context.Background()); err != nil || got != nil {
		t.Fatalf("up to date: %+v, %v", got, err)
	}
	h.waitForLog("update: 0.2.0 available")
}

func TestE2EUpdateChecksOff(t *testing.T) {
	h := newHarness(t)
	h.start()
	if _, err := h.eng.CheckUpdateNow(context.Background()); !errors.Is(err, engine.ErrUpdatesOff) {
		t.Fatalf("err = %v", err)
	}
	h.waitFor("ready", func() bool { return h.eng.Status().Ready })
	if h.eng.Status().Update != nil {
		t.Fatal("no update without a checker")
	}
	h.logMu.Lock()
	defer h.logMu.Unlock()
	for _, l := range h.logs {
		if strings.HasPrefix(l, "update:") {
			t.Fatalf("checked anyway: %s", l)
		}
	}
}
