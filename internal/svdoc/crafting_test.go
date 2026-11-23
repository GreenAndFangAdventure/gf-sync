package svdoc

import (
	"fmt"
	"strings"
	"testing"
)

// A valid crafting table (savedvariables §crafting example), made-up GUIDs.
const goodCrafting = `crafting = {
  services = { recordedAt = 1763373590, value = {
    ["164"] = { active = true, fee = 10000, recipes = { ["2660"] = 500, ["2661"] = 0 } },
    ["171"] = { active = false },
  } },
  orders = {
    ["g3k9.mabc12"] = {
      w = { recordedAt = 1763373590, src = "g", ver = 1, created = 1763370000,
            customer = "Player-0000-0000000A", crafter = "*", lock = "Player-0000-0000000B", lockedAt = 1763370500, returnedAt = 1763371500,
            recipe = 2660, item = 2862, qty = 5, mats = { ["2835"] = 5 },
            fee = 500, feeQuoted = 500, quote = 120, cst = "open", got = 2,
            close = "deal", closedBy = "customer", closeHow = "trade", closedAt = 1763373590 },
      j = { recordedAt = 1763373000, jst = "delivered", gave = 5,
            acceptedAt = 1763371000, deliveredAt = 1763372500, deliveredHow = "mail", statusAt = 1763372500,
            close = "deal", closedBy = "crafter", closeHow = "timeout", closedAt = 1763373000 },
    },
    ["w7.x"] = { w = { recordedAt = 5, src = "w", customer = "Player-0000-0000000A", crafter = "Player-0000-0000000B",
                       recipe = 1, qty = 1, fee = 0, cst = "review" } },
  },
}`

const goodCraftingJSON = `{"orders":{"g3k9.mabc12":{"j":{"acceptedAt":1763371000,"close":"deal","closeHow":"timeout","closedAt":1763373000,"closedBy":"crafter","deliveredAt":1763372500,"deliveredHow":"mail","gave":5,"jst":"delivered","recordedAt":1763373000,"statusAt":1763372500},` +
	`"w":{"close":"deal","closeHow":"trade","closedAt":1763373590,"closedBy":"customer","crafter":"*","created":1763370000,"cst":"open","customer":"Player-0000-0000000A","fee":500,"feeQuoted":500,"got":2,"item":2862,"lock":"Player-0000-0000000B","lockedAt":1763370500,"mats":{"2835":5},"qty":5,"quote":120,"recipe":2660,"recordedAt":1763373590,"returnedAt":1763371500,"src":"g","ver":1}},` +
	`"w7.x":{"w":{"crafter":"Player-0000-0000000B","cst":"review","customer":"Player-0000-0000000A","fee":0,"qty":1,"recipe":1,"recordedAt":5,"src":"w"}}},` +
	`"services":{"recordedAt":1763373590,"value":{"164":{"active":true,"fee":10000,"recipes":{"2660":500,"2661":0}},"171":{"active":false}}}}`

func shareWith(crafting string) string {
	return `{ id = "s1", kind = "member.share", capturedAt = 1, ` + ourUploader + `,
	  payload = { subject = { guid = "Player-0000-0000000B", name = "虛構乙-虛構伺服器" }, ` + crafting + ` } },`
}

func selfWith(crafting string) string {
	return `{ id = "c1", kind = "character.self", capturedAt = 1, ` + ourUploader + `,
	  payload = { level = 60, ` + crafting + ` } },`
}

func craftingOf(t *testing.T, entries string) (any, bool) {
	t.Helper()
	d := parseOne(t, entries)
	if len(d.Entries) != 1 {
		t.Fatalf("entries %d skipped %d", len(d.Entries), d.Skipped)
	}
	c, ok := payloadOf(t, d.Entries[0])["crafting"]
	return c, ok
}

func TestCraftingValidKeptAsWritten(t *testing.T) {
	for name, entry := range map[string]string{"member.share": shareWith(goodCrafting), "character.self": selfWith(goodCrafting)} {
		c, _ := craftingOf(t, entry)
		if got := asJSON(c); got != goodCraftingJSON {
			t.Errorf("%s:\n got %s\nwant %s", name, got, goodCraftingJSON)
		}
	}
}

