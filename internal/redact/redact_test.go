package redact

import (
	"strings"
	"testing"
)

// Made-up names only.
const (
	acct  = "SWORDFISH77"
	acct2 = "SWORDFISH78"
	guid  = "Player-4711-0ABCDEF1"
)

func TestAccount(t *testing.T) {
	m := Account(acct)
	if strings.Contains(m, acct) || !strings.HasPrefix(m, "SW******~") || len(m) != len("SW******~0000") {
		t.Fatalf("Account(%q) = %q", acct, m)
	}
	if Account(acct) != m {
		t.Fatal("must be stable, so one account reads the same through a log")
	}
	if Account(acct2) == m {
		t.Fatal("two accounts with the same prefix must stay distinguishable")
	}
	if len(Account("A")) != len(Account("ABCDEFGHIJKLMNOPQRST"))-2 {
		t.Fatal("the masked form must not reveal the length")
	}
	for _, short := range []string{"AB", "ABCD", "12#1"} {
		if m := Account(short); strings.ContainsAny(m[:1], "A1") || !IsMasked(m) {
			t.Fatalf("short name %q kept characters: %q", short, m)
		}
	}
	if Account(m) != m || !IsMasked(m) {
		t.Fatal("masking twice must not change it")
	}
	if Account("") != "" {
		t.Fatal("empty")
	}
}

func TestTextMasksPathsAndGUIDs(t *testing.T) {
	forever := `D:\World of Warcraft\_forever_`
	for _, line := range []string{
		`read ` + forever + `\WTF\Account\` + acct + `\SavedVariables\GreenFangToolkit.lua: Access is denied.`,
		`game folder: access denied: ` + forever + `\WTF\Account\` + acct + `: Access is denied.`,
		`open d:/world of warcraft/_forever_/wtf/account/` + acct + `/SavedVariables/x.lua: oops`,
		`two: \WTF\Account\` + acct + `\a and \WTF\Account\` + acct2 + `\b`,
		`upload: ` + guid + `:1700000000:character.self rejected: bad`,
	} {
		out := Text(line)
		for _, secret := range []string{acct, acct2, guid, "0ABCDEF1"} {
			if strings.Contains(out, secret) {
				t.Errorf("%q leaked in %q", secret, out)
			}
		}
		if Text(out) != out {
			t.Errorf("not idempotent: %q -> %q", out, Text(out))
		}
	}
	got := Text(`read ` + forever + `\WTF\Account\` + acct + `\SavedVariables\GreenFangToolkit.lua: x`)
	want := `read ` + forever + `\WTF\Account\` + Account(acct) + `\SavedVariables\GreenFangToolkit.lua: x`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if !strings.Contains(Text("x "+guid+" y"), "Player-…DEF1") {
		t.Fatalf("GUID keeps the last 4: %q", Text(guid))
	}
	keep := `game folder detected: ` + forever
	if Text(keep) != keep {
		t.Fatal("the game folder path is kept")
	}
}
