package svdoc

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestContractFixture(t *testing.T) {
	d, err := Parse(fixture(t, "GreenFangToolkit.contract.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != StatusOK || d.AddonVersion != "0.4.0" {
		t.Fatalf("status %v version %q", d.Status, d.AddonVersion)
	}
	if len(d.Entries) != 2 || d.Skipped != 2 {
		t.Fatalf("entries %d skipped %d", len(d.Entries), d.Skipped)
	}
	if d.Entries[0].Kind != "character.self" || d.Entries[1].Kind != "roster.snapshot" {
		t.Fatal("kinds")
	}
	for _, e := range d.Entries {
		s := string(e.JSON)
		if strings.Contains(s, "zone") || strings.Contains(s, "online") || strings.Contains(s, "不該上傳") {
			t.Fatalf("private field leaked: %s", s)
		}
	}
	var m map[string]any
	if err := json.Unmarshal(d.Entries[0].JSON, &m); err != nil {
		t.Fatal(err)
	}
	if m["futureField"] == nil {
		t.Fatal("unknown entry fields are passed through unchanged")
	}
	if m["id"] != d.Entries[0].ID {
		t.Fatal("id mismatch")
	}
	var r map[string]any
	_ = json.Unmarshal(d.Entries[1].JSON, &r)
	members := r["payload"].(map[string]any)["members"].([]any)
	if p := members[1].(map[string]any)["professions"]; p == nil {
		t.Fatal("empty professions should be an empty array")
	} else if arr, ok := p.([]any); !ok || len(arr) != 0 {
		t.Fatalf("professions = %#v", p)
	}
}

// The AddOn's official sample. It lives in the guild's private toolkit repo
// (tests/fixtures/); skip when absent. GFSYNC_ADDON_FIXTURE overrides the path.
func TestAddonSampleFixture(t *testing.T) {
	path := os.Getenv("GFSYNC_ADDON_FIXTURE")
	if path == "" {
		path = filepath.Join("..", "..", "..", "tests", "fixtures", "GreenFangToolkit.sample.lua")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("addon sample not present: %v", err)
	}
	d, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != StatusOK || len(d.Entries) != 3 || d.Skipped != 0 {
		t.Fatalf("status %v entries %d skipped %d", d.Status, len(d.Entries), d.Skipped)
	}
	for _, e := range d.Entries {
		var m map[string]any
		if err := json.Unmarshal(e.JSON, &m); err != nil {
			t.Fatal(err)
		}
		// The one allowed place: character.self payload.location (D-009).
		if p, ok := m["payload"].(map[string]any); ok && e.Kind == "character.self" {
			delete(p, "location")
		}
		b, _ := json.Marshal(m)
		for _, bad := range []string{`"zone"`, `"online"`, `"location"`} {
			if strings.Contains(string(b), bad) {
				t.Fatalf("private field %s in %s", bad, e.ID)
			}
		}
	}
}

func TestStatuses(t *testing.T) {
	cases := []struct {
		src  string
		want Status
	}{
		{`GreenFangToolkitDB = { schemaVersion = 2, sync = { enabled = true, outbox = {} } }`, StatusOK},
		{`GreenFangToolkitDB = { schemaVersion = 1, sync = { enabled = true, outbox = { { id = "a", kind = "character.self", capturedAt = 1 } } } }`, StatusUnsupported},
		{`GreenFangToolkitDB = { schemaVersion = 0, sync = { enabled = true } }`, StatusUnsupported},
		{`GreenFangToolkitDB = { schemaVersion = 2, sync = { enabled = false, outbox = { { id = "a", kind = "character.self", capturedAt = 1 } } } }`, StatusDisabled},
		{`GreenFangToolkitDB = { schemaVersion = 2, sync = { enabled = "true" } }`, StatusDisabled},
		{`GreenFangToolkitDB = { schemaVersion = 2, sync = { enabled = 1 } }`, StatusDisabled},
		{`GreenFangToolkitDB = { schemaVersion = 2 }`, StatusDisabled},
		{`GreenFangToolkitDB = { schemaVersion = 3, sync = { enabled = true } }`, StatusUnsupported},
		{`GreenFangToolkitDB = { schemaVersion = 2.5, sync = { enabled = true } }`, StatusUnsupported},
		{`GreenFangToolkitDB = { schemaVersion = "2", sync = { enabled = true } }`, StatusUnsupported},
		{`GreenFangToolkitDB = { sync = { enabled = true } }`, StatusUnsupported},
		{`GreenFangToolkitDB = "oops"`, StatusNoTable},
		{`SomethingElseDB = {}`, StatusNoTable},
		{``, StatusNoTable},
	}
	for _, c := range cases {
		d, err := Parse([]byte(c.src))
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		if d.Status != c.want {
			t.Errorf("%s: got %v want %v", c.src, d.Status, c.want)
		}
		if d.Status != StatusOK && len(d.Entries) != 0 {
			t.Errorf("%s: entries must be empty when not OK", c.src)
		}
	}
}

func TestBadEntries(t *testing.T) {
	src := `GreenFangToolkitDB = { schemaVersion = 2, sync = { enabled = true, outbox = {
		"not a table",
		{ kind = "character.self", capturedAt = 1 },
		{ id = "", kind = "character.self", capturedAt = 1 },
		{ id = "x", kind = "character.self" },
		{ id = "x", kind = "character.self", capturedAt = "1" },
		{ id = "x", capturedAt = 1 },
		{ id = 5, kind = "character.self", capturedAt = 1 },
		{ id = "` + strings.Repeat("a", 300) + `", kind = "character.self", capturedAt = 1 },
		{ id = "good", kind = "character.self", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" } },
	} } }`
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Entries) != 1 || d.Entries[0].ID != "good" || d.Skipped != 8 {
		t.Fatalf("entries %d skipped %d", len(d.Entries), d.Skipped)
	}
}