// Without a fee key the profession or order has no fee; it must not turn
// into 0 (free).
func TestCraftingMissingFeeStaysMissing(t *testing.T) {
	c, _ := craftingOf(t, shareWith(goodCrafting))
	s := asJSON(c)
	if !strings.Contains(s, `"171":{"active":false}`) {
		t.Errorf("171 should have no fee: %s", s)
	}
	if strings.Contains(s, `"w7.x":{"w":{"crafter":"Player-0000-0000000B","cst":"review","customer":"Player-0000-0000000A","fee":0,"feeQuoted"`) {
		t.Errorf("feeQuoted invented: %s", s)
	}
}

func TestCraftingStripsTextAndBadItems(t *testing.T) {
	bad := `crafting = {
	  prices = { ["2835"] = 99 }, owner = "虛構甲",
	  services = { recordedAt = 10, note = "不該上傳", value = {
	    ["164"] = { active = true, fee = 100, name = "不該上傳", recipes = { ["1"] = 5, ["2"] = -1, ["3"] = "免費", ["x"] = 1, ["4"] = 1.5 } },
	    ["171"] = { active = "yes" },             -- wrong type: this profession only
	    ["182"] = { fee = -5 },                    -- negative: this profession only
	    ["notid"] = { active = true },
	    ["333"] = "不該上傳",
	  } },
	  orders = {
	    ["g1"] = { w = { recordedAt = 1, customer = "Player-0000-0000000A", crafter = "*", qty = 1, cst = "open",
	                     note = "不該上傳", ext = { ref = "不該上傳" }, customerName = "虛構甲", extra = 1 },
	               j = { recordedAt = 2, jst = "accept", crafterName = "虛構乙" }, x = "不該上傳" },
	    ["g2"] = { w = { recordedAt = 1, customer = "虛構甲", qty = 1 } },              -- name instead of GUID
	    ["g3"] = { w = { recordedAt = 1, customer = "Player-0000-0000000A", cst = "maybe" } }, -- unknown enum
	    ["g4"] = { j = { recordedAt = 1, jst = "accept", gave = 1.5 } },               -- not an integer
	    ["g5"] = { w = { recordedAt = 1, customer = "Player-0000-0000000A", qty = 0 } }, -- qty must be >= 1
	    ["g6"] = { w = { customer = "Player-0000-0000000A" } },                       -- no recordedAt
	    ["g7"] = { w = { recordedAt = 1, mats = { ["2835"] = "五個" } } },
	    ["G8"] = { w = { recordedAt = 1 } },                                           -- id: upper case
	    ["g9 虛構"] = { w = { recordedAt = 1 } },
	    ["` + strings.Repeat("a", 65) + `"] = { w = { recordedAt = 1 } },
	    ["g10"] = "不該上傳",
	    ["g11"] = { note = "不該上傳" },                                                -- no half left
	    ["g12"] = { w = { recordedAt = 3, customer = "Player-0000-0000000A", qty = 2, cst = "open" },
	                j = { recordedAt = 3, jst = "maybe" } },                              -- bad j: keep w
	    ["g13"] = { w = { recordedAt = 4, customer = "虛構甲" },                          -- bad w: keep j
	                j = { recordedAt = 4, jst = "accept", deliveredHow = "mail", acceptedAt = 9 } },
	    ["g15"] = { w = { recordedAt = 6, returnedAt = -1 }, j = { recordedAt = 6, jst = "drop", statusAt = 7 } },     -- bad w: keep j
	    ["g16"] = { w = { recordedAt = 7, cst = "returned", returnedAt = 8 }, j = { recordedAt = 7, statusAt = "昨天" } }, -- bad j: keep w
	    ["g14"] = { w = { recordedAt = 5, cst = "maybe" }, j = { recordedAt = 5, deliveredHow = "pigeon" } }, -- both bad
	  },
	}`
	for name, entry := range map[string]string{"member.share": shareWith(bad), "character.self": selfWith(bad)} {
		c, _ := craftingOf(t, entry)
		got := asJSON(c)
		want := `{"orders":{"g1":{"j":{"jst":"accept","recordedAt":2},"w":{"crafter":"*","cst":"open","customer":"Player-0000-0000000A","qty":1,"recordedAt":1}},` +
			`"g12":{"w":{"cst":"open","customer":"Player-0000-0000000A","qty":2,"recordedAt":3}},` +
			`"g13":{"j":{"acceptedAt":9,"deliveredHow":"mail","jst":"accept","recordedAt":4}},` +
			`"g15":{"j":{"jst":"drop","recordedAt":6,"statusAt":7}},` +
			`"g16":{"w":{"cst":"returned","recordedAt":7,"returnedAt":8}}},` +
			`"services":{"recordedAt":10,"value":{"164":{"active":true,"fee":100,"recipes":{"1":5}}}}}`
		if got != want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, want)
		}
		assertOnlyAllowedStrings(t, c)
	}
}

