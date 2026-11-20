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
//   - member.report and member.share entry ids (data other members shared
//     in game): kept only when shaped "GUID:number…:member.<kind>" with an
//     optional ":<subject GUID>" (the GUIDs are then shortened as above);
//     any other shape could hold a member's name, so it becomes
//     "member.<kind>~" and 4 hex digits of SHA-256(id).
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

// EntryID masks an outbox id for the log (see the package rules).
func EntryID(id string) string {
	m := memberKindRE.FindStringSubmatch(id)
	if m == nil || memberIDRE.MatchString(id) {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return "member." + m[1] + "~" + hex.EncodeToString(sum[:2])
}

var (
	// Any id that mentions a member kind, wherever the kind appears.
	memberKindRE = regexp.MustCompile(`:member\.(report|share)(:|$)`)
	// The safe shapes: "uploader GUID:number…:member.<kind>", optionally
	// followed by ":<subject GUID>" (savedvariables v2 ext). Only GUIDs and
	// numbers, so nothing but the GUIDs (shortened by Text) can identify
	// anyone.
	memberIDRE = regexp.MustCompile(`^Player-\d+-[0-9A-Fa-f]+(:\d+)+:member\.(report|share)(:Player-\d+-[0-9A-Fa-f]+)?$`)
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
