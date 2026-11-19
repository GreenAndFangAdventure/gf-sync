package engine_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// Decision D-009: the player's own location is uploaded with
// character.self, and never written to the log. Made-up place names.
func TestE2EOwnLocationUploadedNotLogged(t *testing.T) {
	const zone, sub = "虛構森林", "虛構營地"
	h := newHarness(t)
	src := strings.Replace(sv(2, true, "loc:1:character.self"),
		`["zone"] = "不該上傳" }`,
		`["zone"] = "不該上傳", ["location"] = { ["zone"] = "`+zone+`", ["subZone"] = "`+sub+`", ["x"] = 0.5 } }`, 1)
	if !strings.Contains(src, sub) {
		t.Fatal("fixture")
	}
	h.writeSV("A", src)
	h.start()
	h.waitFor("upload", func() bool { return h.receivedIDs()["loc:1:character.self"] })

	var got struct {
		Payload map[string]json.RawMessage
	}
	for _, raw := range h.mock.Received() {
		if strings.Contains(string(raw), "loc:1:") {
			json.Unmarshal(raw, &got)
		}
	}
	if l := string(got.Payload["location"]); l != `{"subZone":"`+sub+`","zone":"`+zone+`"}` {
		t.Fatalf("location = %s", l)
	}
	if _, ok := got.Payload["zone"]; ok {
		t.Fatal("payload.zone must still be removed")
	}
	h.logMu.Lock()
	defer h.logMu.Unlock()
	for _, l := range h.logs {
		if strings.Contains(l, zone) || strings.Contains(l, sub) {
			t.Fatalf("log shows the location: %s", l)
		}
	}
}