func TestMaliciousFile(t *testing.T) {
	for _, src := range []string{
		`GreenFangToolkitDB = { schemaVersion = 2, sync = { enabled = os.execute("calc") } }`,
		`GreenFangToolkitDB = { schemaVersion = 2 } os.exit()`,
		`GreenFangToolkitDB = { schemaVersion = 2, sync = { enabled = true, outbox = {`,
	} {
		if _, err := Parse([]byte(src)); !errors.Is(err, ErrParse) {
			t.Errorf("%s: want ErrParse, got %v", src, err)
		}
	}
}

// Entries from other guilds (or without a guild) never leave the computer,
// whatever an old or edited AddOn wrote. Made-up guild names.
func TestOnlyOurGuild(t *testing.T) {
	src := `GreenFangToolkitDB = { schemaVersion = 2, sync = { enabled = true, outbox = {
		{ id = "ours", kind = "character.self", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" } },
		{ id = "other", kind = "character.self", capturedAt = 1, uploader = { guild = "虛構的別家公會" } },
		{ id = "near", kind = "roster.snapshot", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所 " } },
		{ id = "empty", kind = "character.self", capturedAt = 1, uploader = { guild = "" } },
		{ id = "noguild", kind = "character.self", capturedAt = 1, uploader = { name = "虛構" } },
		{ id = "nouploader", kind = "character.self", capturedAt = 1 },
		{ id = "notstring", kind = "character.self", capturedAt = 1, uploader = { guild = 1 } },
		{ id = "badkind", kind = "nope", capturedAt = 1, uploader = { guild = "虛構的別家公會" } },
	} } }`
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Entries) != 1 || d.Entries[0].ID != "ours" {
		t.Fatalf("entries %+v", d.Entries)
	}
	if d.OtherGuild != 6 || d.Skipped != 1 {
		t.Fatalf("otherGuild %d skipped %d", d.OtherGuild, d.Skipped)
	}
	for _, e := range d.Entries {
		if strings.Contains(string(e.JSON), "虛構的別家公會") {
			t.Fatal("other guild in upload")
		}
	}
}

// Decision D-009: only character.self may carry the player's own location,
// and only as {zone, subZone} strings. Made-up place names.
func TestOwnLocationOnly(t *testing.T) {
	long := strings.Repeat("地", MaxLocationLen+1)
	ok60 := strings.Repeat("地", MaxLocationLen)
	src := `GreenFangToolkitDB = { schemaVersion = 2, sync = { enabled = true, outbox = {
		{ id = "full", kind = "character.self", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" },
		  zone = "頂層不該上傳", online = true, location = { zone = "頂層不該上傳" },
		  payload = { level = 10, zone = "不該上傳", online = true,
		    location = { zone = "虛構森林", subZone = "虛構營地", x = 0.5, y = 0.25, mapID = 1, online = true } } },
		{ id = "zoneonly", kind = "character.self", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" },
		  payload = { location = { zone = "` + ok60 + `", subZone = "` + long + `" } } },
		{ id = "nozone", kind = "character.self", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" },
		  payload = { location = { subZone = "虛構營地" } } },
		{ id = "toolong", kind = "character.self", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" },
		  payload = { location = { zone = "` + long + `", subZone = "虛構營地" } } },
		{ id = "notstring", kind = "character.self", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" },
		  payload = { location = { zone = 5, subZone = { "虛構營地" } } } },
		{ id = "empty", kind = "character.self", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" },
		  payload = { location = { zone = "", subZone = "" } } },
		{ id = "flat", kind = "character.self", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" },
		  payload = { location = "虛構森林" } },
		{ id = "roster", kind = "roster.snapshot", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" },
		  payload = { total = 1, location = { zone = "虛構森林" },
		    members = { { name = "虛構甲", zone = "虛構森林", online = true, location = { zone = "虛構森林" } } } } },
	} } }`
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]map[string]any{}
	for _, e := range d.Entries {
		var m map[string]any
		if err := json.Unmarshal(e.JSON, &m); err != nil {
			t.Fatal(err)
		}
		got[e.ID] = m
	}
	if len(got) != 8 {
		t.Fatalf("entries %d", len(got))
	}
	loc := func(id string) any { return got[id]["payload"].(map[string]any)["location"] }

	full := got["full"]
	for _, k := range []string{"zone", "online", "location"} {
		if _, ok := full[k]; ok {
			t.Errorf("entry-level %s kept", k)
		}
	}
	p := full["payload"].(map[string]any)
	if _, ok := p["zone"]; ok {
		t.Error("payload.zone kept")
	}
	if _, ok := p["online"]; ok {
		t.Error("payload.online kept")
	}
	if l, _ := json.Marshal(loc("full")); string(l) != `{"subZone":"虛構營地","zone":"虛構森林"}` {
		t.Errorf("location = %s (only zone and subZone may stay)", l)
	}
	if l, _ := json.Marshal(loc("zoneonly")); string(l) != `{"zone":"`+ok60+`"}` {
		t.Errorf("a subZone over the limit is dropped alone: %s", l)
	}
	for _, id := range []string{"nozone", "toolong", "notstring", "empty", "flat"} {
		if l := loc(id); l != nil {
			t.Errorf("%s: location kept: %v", id, l)
		}
	}
	rs, _ := json.Marshal(got["roster"])
	for _, bad := range []string{`"location"`, `"zone"`, `"online"`} {
		if strings.Contains(string(rs), bad) {
			t.Errorf("roster.snapshot keeps %s: %s", bad, rs)
		}
	}
	for id, m := range got {
		if b, _ := json.Marshal(m); strings.Contains(string(b), "不該上傳") {
			t.Errorf("%s: zone leaked: %s", id, b)
		}
	}
}

