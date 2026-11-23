package svdoc

import (
	"encoding/json"
	"strings"
	"testing"
)

// The site matches characters on the Blizzard roster's full name
// ("名字 + U+0020 + 姓氏"). uploader.fullName passes through untouched on
// every kind; subject.fullName is kept on member.report and member.share,
// exactly as written, and dropped alone when it is not a 1-64 character
// string. Made-up names.
func TestFullNameKept(t *testing.T) {
	const up = `game = { world = "forever" }, uploader = { guild = "綠皮與獠牙冒險事務所", name = "虛構甲", fullName = "虛構甲 虛構姓" }`
	long := strings.Repeat("名", MaxSubjectLen+1)
	d := parseOne(t, `
		{ id = "self", kind = "character.self", capturedAt = 1, `+up+`, payload = { level = 1 } },
		{ id = "roster", kind = "roster.snapshot", capturedAt = 1, `+up+`, payload = { total = 0, members = {} } },
		{ id = "rep", kind = "member.report", capturedAt = 1, `+up+`,
		  payload = { subject = { name = "虛構乙", fullName = "虛構乙  虛構 姓 " } } },
		{ id = "share", kind = "member.share", capturedAt = 1, `+up+`,
		  payload = { subject = { name = "虛構丙", realm = "虛構伺服器", fullName = "虛構丙 虛構姓" } } },
		{ id = "long", kind = "member.share", capturedAt = 1, `+up+`,
		  payload = { subject = { name = "虛構丁", fullName = "`+long+`" } } },
		{ id = "num", kind = "member.report", capturedAt = 1, `+up+`,
		  payload = { subject = { name = "虛構戊", fullName = 5 } } },
		{ id = "empty", kind = "member.report", capturedAt = 1, `+up+`,
		  payload = { subject = { name = "虛構己", fullName = "" } } },`)
	if len(d.Entries) != 7 || d.Skipped != 0 {
		t.Fatalf("entries %d skipped %d", len(d.Entries), d.Skipped)
	}
	wantSubject := map[string]string{
		"rep":   `{"fullName":"虛構乙  虛構 姓 ","name":"虛構乙"}`,
		"share": `{"fullName":"虛構丙 虛構姓","name":"虛構丙","realm":"虛構伺服器"}`,
		"long":  `{"name":"虛構丁"}`,
		"num":   `{"name":"虛構戊"}`,
		"empty": `{"name":"虛構己"}`,
	}
	for _, e := range d.Entries {
		var m struct {
			Uploader map[string]any `json:"uploader"`
		}
		if err := json.Unmarshal(e.JSON, &m); err != nil {
			t.Fatal(err)
		}
		if got := m.Uploader["fullName"]; got != "虛構甲 虛構姓" {
			t.Errorf("%s: uploader.fullName = %v", e.ID, got)
		}
		if w, ok := wantSubject[e.ID]; ok {
			if got := asJSON(payloadOf(t, e)["subject"]); got != w {
				t.Errorf("%s: subject\n got %s\nwant %s", e.ID, got, w)
			}
		}
	}
}
