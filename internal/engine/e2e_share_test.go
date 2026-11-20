package engine_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gfsync/internal/engine"
	"gfsync/internal/mockapi"
)

// share is a member.share entry (savedvariables v2 ext) with about
// recipeIDs recipe ids. Made-up names.
func share(id string, capturedAt int64, subject string, recipeIDs int) string {
	var ids strings.Builder
	for i := 0; i < recipeIDs; i++ {
		fmt.Fprintf(&ids, "%d, ", 2000+i)
	}
	return fmt.Sprintf(`
			{
				["id"] = %q,
				["kind"] = "member.share",
				["capturedAt"] = %d,
				["capturedAtServer"] = %d,
				["game"] = { ["world"] = "forever", ["interface"] = 16001, ["realm"] = "虛構伺服器", ["locale"] = "zhTW" },
				["uploader"] = { ["guid"] = "Player-1-1", ["name"] = "虛構", ["realm"] = "虛構伺服器", ["faction"] = "Horde", ["guild"] = %q },
				["payload"] = {
					["subject"] = { ["name"] = %q, ["realm"] = "虛構伺服器" },
					["level"] = { ["value"] = 60, ["recordedAt"] = %d, ["via"] = "relay" },
					["location"] = { ["value"] = { ["zone"] = "虛構森林" }, ["recordedAt"] = %d, ["via"] = "relay" },
					["recipes"] = { ["164"] = { ["recordedAt"] = %d, ["via"] = "relay", ["hash"] = "3fz9q", ["ids"] = { %s } } },
					["withdrawn"] = { ["location"] = 1 },
				},
			},`, id, capturedAt, capturedAt, mockapi.Guild, subject, capturedAt, capturedAt, capturedAt, ids.String())
}

func outbox(entries ...string) string {
	return "GreenFangToolkitDB = {\n\t[\"schemaVersion\"] = 2,\n\t[\"sync\"] = {\n\t\t[\"enabled\"] = true,\n\t\t[\"outbox\"] = {" +
		strings.Join(entries, "") + "\n\t\t},\n\t},\n}\n"
}

// upload-api v1.1 §2: an old site answers "unknown_kind". That is not an
// answer: GF Sync does not resend at once, but tries again after 24
// hours, and the entry then goes up.
func TestE2EUnknownKindRetried(t *testing.T) {
	const id = "Player-1-0000AAAA:1763370000:member.share:Player-1-0000BBBB"
	// Real time plus a jump we control (the watcher's settle delay needs
	// the clock to keep moving).
	var mu sync.Mutex
	var jump time.Duration
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return time.Now().Add(jump) }
	h := newHarness(t)
	h.cfg.Now = clock
	h.mock.SetUnknownKinds("member.share")
	h.writeSV("A", outbox(entry("self:1:character.self", 1763370000), share(id, 1763370000, "虛構乙", 10)))
	h.start()
	h.waitFor("character.self accepted", func() bool { return h.receivedIDs()["self:1:character.self"] })
	h.waitForLog("1 entries of a kind the site does not know yet")

	calls := h.mock.Calls()
	time.Sleep(500 * time.Millisecond)
	if h.mock.Calls() != calls {
		t.Fatalf("resent before 24 hours (%d -> %d calls)", calls, h.mock.Calls())
	}
	if h.mock.Status(id) != "" {
		t.Fatal("the mock must not remember unknown_kind ids")
	}

	h.mock.SetUnknownKinds() // the site is updated
	mu.Lock()
	jump = engine.UnknownKindRetry + time.Minute
	mu.Unlock()
	h.waitFor("retried after 24 hours", func() bool { return h.receivedIDs()[id] })
}

// savedvariables §2f: the outbox may hold 200 entries, 150 of them
// member.share of about 4 KB. They go up in batches of at most 50, and
// the log names no member.
func TestE2EOutbox200(t *testing.T) {
	var entries []string
	want := map[string]bool{}
	for i := 0; i < 150; i++ {
		id := fmt.Sprintf("Player-1-0000AAAA:1763370000:member.share:Player-1-%08X", i)
		entries = append(entries, share(id, 1763370000, fmt.Sprintf("虛構成員%d", i), 600))
		want[id] = true
	}
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("self:%d:character.self", i)
		entries = append(entries, entry(id, 1763370000+int64(i)))
		want[id] = true
	}
	h := newHarness(t)
	h.writeSV("A", outbox(entries...))
	h.start()
	h.waitFor("all 200 accepted", func() bool {
		got := h.receivedIDs()
		for id := range want {
			if !got[id] {
				return false
			}
		}
		return true
	})
	if c := h.mock.Calls(); c < 4 || c > 6 {
		t.Errorf("%d upload requests for 200 entries (want about 4)", c)
	}
	for _, raw := range h.mock.Received() {
		if strings.Contains(string(raw), `"member.share"`) && len(raw) < 3000 {
			t.Fatalf("fixture: a member.share should be about 4 KB, got %d bytes", len(raw))
		}
	}
	h.logMu.Lock()
	defer h.logMu.Unlock()
	for _, l := range h.logs {
		if strings.Contains(l, "虛構成員") || strings.Contains(l, "虛構森林") {
			t.Fatalf("log names a member or a place: %s", l)
		}
	}
}
