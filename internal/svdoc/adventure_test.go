package svdoc

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// A valid adventure table (savedvariables §adventure example), made-up
// GUIDs and text.
const goodAdventure = `adventure = {
  tickets = { recordedAt = 1763373590, value = {
    ["a3k9.mabc12"] = {
      src = "g", ver = 1, created = 1763370000, dungeon = "sfk", quests = { 1013, 1014 },
      time = { mode = "future", from = 1763380800, to = 1763388000 },
      slots = { tank = 1, healer = 1, dps = 3 }, tags = { "quest", "repeat" },
      note = "七點集合，語音頻道二", admit = "consent", status = "open", statusAt = 1763373590,
      decisions = { ["Player-0000-0000000B"] = { d = "approve", at = 1763372000 } },
    },
    ["a3k9.q1"] = { src = "g", created = 1763370001, quests = { 1 }, time = { mode = "now" },
      slots = { size = 4 }, admit = "direct", status = "departed", statusAt = 1763373000 },
  } },
  worker = { recordedAt = 1763373590, value = {
    dungeons = { "sfk", "rfk" }, quests = { 1013 }, allQuests = false, main = "healer", alt = "dps",
    times = { { mode = "now" }, { mode = "today", from = 1, to = 2 } }, expires = 1763456390 } },
  actions = { recordedAt = 1763373590, value = {
    ["a3k9.mabc12"] = { a = "apply", at = 1763372000, firstAt = 1763371000, role = "healer" },
  } },
}`

const goodAdventureJSON = `{"actions":{"recordedAt":1763373590,"value":{"a3k9.mabc12":{"a":"apply","at":1763372000,"firstAt":1763371000,"role":"healer"}}},` +
	`"tickets":{"recordedAt":1763373590,"value":{` +
	`"a3k9.mabc12":{"admit":"consent","created":1763370000,"decisions":{"Player-0000-0000000B":{"at":1763372000,"d":"approve"}},"dungeon":"sfk","note":"七點集合，語音頻道二","quests":[1013,1014],"slots":{"dps":3,"healer":1,"tank":1},"src":"g","status":"open","statusAt":1763373590,"tags":["quest","repeat"],"time":{"from":1763380800,"mode":"future","to":1763388000},"ver":1},` +
	`"a3k9.q1":{"admit":"direct","created":1763370001,"quests":[1],"slots":{"size":4},"src":"g","status":"departed","statusAt":1763373000,"time":{"mode":"now"}}}},` +
	`"worker":{"recordedAt":1763373590,"value":{"allQuests":false,"alt":"dps","dungeons":["sfk","rfk"],"expires":1763456390,"main":"healer","quests":[1013],"times":[{"mode":"now"},{"from":1,"mode":"today","to":2}]}}}`

func adventureOf(t *testing.T, entry string) (any, bool) {
	t.Helper()
	d := parseOne(t, entry)
	if len(d.Entries) != 1 {
		t.Fatalf("entries %d skipped %d", len(d.Entries), d.Skipped)
	}
	a, ok := payloadOf(t, d.Entries[0])["adventure"]
	return a, ok
}

func bothKinds(adv string) map[string]string {
	return map[string]string{"member.share": shareWith(adv), "character.self": selfWith(adv)}
}

func TestAdventureValidKeptAsWritten(t *testing.T) {
	for name, entry := range bothKinds(goodAdventure) {
		a, _ := adventureOf(t, entry)
		if got := asJSON(a); got != goodAdventureJSON {
			t.Errorf("%s:\n got %s\nwant %s", name, got, goodAdventureJSON)
		}
	}
}

func TestAdventureStripsTextAndBadItems(t *testing.T) {
	bad := `adventure = {
	  leader = "虛構甲", extra = { x = 1 },
	  tickets = { recordedAt = 10, note = "不該上傳", value = {
	    ["a1"] = { created = 5, status = "open", leaderName = "虛構甲", channel = "不該上傳",
	               tags = { "quest", "不該上傳", "voice", 3, "quest" },
	               decisions = {
	                 ["Player-0000-0000000B"] = { d = "kick", at = 7, name = "虛構乙" },
	                 ["虛構丙"] = { d = "approve", at = 8 },                    -- name, not GUID
	                 ["Player-0000-0000000C"] = { d = "maybe", at = 8 },        -- unknown decision
	                 ["Player-0000-0000000D"] = { d = "invite", at = -1 },      -- negative
	               } },
	    ["a2"] = { dungeon = "影牙城堡" },                                   -- name, not a code
	    ["a3"] = { dungeon = "averyveryverylongcode" },                      -- over 16
	    ["a4"] = { quests = { 1, 2, 3, 4, 5, 6 } },                          -- over 5
	    ["a5"] = { quests = { 0 } },                                         -- not positive
	    ["a6"] = { status = "full" },                                        -- not written by the AddOn
	    ["a7"] = { time = { mode = "soon" } },
	    ["a8"] = { slots = { tank = "一" } },
	    ["A9"] = { created = 1 },                                            -- id: upper case
	    ["a10"] = "不該上傳",
	    ["a11"] = { note = 5, created = 6, status = "open" },                -- bad note: ticket kept
	  } },
	  worker = { recordedAt = 11, value = { main = "healer", nickname = "虛構甲" } },
	  actions = { recordedAt = 12, value = {
	    ["a1"] = { a = "apply", at = 9, role = "healer", name = "虛構乙" },
	    ["a2"] = { a = "cheer", at = 9 },
	    ["a3"] = { a = "apply", at = "昨天" },
	    ["a4"] = { a = "accept", role = "boss" },
	    ["bad id"] = { a = "apply", at = 1 },
	  } },
	}`
	want := `{"actions":{"recordedAt":12,"value":{"a1":{"a":"apply","at":9,"role":"healer"}}},` +
		`"tickets":{"recordedAt":10,"value":{"a1":{"created":5,"decisions":{"Player-0000-0000000B":{"at":7,"d":"kick"}},"status":"open","tags":["quest","voice"]},` +
		`"a11":{"created":6,"status":"open"}}},` +
		`"worker":{"recordedAt":11,"value":{"main":"healer"}}}`
	for name, entry := range bothKinds(bad) {
		a, _ := adventureOf(t, entry)
		if got := asJSON(a); got != want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, want)
		}
		if s := asJSON(a); strings.Contains(s, "不該上傳") || strings.Contains(s, "虛構") {
			t.Errorf("%s: text leaked: %s", name, s)
		}
	}
	// A worker ticket with one bad field is dropped (there is only one).
	a, _ := adventureOf(t, selfWith(`adventure = { worker = { recordedAt = 1, value = { main = "healer", times = { { mode = "now" }, { mode = "x" } } } } }`))
	if a != nil {
		t.Errorf("bad worker kept: %s", asJSON(a))
	}
}

