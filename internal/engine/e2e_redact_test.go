package engine_test

import (
	"strings"
	"testing"

	"gfsync/internal/redact"
)

// The log must not contain WoW account folder names or full character
// GUIDs (players paste gfsync.log when asking for help). Made-up names.
func TestE2ELogMasksAccountNames(t *testing.T) {
	const acct = "SWORDFISH77"
	const broken = "BROKENACCT99"
	const id = "Player-4711-0ABCDEF1:4102444800:character.self"
	h := newHarness(t)
	// Captured "in 2100": the mock rejects it, so the GUID reaches the
	// "upload: … rejected" line.
	h.writeSV(acct, strings.Replace(sv(2, true, id), "1763370000", "4102444800", 1))
	// A second account whose file cannot be parsed: an error line.
	h.writeSV(broken, "this is { not lua")
	h.start()
	h.waitForLog("read " + redact.Account(acct) + ": 1 entries")
	h.waitForLog("read " + redact.Account(broken) + ":")
	h.waitForLog("rejected: captured_in_future")

	h.logMu.Lock()
	defer h.logMu.Unlock()
	sawGUID := false
	for _, l := range h.logs {
		for _, secret := range []string{acct, broken, "0ABCDEF1"} {
			if strings.Contains(l, secret) {
				t.Errorf("log leaks %q: %s", secret, l)
			}
		}
		if strings.Contains(l, "Player-…DEF1") {
			sawGUID = true
		}
	}
	if !sawGUID {
		t.Error("the rejected line should keep the last 4 digits of the GUID")
	}
}