// Decision D-011: member.report is data another member shared in game.
// The subject keeps only name, realm and guid; the location follows the
// character.self rule; the guild filter applies to the uploader.
// Made-up names.
func TestMemberReport(t *testing.T) {
	src := `GreenFangToolkitDB = { schemaVersion = 2, sync = { enabled = true, outbox = {
		{ id = "r1", kind = "member.report", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" },
		  payload = { subject = { name = "虛構乙", realm = "虛構伺服器", guid = "Player-1-00000B0B", note = "不該上傳", online = true },
		    reportedAt = 2, level = 60, classToken = "MAGE", zone = "不該上傳", online = true,
		    professions = { { name = "裁縫", skillLineID = 197, rank = 300, max = 300 } },
		    location = { zone = "虛構森林", subZone = "虛構營地", x = 1 } } },
		{ id = "r2", kind = "member.report", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" },
		  payload = { subject = { name = "虛構丙", realm = 5 }, level = 10 } },
		{ id = "noname", kind = "member.report", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" },
		  payload = { subject = { realm = "虛構伺服器" } } },
		{ id = "nosubject", kind = "member.report", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" },
		  payload = { level = 10 } },
		{ id = "longname", kind = "member.report", capturedAt = 1, uploader = { guild = "綠皮與獠牙冒險事務所" },
		  payload = { subject = { name = "` + strings.Repeat("名", MaxSubjectLen+1) + `" } } },
		{ id = "other", kind = "member.report", capturedAt = 1, uploader = { guild = "虛構的別家公會" },
		  payload = { subject = { name = "虛構丁" } } },
	} } }`
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Entries) != 2 || d.Skipped != 3 || d.OtherGuild != 1 {
		t.Fatalf("entries %d skipped %d otherGuild %d", len(d.Entries), d.Skipped, d.OtherGuild)
	}
	var r1, r2 struct {
		Payload map[string]json.RawMessage
	}
	json.Unmarshal(d.Entries[0].JSON, &r1)
	json.Unmarshal(d.Entries[1].JSON, &r2)
	if d.Entries[0].Kind != KindMemberReport || d.Entries[0].ID != "r1" {
		t.Fatalf("kind %q id %q", d.Entries[0].Kind, d.Entries[0].ID)
	}
	if s := string(r1.Payload["subject"]); s != `{"guid":"Player-1-00000B0B","name":"虛構乙","realm":"虛構伺服器"}` {
		t.Errorf("subject = %s", s)
	}
	if s := string(r1.Payload["location"]); s != `{"subZone":"虛構營地","zone":"虛構森林"}` {
		t.Errorf("location = %s", s)
	}
	for _, k := range []string{"zone", "online"} {
		if _, ok := r1.Payload[k]; ok {
			t.Errorf("payload.%s kept", k)
		}
	}
	for _, k := range []string{"reportedAt", "level", "classToken", "professions"} {
		if _, ok := r1.Payload[k]; !ok {
			t.Errorf("payload.%s missing", k)
		}
	}
	if strings.Contains(string(d.Entries[0].JSON), "不該上傳") {
		t.Errorf("private data leaked: %s", d.Entries[0].JSON)
	}
	if s := string(r2.Payload["subject"]); s != `{"name":"虛構丙"}` {
		t.Errorf("a non-string realm is dropped alone: %s", s)
	}
}
