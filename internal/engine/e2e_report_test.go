package engine_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gfsync/internal/mockapi"
)

func report(id string, capturedAt int64, subject, zone string) string {
	return fmt.Sprintf(`
			{
				["id"] = %q,
				["kind"] = "member.report",
				["capturedAt"] = %d,
				["game"] = { ["world"] = "forever", ["interface"] = 16001, ["realm"] = "虛構伺服器", ["locale"] = "zhTW" },
				["uploader"] = { ["guid"] = "Player-1-1", ["name"] = "虛構", ["realm"] = "虛構伺服器", ["faction"] = "Horde", ["guild"] = %q },
				["payload"] = {
					["subject"] = { ["name"] = %q, ["realm"] = "虛構伺服器", ["guid"] = "Player-1-0000BEEF", ["extra"] = "不該上傳" },
					["reportedAt"] = %d, ["level"] = 60, ["classToken"] = "MAGE",
					["professions"] = { { ["name"] = "裁縫", ["skillLineID"] = 197, ["rank"] = 300, ["max"] = 300 } },
					["location"] = { ["zone"] = %q },
				},
			},`, id, capturedAt, mockapi.Guild, subject, capturedAt, zone)
}

// Decision D-011: member reports are uploaded with only the allowed
// fields, and the log never names the member or the location. Made-up
// names.
func TestE2EMemberReport(t *testing.T) {
	const subject, zone = "虛構乙", "虛構森林"
	// The second id is in an unexpected shape that includes the name, and
	// is captured "in 2100", so the mock rejects it and the id reaches the
	// "rejected" log line.
	const good, odd = "Player-1-0000BEEF:1763370000:member.report", subject + ":4102444800:member.report"
	h := newHarness(t)
	h.writeSV("A", "GreenFangToolkitDB = {\n\t[\"schemaVersion\"] = 2,\n\t[\"sync\"] = {\n\t\t[\"enabled\"] = true,\n\t\t[\"outbox\"] = {"+
		report(good, 1763370000, subject, zone)+report(odd, 4102444800, subject, zone)+"\n\t\t},\n\t},\n}\n")
	h.start()
	h.waitFor("upload", func() bool { return h.receivedIDs()[good] })
	h.waitForLog("rejected: captured_in_future")

	var got struct {
		Payload map[string]json.RawMessage
	}
	for _, raw := range h.mock.Received() {
		if strings.Contains(string(raw), good) {
			json.Unmarshal(raw, &got)
		}
	}
	if s := string(got.Payload["subject"]); s != `{"guid":"Player-1-0000BEEF","name":"`+subject+`","realm":"虛構伺服器"}` {
		t.Fatalf("subject = %s", s)
	}
	if s := string(got.Payload["location"]); s != `{"zone":"`+zone+`"}` {
		t.Fatalf("location = %s", s)
	}
	h.logMu.Lock()
	defer h.logMu.Unlock()
	for _, l := range h.logs {
		for _, secret := range []string{subject, zone, "0000BEEF"} {
			if strings.Contains(l, secret) {
				t.Errorf("log leaks %q: %s", secret, l)
			}
		}
	}
}
