package luasv

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const wowStyle = `
GreenFangToolkitDB = {
	["schema"] = "gf.toolkit",
	["schemaVersion"] = 2,
	["addonVersion"] = "0.4.0",
	["sync"] = {
		["enabled"] = true,
		["outbox"] = {
			{
				["id"] = "Player-1234-ABCDEF01:1763370000:character.self",
				["capturedAt"] = 1763370000,
				["payload"] = {
					["level"] = 32,
					["class"] = "德魯伊",
					["professions"] = {
						{
							["name"] = "製皮",
							["rank"] = 151,
						}, -- [1]
					},
				},
			}, -- [1]
		},
	},
}
OtherAddonDB = nil
`

func TestParseWoWStyle(t *testing.T) {
	vars, err := Parse([]byte(wowStyle))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := vars["OtherAddonDB"]; ok {
		t.Fatal("nil assignment should not create a variable")
	}
	db := vars["GreenFangToolkitDB"].(*Table)
	if db.Get("schemaVersion") != int64(2) {
		t.Fatalf("schemaVersion = %#v", db.Get("schemaVersion"))
	}
	sync := db.Get("sync").(*Table)
	if sync.Get("enabled") != true {
		t.Fatal("enabled")
	}
	outbox := sync.Get("outbox").(*Table)
	if outbox.Len() != 1 || !outbox.IsArray() {
		t.Fatalf("outbox len %d", outbox.Len())
	}
	e := outbox.Array()[0].(*Table)
	if e.Get("id") != "Player-1234-ABCDEF01:1763370000:character.self" {
		t.Fatal("id")
	}
	b, _ := json.Marshal(ToJSON(e))
	want := `{"capturedAt":1763370000,"id":"Player-1234-ABCDEF01:1763370000:character.self","payload":{"class":"德魯伊","level":32,"professions":[{"name":"製皮","rank":151}]}}`
	if string(b) != want {
		t.Fatalf("json:\n got %s\nwant %s", b, want)
	}
}

func TestScalars(t *testing.T) {
	cases := map[string]any{
		`x = 0`:                    int64(0),
		`x = -12`:                  int64(-12),
		`x = - 12`:                 int64(-12),
		`x = 3.5`:                  3.5,
		`x = -0.25`:                -0.25,
		`x = .5`:                   0.5,
		`x = 1e3`:                  1000.0,
		`x = 1.5E-2`:               0.015,
		`x = 0x1F`:                 int64(31),
		`x = 9223372036854775807`:  int64(9223372036854775807),
		`x = 99999999999999999999`: 1e20,
		`x = true`:                 true,
		`x = false`:                false,
		`x = "a\"b\\c"`:            `a"b\c`,
		`x = 'single "q"'`:         `single "q"`,
		`x = "tab\tnl\n"`:          "tab\tnl\n",
		`x = "\65\066\0671"`:       "ABC1",
		`x = "\x41\x42"`:           "AB",
		`x = "line\
two"`: "line\ntwo",
		`x = [[long
string]]`: "long\nstring",
		`x = [==[has ]] inside]==]`: "has ]] inside",
		`x = "中文綠皮"`:                "中文綠皮",
	}
	for src, want := range cases {
		vars, err := Parse([]byte(src))
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		if vars["x"] != want {
			t.Errorf("%q: got %#v want %#v", src, vars["x"], want)
		}
	}
}

func TestTableForms(t *testing.T) {
	src := `t = { 1, "two"; [3] = 3, name = "n", ["k"] = { }, [10] = true, [1.0] = "one", [true] = 1, [2.5] = "f", }`
	vars, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	tb := vars["t"].(*Table)
	if tb.Int[1] != "one" || tb.Int[2] != "two" || tb.Int[3] != int64(3) || tb.Int[10] != true {
		t.Fatalf("int part %#v", tb.Int)
	}
	if tb.Str["name"] != "n" || tb.Str["true"] != int64(1) || tb.Str["2.5"] != "f" {
		t.Fatalf("str part %#v", tb.Str)
	}
	if tb.IsArray() {
		t.Fatal("mixed table must not be an array")
	}
	b, _ := json.Marshal(ToJSON(tb))
	if !strings.Contains(string(b), `"k":[]`) || !strings.Contains(string(b), `"10":true`) {
		t.Fatalf("json %s", b)
	}
}

func TestPositionalNilHole(t *testing.T) {
	vars, err := Parse([]byte(`t = { "a", nil, "c" }`))
	if err != nil {
		t.Fatal(err)
	}
	tb := vars["t"].(*Table)
	if tb.Int[1] != "a" || tb.Int[3] != "c" || tb.Len() != 1 || tb.IsArray() {
		t.Fatalf("%#v", tb.Int)
	}
}

