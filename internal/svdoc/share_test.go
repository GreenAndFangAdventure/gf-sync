package svdoc

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const ourUploader = `game = { world = "forever" }, uploader = { guild = "綠皮與獠牙冒險事務所" }`

func parseOne(t *testing.T, entries string) *Doc {
	t.Helper()
	d, err := Parse([]byte(`GreenFangToolkitDB = { schemaVersion = 2, sync = { enabled = true, outbox = {` + entries + `} } }`))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func payloadOf(t *testing.T, e Entry) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(e.JSON, &m); err != nil {
		t.Fatal(err)
	}
	return m["payload"].(map[string]any)
}

func asJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

// savedvariables §3.8: member.share is rebuilt from a whitelist; anything
// else the AddOn wrote never leaves the computer. Made-up names.
func TestMemberShareWhitelist(t *testing.T) {
	d := parseOne(t, `
		{ id = "s1", kind = "member.share", capturedAt = 1, capturedAtServer = 5, `+ourUploader+`, zone = "不該上傳",
		  payload = {
		    subject = { guid = "Player-1-0000000B", name = "虛構乙-虛構伺服器", realm = "虛構伺服器", note = "不該上傳" },
		    level = { value = 61, recordedAt = 100, via = "author", extra = "不該上傳" },
		    classToken = { value = "WARRIOR", recordedAt = 100, via = "somebody" },
		    professions = { recordedAt = 100, via = "relay",
		      value = { { skillLineID = 186, name = "採礦", rank = 300, max = 300, secret = "不該上傳" }, "bad", { name = "沒有 id" } } },
		    location = { recordedAt = 90, via = "relay", x = 1,
		      value = { zone = "虛構森林", subZone = "虛構營地", x = 0.5 } },
		    recipes = {
		      ["164"] = { recordedAt = 80, via = "relay", hash = "3fz9q", ids = { 2660, 2661, "x" }, names = "不該上傳" },
		      ["171"] = { recordedAt = 81, via = "relay", hash = "1k2ab" },
		      ["notid"] = { recordedAt = 1, ids = { 1 } },
		      ["202"] = { via = "relay", ids = { 1 } },
		    },
		    withdrawn = { all = 70, location = 71, recipes = { ["202"] = 72, ["x"] = 1 }, zone = "不該上傳" },
		    zone = "不該上傳", online = true, members = { { zone = "不該上傳" } }, note = "不該上傳",
		  } },`)
	if len(d.Entries) != 1 || d.Entries[0].Kind != KindMemberShare {
		t.Fatalf("entries %+v skipped %d", d.Entries, d.Skipped)
	}
	p := payloadOf(t, d.Entries[0])
	want := map[string]string{
		"subject":     `{"guid":"Player-1-0000000B","name":"虛構乙-虛構伺服器","realm":"虛構伺服器"}`,
		"level":       `{"recordedAt":100,"value":61,"via":"author"}`,
		"classToken":  `{"recordedAt":100,"value":"WARRIOR"}`,
		"professions": `{"recordedAt":100,"value":[{"max":300,"name":"採礦","rank":300,"skillLineID":186}],"via":"relay"}`,
		"location":    `{"recordedAt":90,"value":{"subZone":"虛構營地","zone":"虛構森林"},"via":"relay"}`,
		"recipes":     `{"164":{"hash":"3fz9q","ids":[2660,2661],"recordedAt":80,"via":"relay"},"171":{"hash":"1k2ab","recordedAt":81,"via":"relay"}}`,
		"withdrawn":   `{"all":70,"location":71,"recipes":{"202":72}}`,
	}
	for k, w := range want {
		if got := asJSON(p[k]); got != w {
			t.Errorf("%s:\n got %s\nwant %s", k, got, w)
		}
	}
	if len(p) != len(want) {
		t.Errorf("extra payload keys: %s", asJSON(p))
	}
	if s := string(d.Entries[0].JSON); strings.Contains(s, "不該上傳") || strings.Contains(s, `"online"`) {
		t.Errorf("leaked: %s", s)
	}
	var top map[string]any
	json.Unmarshal(d.Entries[0].JSON, &top)
	if top["capturedAtServer"] != float64(5) {
		t.Errorf("capturedAtServer = %v", top["capturedAtServer"])
	}
}

func TestMemberShareFields(t *testing.T) {
	d := parseOne(t, `
		{ id = "bad-fields", kind = "member.share", capturedAt = 1, `+ourUploader+`,
		  payload = { subject = { name = "虛構丙" },
		    level = { value = "60", recordedAt = 1 },
		    classToken = { value = "MAGE" },
		    location = { recordedAt = 1, value = { subZone = "沒有 zone" } },
		    professions = { recordedAt = 1, value = "bad" } } },
		{ id = "empty-prof", kind = "member.share", capturedAt = 1, `+ourUploader+`,
		  payload = { subject = { name = "虛構丁" }, professions = { recordedAt = 1, value = {} } } },
		{ id = "no-name", kind = "member.share", capturedAt = 1, `+ourUploader+`,
		  payload = { subject = { guid = "Player-1-0000000C" }, level = { value = 1, recordedAt = 1 } } },
		{ id = "no-payload", kind = "member.share", capturedAt = 1, `+ourUploader+` },
		{ id = "other", kind = "member.share", capturedAt = 1, game = { world = "forever" }, uploader = { guild = "虛構的別家公會" },
		  payload = { subject = { name = "虛構戊" } } },`)
	if len(d.Entries) != 2 || d.Skipped != 2 || d.OtherGuild != 1 {
		t.Fatalf("entries %d skipped %d otherGuild %d", len(d.Entries), d.Skipped, d.OtherGuild)
	}
	if p := payloadOf(t, d.Entries[0]); len(p) != 1 {
		t.Errorf("only the subject is valid: %s", asJSON(p))
	}
	if p := payloadOf(t, d.Entries[1]); asJSON(p["professions"]) != `{"recordedAt":1,"value":[]}` {
		t.Errorf("an empty profession list is a value: %s", asJSON(p))
	}
}