// Every string left in crafting is a GUID, "*" or a listed enumeration.
func assertOnlyAllowedStrings(t *testing.T, v any) {
	t.Helper()
	allowed := map[string]bool{"*": true}
	for _, e := range []map[string]bool{enumSrc, enumCst, enumJst, enumDelivery, enumClose, enumClosedBy, enumCloseHow} {
		for k := range e {
			allowed[k] = true
		}
	}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			if !allowed[x] && !guidRE.MatchString(x) {
				t.Errorf("text left in crafting: %q", x)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
}

func TestCraftingNotATableOrEmpty(t *testing.T) {
	for _, c := range []string{`crafting = "不該上傳"`, `crafting = { orders = { ["g1"] = { w = { customer = "x" } } } }`, `crafting = {}`} {
		for _, entry := range []string{shareWith(c), selfWith(c)} {
			if v, has := craftingOf(t, entry); has {
				t.Errorf("%s: crafting kept as %s", c, asJSON(v))
			}
		}
	}
}

func TestCraftingLimits(t *testing.T) {
	var b strings.Builder
	b.WriteString(`crafting = { services = { recordedAt = 1, value = {`)
	for p := 1; p <= MaxCraftingServices+1; p++ {
		fmt.Fprintf(&b, `["%d"] = { active = true`, p)
		if p == 1 {
			b.WriteString(`, recipes = {`)
			for r := 1; r <= MaxCraftingQuotes+1; r++ {
				fmt.Fprintf(&b, `["%d"] = %d, `, r, r)
			}
			b.WriteString(`}`)
		}
		b.WriteString(` }, `)
	}
	b.WriteString(`} }, orders = {`)
	// 5 open orders and 100 closed ones (closedAt 1..100): the 5 oldest
	// closed orders go.
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&b, `["o%d"] = { w = { recordedAt = %d, customer = "Player-0000-0000000A", cst = "open" } }, `, i, 1000+i)
	}
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&b, `["c%d"] = { w = { recordedAt = %d, close = "deal", closedAt = %d } }, `, i, i, i)
	}
	b.WriteString(`} }`)

	c, _ := craftingOf(t, shareWith(b.String()))
	m := c.(map[string]any)
	profs := m["services"].(map[string]any)["value"].(map[string]any)
	if len(profs) != MaxCraftingServices || profs["13"] != nil || profs["1"] == nil {
		t.Errorf("professions: %d kept, want ids 1..12", len(profs))
	}
	quotes := profs["1"].(map[string]any)["recipes"].(map[string]any)
	if len(quotes) != MaxCraftingQuotes || quotes["501"] != nil {
		t.Errorf("quotes: %d kept, want ids 1..500", len(quotes))
	}
	orders := m["orders"].(map[string]any)
	if len(orders) != MaxCraftingOrders {
		t.Fatalf("orders: %d kept, want %d", len(orders), MaxCraftingOrders)
	}
	for i := 1; i <= 5; i++ {
		if orders[fmt.Sprintf("o%d", i)] == nil {
			t.Errorf("open order o%d dropped", i)
		}
		if orders[fmt.Sprintf("c%d", i)] != nil {
			t.Errorf("oldest closed order c%d kept", i)
		}
	}
	if orders["c6"] == nil || orders["c100"] == nil {
		t.Error("newer closed orders dropped")
	}
}

// Only open orders over the limit: the oldest w.created go (recordedAt is
// the same for all of one author's orders and says nothing).
func TestCraftingLimitOpenByCreated(t *testing.T) {
	var b strings.Builder
	b.WriteString(`crafting = { orders = {`)
	for i := 1; i <= MaxCraftingOrders+2; i++ {
		fmt.Fprintf(&b, `["o%d"] = { w = { recordedAt = 999, created = %d, cst = "open" } }, `, i, i)
	}
	b.WriteString(`} }`)
	c, _ := craftingOf(t, selfWith(b.String()))
	orders := c.(map[string]any)["orders"].(map[string]any)
	if len(orders) != MaxCraftingOrders || orders["o1"] != nil || orders["o2"] != nil || orders["o3"] == nil || orders["o102"] == nil {
		t.Fatalf("kept %d orders; o1 %v o2 %v o3 %v", len(orders), orders["o1"] != nil, orders["o2"] != nil, orders["o3"] != nil)
	}
}