func TestComments(t *testing.T) {
	src := "-- header\n--[[ block\ncomment ]] a = 1 --[==[ x ]==] b = { -- c\n 2 } ; c = 3"
	vars, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if vars["a"] != int64(1) || vars["c"] != int64(3) || vars["b"].(*Table).Int[1] != int64(2) {
		t.Fatalf("%#v", vars)
	}
}

func TestBOMAndEmpty(t *testing.T) {
	vars, err := Parse([]byte("\xEF\xBB\xBFa = 1"))
	if err != nil || vars["a"] != int64(1) {
		t.Fatal(err, vars)
	}
	vars, err = Parse(nil)
	if err != nil || len(vars) != 0 {
		t.Fatal(err, vars)
	}
}

// Every one of these must be rejected without executing anything.
func TestRejectsCodeAndMalformed(t *testing.T) {
	bad := []string{
		`os.execute("calc")`,
		`x = os.execute("calc")`,
		`x = loadstring("return 1")()`,
		`x = function() end`,
		`x = y`,
		`x = 1 + 2`,
		`x = "a" .. "b"`,
		`x = -"str"`,
		`x = #t`,
		`x = not true`,
		`x = {f = print}`,
		`x = { [print] = 1 }`,
		`x = { {} = 1 }`,
		`x = { [{}] = 1 }`,
		`x = { [nil] = 1 }`,
		`local x = 1`,
		`return {}`,
		`x = 1 y`,
		`x 1`,
		`= 1`,
		`x =`,
		`x = {`,
		`x = { 1 2 }`,
		`x = "unterminated`,
		"x = \"newline\nin string\"",
		`x = [[never closed`,
		`--[[ never closed`,
		`x = "\q"`,
		`x = "\300"`,
		`x = "\xZZ"`,
		`x = 0x`,
		`x = 1e`,
		`x = 1.2.3`,
		`x = 12abc`,
		`x = 1e999`,
		`x = inf`,
		`x = nan`,
		`x = { [1] }`,
		`x = { [1] = }`,
		`true = 1`,
		`x = {}; y = setmetatable({}, {})`,
		`x = { a = 1 } }`,
		"x = \x00",
		`x = @`,
	}
	for _, src := range bad {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("expected error for %q", src)
		} else {
			var se *SyntaxError
			if !errors.As(err, &se) {
				t.Errorf("%q: unexpected error type %T", src, err)
			}
		}
	}
}

func TestLimits(t *testing.T) {
	deep := "x = " + strings.Repeat("{", 100) + strings.Repeat("}", 100)
	if _, err := Parse([]byte(deep)); err == nil {
		t.Error("deep nesting should fail")
	}
	ok := "x = " + strings.Repeat("{", 60) + strings.Repeat("}", 60)
	if _, err := Parse([]byte(ok)); err != nil {
		t.Errorf("depth 60 should pass: %v", err)
	}
	lim := DefaultLimits
	lim.MaxBytes = 10
	if _, err := ParseWithLimits([]byte("x = 12345678901"), lim); !errors.Is(err, ErrTooLarge) {
		t.Errorf("want ErrTooLarge, got %v", err)
	}
	lim = DefaultLimits
	lim.MaxStringLen = 4
	if _, err := ParseWithLimits([]byte(`x = "12345"`), lim); err == nil {
		t.Error("long string should fail")
	}
	if _, err := ParseWithLimits([]byte(`x = [[12345]]`), lim); err == nil {
		t.Error("long bracket string should fail")
	}
	lim = DefaultLimits
	lim.MaxNodes = 5
	if _, err := ParseWithLimits([]byte(`x = {1,2,3,4,5,6}`), lim); err == nil {
		t.Error("too many nodes should fail")
	}
}

func TestTruncatedFileFails(t *testing.T) {
	full := []byte(wowStyle)
	for i := 1; i < len(full)-2; i += 7 {
		cut := full[:i]
		// Must not panic. Most cuts are errors; a cut that happens to be
		// valid Lua is fine too.
		_, _ = Parse(cut)
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(wowStyle))
	f.Add([]byte(`x = { [1] = "a", b = { -1.5e3, 0x10 } }`))
	f.Add([]byte(`--[[ c ]] x = [==[ s ]==]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		v, err := Parse(data)
		if err == nil {
			for _, x := range v {
				_, _ = json.Marshal(ToJSON(x))
			}
		}
	})
}
