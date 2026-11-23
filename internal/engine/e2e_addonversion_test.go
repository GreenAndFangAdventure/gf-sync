package engine_test

import (
	"fmt"
	"strings"
	"testing"
)

// outboxVersion is outbox() with a top-level addonVersion ("" = none).
func outboxVersion(version string, entries ...string) string {
	v := ""
	if version != "" {
		v = fmt.Sprintf("\t[\"addonVersion\"] = %q,\n", version)
	}
	return strings.Replace(outbox(entries...), "\t[\"schemaVersion\"] = 2,\n", "\t[\"schemaVersion\"] = 2,\n"+v, 1)
}

// D-017 follow-up: GF Sync sends the file's addonVersion as
// client.addonVersion. A request never mixes entries from AddOn versions,
// and a file without addonVersion sends none.
func TestE2EClientAddonVersion(t *testing.T) {
	files := map[string]string{"A": "0.10.0", "B": "", "C": "0.9.1"}
	want := map[string]string{} // id -> addonVersion it must be sent with
	h := newHarness(t)
	for acct, v := range files {
		var entries []string
		// 60 entries per file: more than one request each, so batches
		// have to be cut by version, not only by size or count.
		for i := 0; i < 60; i++ {
			id := fmt.Sprintf("%s:%d:character.self", acct, i)
			entries = append(entries, entry(id, 1763370000+int64(i)))
			want[id] = v
		}
		h.writeSV(acct, outboxVersion(v, entries...))
	}
	h.start()
	h.waitFor("all 180 accepted", func() bool { return len(h.receivedIDs()) == len(want) })
	for i, r := range h.mock.Requests() {
		for _, id := range r.IDs {
			if want[id] != r.AddonVersion {
				t.Fatalf("request %d sent addonVersion %q with %s (from a file with %q)", i, r.AddonVersion, id, want[id])
			}
		}
	}
	// 3 files x 60 = at least 6 requests (50 + 10 each); mixing would
	// have allowed 4.
	if n := len(h.mock.Requests()); n < 6 {
		t.Errorf("%d requests; versions were mixed", n)
	}
}

// The site decides addon_too_old from client.addonVersion: only the file
// with a new enough AddOn goes up; the others are kept and retried.
func TestE2EMinAddonVersion(t *testing.T) {
	h := newHarness(t)
	h.mock.SetMinAddon("0.10.0")
	h.writeSV("A", outboxVersion("0.10.0", entry("new:1:character.self", 1763370000)))
	h.writeSV("B", outboxVersion("0.9.1", entry("old:1:character.self", 1763370000)))
	h.writeSV("C", outboxVersion("", entry("none:1:character.self", 1763370000)))
	h.start()
	h.waitFor("new AddOn accepted, others told to update", func() bool {
		return h.receivedIDs()["new:1:character.self"] && h.eng.Status().Pending == 0
	})
	h.waitForLog("entries from an AddOn the site finds too old")
	got := h.receivedIDs()
	if got["old:1:character.self"] || got["none:1:character.self"] {
		t.Fatalf("old AddOn data accepted: %v", got)
	}
	for _, id := range []string{"old:1:character.self", "none:1:character.self"} {
		if h.mock.Status(id) != "" {
			t.Errorf("%s: the mock must not remember addon_too_old ids", id)
		}
	}
}
