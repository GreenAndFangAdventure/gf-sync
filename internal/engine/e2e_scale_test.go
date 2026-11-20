package engine_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// EVAL-1000: a guild of 1,000 members gives about 1,000 member.share
// entries of about 4 KB in one file. All of them go up, at most 50 items
// and 1 MB per request; when the hourly cap is reached the rest waits for
// a later run instead of being dropped, and the player's own character
// goes first. Made-up names.
func TestE2EGuild1000SpreadOverRuns(t *testing.T) {
	const members = 1000
	var mu sync.Mutex
	var jump time.Duration
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return time.Now().Add(jump) }
	setJump := func(d time.Duration) { mu.Lock(); jump = d; mu.Unlock() }

	var entries []string
	want := map[string]bool{}
	for i := 0; i < members; i++ {
		id := fmt.Sprintf("Player-1-0000AAAA:1763370000:member.share:Player-1-%08X", i)
		entries = append(entries, share(id, 1763370000, fmt.Sprintf("虛構成員%d", i), 600))
		want[id] = true
	}
	// The player's own character is written last in the outbox but must
	// not wait behind the 1,000 shares.
	entries = append(entries, entry("self:1:character.self", 1763370000))
	want["self:1:character.self"] = true

	h := newHarness(t)
	h.cfg.Now = clock
	h.cfg.HourlyLimit = 10
	h.writeSV("A", outbox(entries...))
	h.start()

	// 1,001 entries / 50 per request = 21 requests: 10 + 10 + 1 over
	// three hours.
	h.waitFor("first hour's 10 requests", func() bool { return h.mock.Calls() == 10 })
	time.Sleep(500 * time.Millisecond)
	if c := h.mock.Calls(); c != 10 {
		t.Fatalf("%d requests in the first hour (cap 10)", c)
	}
	st := h.eng.Status()
	if st.Pending != members+1-500 || !strings.Contains(st.LastError, "上限") {
		t.Fatalf("after the first hour: pending %d, last error %q", st.Pending, st.LastError)
	}
	if first := firstID(t, h.mock.Received()); first != "self:1:character.self" {
		t.Errorf("first item uploaded is %q, want the player's own character", first)
	}

	setJump(time.Hour + time.Minute)
	h.waitFor("second hour", func() bool { return h.mock.Calls() == 20 })
	setJump(2*time.Hour + 2*time.Minute)
	h.waitFor("all 1,001 accepted", func() bool {
		got := h.receivedIDs()
		for id := range want {
			if !got[id] {
				return false
			}
		}
		return true
	})
	// 21 means no request was refused for size or item count (a 413 or
	// 400 would split the batch and add requests) and nothing was resent.
	if c := h.mock.Calls(); c != 21 {
		t.Errorf("%d upload requests for 1,001 entries (want 21)", c)
	}
	if n := len(h.mock.Received()); n != members+1 {
		t.Errorf("%d items received (want %d, each once)", n, members+1)
	}
	h.logMu.Lock()
	defer h.logMu.Unlock()
	for _, l := range h.logs {
		if strings.Contains(l, "虛構成員") || strings.Contains(l, "虛構森林") {
			t.Fatalf("log names a member or a place: %s", l)
		}
	}
}

// The unknown_kind 24-hour retry still holds for a large outbox: the
// shares are not answered, the other entries are, and nothing is resent
// until the retry time.
func TestE2EGuild1000UnknownKind(t *testing.T) {
	const members = 1000
	var mu sync.Mutex
	var jump time.Duration
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return time.Now().Add(jump) }

	var entries []string
	for i := 0; i < members; i++ {
		id := fmt.Sprintf("Player-1-0000AAAA:1763370000:member.share:Player-1-%08X", i)
		entries = append(entries, share(id, 1763370000, fmt.Sprintf("虛構成員%d", i), 10))
	}
	entries = append(entries, entry("self:1:character.self", 1763370000))

	h := newHarness(t)
	h.cfg.Now = clock
	h.mock.SetUnknownKinds("member.share")
	h.writeSV("A", outbox(entries...))
	h.start()
	h.waitFor("every share answered unknown_kind", func() bool {
		return h.mock.Calls() >= 21 && h.eng.Status().Pending == 0
	})
	calls := h.mock.Calls()
	if calls != 21 {
		t.Errorf("%d requests (want 21)", calls)
	}
	time.Sleep(500 * time.Millisecond)
	if h.mock.Calls() != calls {
		t.Fatalf("resent before 24 hours (%d -> %d calls)", calls, h.mock.Calls())
	}

	h.mock.SetUnknownKinds()
	mu.Lock()
	jump = 24*time.Hour + time.Minute
	mu.Unlock()
	h.waitFor("all shares accepted after 24 hours", func() bool {
		got := h.receivedIDs()
		for i := 0; i < members; i++ {
			if !got[fmt.Sprintf("Player-1-0000AAAA:1763370000:member.share:Player-1-%08X", i)] {
				return false
			}
		}
		return true
	})
}

func firstID(t *testing.T, items []json.RawMessage) string {
	t.Helper()
	if len(items) == 0 {
		return ""
	}
	var m struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(items[0], &m); err != nil {
		t.Fatal(err)
	}
	return m.ID
}
