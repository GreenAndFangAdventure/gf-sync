// Package redact masks personal identifiers before they reach gfsync.log,
// so players can paste the log when asking for help.
//
// Rules (also in PRIVACY.md):
//
//   - WoW account folder names (WTF\Account\<name>): the first 2 characters,
//     then a fixed "******" (the length is not revealed), then "~" and the
//     first 4 hex digits of SHA-256(name), e.g. "SW******~3f9a". Names of 4
//     characters or fewer keep no characters: "******~1c2e". The hash only
//     tells two accounts apart in one log; 16 bits are far too few to
//     recover a name.
//   - Character GUIDs (Player-<realm>-<hex>): only the last 4 hex digits,
//     e.g. "Player-…EF01".
//   - Everything else, including the game folder
//     (D:\World of Warcraft\_forever_), is kept.
package redact

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

const stars = "******"

// Account masks a WoW account folder name.
func Account(name string) string {
	if name == "" || IsMasked(name) {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	keep := ""
	if r := []rune(name); len(r) > 4 {
		keep = string(r[:2])
	}
	return keep + stars + "~" + hex.EncodeToString(sum[:2])
}

// IsMasked reports whether s is already the output of Account.
func IsMasked(s string) bool { return maskedRE.MatchString(s) }

var (
	maskedRE = regexp.MustCompile(`^[^\\/]{0,2}\*{6}~[0-9a-f]{4}$`)
	// \WTF\Account\<name> in any path, with \ or /, any case. The name
	// stops at a separator or a character Windows does not allow in names
	// (so "…\Account\NAME: access denied" stops before the colon). "*" is
	// allowed so an already masked name is matched whole and kept as is.
	accountRE = regexp.MustCompile(`(?i)([\\/]WTF[\\/]Account[\\/])([^\\/:?"<>|\r\n]+)`)
	guidRE    = regexp.MustCompile(`Player-\d+-([0-9A-Fa-f]{4,})`)
)

// GUID shortens one character GUID; other strings are returned unchanged.
func GUID(s string) string { return guidRE.ReplaceAllStringFunc(s, shortGUID) }

func shortGUID(m string) string {
	hexPart := guidRE.FindStringSubmatch(m)[1]
	return "Player-…" + hexPart[len(hexPart)-4:]
}

// Text masks every account folder name in a path and every character GUID
// inside s (a log line, an error message). Bare account names that are not
// part of a path must be passed through Account by the caller.
func Text(s string) string {
	s = accountRE.ReplaceAllStringFunc(s, func(m string) string {
		p := accountRE.FindStringSubmatch(m)
		return p[1] + Account(p[2])
	})
	if strings.Contains(s, "Player-") {
		s = guidRE.ReplaceAllStringFunc(s, shortGUID)
	}
	return s
}