// character.self.recipes and recipe.names are passed through (§3.10);
// recipe.names needs the common fields like every kind (§2a.1).
func TestRecipesAndRecipeNames(t *testing.T) {
	d := parseOne(t, `
		{ id = "self", kind = "character.self", capturedAt = 1, `+ourUploader+`,
		  payload = { level = 10, zone = "不該上傳",
		    recipes = { ["171"] = { recordedAt = 9, hash = "3fz9q", ids = { 2329, 2330, 3170 } } } } },
		{ id = "names", kind = "recipe.names", capturedAt = 1, capturedAtServer = "oops", `+ourUploader+`,
		  payload = { locale = "zhTW", names = { ["2660"] = "粗製磨刀石", ["2661"] = "虛構配方" }, zone = "不該上傳" } },
		{ id = "names-bare", kind = "recipe.names", payload = { locale = "zhTW", names = { ["1"] = "x" } } },
		{ id = "names-noguild", kind = "recipe.names", capturedAt = 1, payload = { locale = "zhTW", names = { ["1"] = "x" } } },`)
	if len(d.Entries) != 2 || d.Skipped != 1 || d.OtherGuild != 1 {
		t.Fatalf("entries %d skipped %d otherGuild %d", len(d.Entries), d.Skipped, d.OtherGuild)
	}
	if r := asJSON(payloadOf(t, d.Entries[0])["recipes"]); r != `{"171":{"hash":"3fz9q","ids":[2329,2330,3170],"recordedAt":9}}` {
		t.Errorf("recipes = %s", r)
	}
	n := payloadOf(t, d.Entries[1])
	if asJSON(n) != `{"locale":"zhTW","names":{"2660":"粗製磨刀石","2661":"虛構配方"}}` {
		t.Errorf("recipe.names payload = %s", asJSON(n))
	}
	var top map[string]any
	json.Unmarshal(d.Entries[1].JSON, &top)
	if _, ok := top["capturedAtServer"]; ok {
		t.Error("a non-numeric capturedAtServer is dropped alone")
	}
	for _, e := range d.Entries {
		if strings.Contains(string(e.JSON), "不該上傳") {
			t.Errorf("%s leaked: %s", e.ID, e.JSON)
		}
	}
}

// EVAL-1000: every member.share the AddOn writes is taken up to
// MaxMemberShare; above it only the newest are kept and the rest counted.
func TestMemberShareCeiling(t *testing.T) {
	build := func(n int) string {
		var b strings.Builder
		b.WriteString(`{ id = "self", kind = "character.self", capturedAt = 1, ` + ourUploader + `, payload = { level = 1 } },`)
		for i := 0; i < n; i++ {
			// capturedAt runs backwards, so the oldest are at the end.
			fmt.Fprintf(&b, `{ id = "s%d", kind = "member.share", capturedAt = %d, `+ourUploader+`, payload = { subject = { name = "虛構%d" } } },`, i, 100000-i, i)
		}
		return b.String()
	}
	for _, n := range []int{150, 1000, MaxMemberShare} {
		d := parseOne(t, build(n))
		if len(d.Entries) != n+1 || d.OverLimit != 0 {
			t.Errorf("%d shares: entries %d overLimit %d", n, len(d.Entries), d.OverLimit)
		}
	}
	d := parseOne(t, build(MaxMemberShare+5))
	if len(d.Entries) != MaxMemberShare+1 || d.OverLimit != 5 {
		t.Fatalf("entries %d overLimit %d", len(d.Entries), d.OverLimit)
	}
	if d.Entries[0].ID != "self" {
		t.Errorf("other kinds are never capped or reordered: first is %s", d.Entries[0].ID)
	}
	for _, e := range d.Entries {
		var i int
		if _, err := fmt.Sscanf(e.ID, "s%d", &i); err == nil && i >= MaxMemberShare {
			t.Errorf("kept an older share %s", e.ID)
		}
	}
}

// D-017 follow-up: every entry carries its file's addonVersion so GF Sync
// can send it as client.addonVersion; anything that is not a short
// version string is dropped.
func TestAddonVersionOnEntries(t *testing.T) {
	for _, c := range []struct{ raw, want string }{
		{`addonVersion = "0.10.0",`, "0.10.0"},
		{`addonVersion = "0.10.0-beta1",`, "0.10.0-beta1"},
		{``, ""},
		{`addonVersion = 10,`, ""},
		{`addonVersion = "0.10 虛構",`, ""},
		{`addonVersion = "` + strings.Repeat("9", 40) + `",`, ""},
	} {
		d, err := Parse([]byte(`GreenFangToolkitDB = { schemaVersion = 2, ` + c.raw + ` sync = { enabled = true, outbox = {
			{ id = "self", kind = "character.self", capturedAt = 1, ` + ourUploader + `, payload = { level = 1 } } } } }`))
		if err != nil {
			t.Fatal(err)
		}
		if d.AddonVersion != c.want || len(d.Entries) != 1 || d.Entries[0].AddonVersion != c.want {
			t.Errorf("%q: doc %q entries %+v", c.raw, d.AddonVersion, d.Entries)
		}
	}
}