func TestAdventureNote(t *testing.T) {
	cases := []struct{ lua, want string }{
		{`"第一行\n第二行\r\n第三行"`, "第一行第二行第三行"},
		{`"tab\there\0\7bell\27[31m"`, "tabherebell[31m"},
		{`"bidi \xE2\x80\xAEevil"`, "bidi evil"},                       // U+202E right-to-left override
		{`"cut \xE6\xB8"`, "cut"},                                      // broken UTF-8 at the end
		{`"` + strings.Repeat("長", 45) + `"`, strings.Repeat("長", 40)}, // 40 characters, not bytes
		{`"  <b>html</b>  "`, "<b>html</b>"},                           // kept as text; the site escapes it
	}
	for _, c := range cases {
		a, _ := adventureOf(t, shareWith(`adventure = { tickets = { recordedAt = 1, value = { ["a1"] = { created = 1, note = `+c.lua+` } } } }`))
		got, _ := a.(map[string]any)["tickets"].(map[string]any)["value"].(map[string]any)["a1"].(map[string]any)["note"].(string)
		if got != c.want {
			t.Errorf("note %s = %q, want %q", c.lua, got, c.want)
		}
		if utf8.RuneCountInString(got) > MaxNoteRunes || !utf8.ValidString(got) {
			t.Errorf("note %q too long or broken", got)
		}
	}
	// Only control characters: no note, ticket kept.
	a, _ := adventureOf(t, shareWith(`adventure = { tickets = { recordedAt = 1, value = { ["a1"] = { created = 1, note = "\n\t\r" } } } }`))
	if got := asJSON(a); got != `{"tickets":{"recordedAt":1,"value":{"a1":{"created":1}}}}` {
		t.Errorf("empty note: %s", got)
	}
}

func TestAdventureLimits(t *testing.T) {
	var b strings.Builder
	b.WriteString(`adventure = { tickets = { recordedAt = 1, value = {`)
	// 3 open tickets and 20 ended ones (statusAt 1..20): the 3 that ended
	// longest ago go.
	for i := 1; i <= 3; i++ {
		fmt.Fprintf(&b, `["o%d"] = { created = %d, status = "open" }, `, i, 100+i)
	}
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&b, `["e%d"] = { created = 1, status = "done", statusAt = %d`, i, i)
		if i == 20 {
			b.WriteString(`, decisions = {`)
			for d := 1; d <= MaxAdventureDecisions+2; d++ {
				fmt.Fprintf(&b, `["Player-0000-%08X"] = { d = "approve", at = %d }, `, d, d)
			}
			b.WriteString(`}`)
		}
		b.WriteString(` }, `)
	}
	b.WriteString(`} }, actions = { recordedAt = 1, value = {`)
	for i := 1; i <= MaxAdventureActions+3; i++ {
		fmt.Fprintf(&b, `["a%d"] = { a = "apply", at = %d }, `, i, i)
	}
	b.WriteString(`} } }`)

	a, _ := adventureOf(t, selfWith(b.String()))
	m := a.(map[string]any)
	tickets := m["tickets"].(map[string]any)["value"].(map[string]any)
	if len(tickets) != MaxAdventureTickets {
		t.Fatalf("tickets: %d", len(tickets))
	}
	for _, id := range []string{"o1", "o2", "o3", "e4", "e20"} {
		if tickets[id] == nil {
			t.Errorf("%s dropped", id)
		}
	}
	for _, id := range []string{"e1", "e2", "e3"} {
		if tickets[id] != nil {
			t.Errorf("%s (ended longest ago) kept", id)
		}
	}
	dec := tickets["e20"].(map[string]any)["decisions"].(map[string]any)
	if len(dec) != MaxAdventureDecisions || dec["Player-0000-00000001"] != nil || dec["Player-0000-0000002A"] == nil {
		t.Errorf("decisions: %d kept", len(dec))
	}
	acts := m["actions"].(map[string]any)["value"].(map[string]any)
	if len(acts) != MaxAdventureActions || acts["a1"] != nil || acts["a53"] == nil {
		t.Errorf("actions: %d kept", len(acts))
	}
}

func TestAdventureNotATable(t *testing.T) {
	for _, adv := range []string{`adventure = "不該上傳"`, `adventure = {}`, `adventure = { tickets = { value = { ["a1"] = { created = 1 } } } }`} {
		for _, entry := range bothKinds(adv) {
			if v, has := adventureOf(t, entry); has {
				t.Errorf("%s: kept %s", adv, asJSON(v))
			}
		}
	}
}
