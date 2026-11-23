package svdoc

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits on adventure (savedvariables §adventure "上限"), per author.
const (
	MaxAdventureTickets   = 20
	MaxAdventureActions   = 50
	MaxAdventureDecisions = 40 // per ticket
	MaxAdventureQuests    = 5
	MaxAdventureTimes     = 3
	MaxAdventureDungeons  = 40 // worker.dungeons; not in the contract, a bound only
	MaxNoteRunes          = 40
)

var dungeonRE = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

var (
	advSrc      = set("g", "w", "d")
	advMode     = set("now", "today", "future")
	advRole     = set("tank", "healer", "dps")
	advTag      = set("quest", "repeat", "slow", "fast", "voice")
	advAdmit    = set("direct", "consent")
	advStatus   = set("open", "departed", "done", "cancel")
	advDecision = set("approve", "kick", "invite")
	advAction   = set("apply", "withdraw", "accept", "refuse")
)

// adventurePayload rebuilds an adventure table (savedvariables §adventure,
// §3.8) from only the listed keys and types. Strings are only GUIDs, ids,
// dungeon codes and the listed enumerations, plus one free text: a
// ticket's note, cleaned by cleanNote. A ticket, action or decision with
// a listed key of the wrong type is dropped alone; a bad note is dropped
// alone and never costs the ticket. nil when nothing is left.
func adventurePayload(v any) map[string]any {
	in, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if t := advSection(in["tickets"], advTickets); t != nil {
		out["tickets"] = t
	}
	if w := advSection(in["worker"], advWorker); w != nil {
		out["worker"] = w
	}
	if a := advSection(in["actions"], advActions); a != nil {
		out["actions"] = a
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// advSection is { recordedAt, value } with value rebuilt by f.
func advSection(v any, f func(any) any) map[string]any {
	in, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	at, ok := nonNegInt(in["recordedAt"])
	if !ok {
		return nil
	}
	val := f(in["value"])
	if val == nil {
		return nil
	}
	return map[string]any{"recordedAt": at, "value": val}
}

// advTickets keeps valid tickets by id. Over MaxAdventureTickets, the ones
// that ended longest ago go first (done/cancel by statusAt), then the
// oldest others by created.
func advTickets(v any) any {
	in, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	type kept struct {
		id    string
		t     map[string]any
		ended bool
		at    int64
	}
	var list []kept
	for id, raw := range in {
		if !orderIDRE.MatchString(id) {
			continue
		}
		t := advTicket(raw)
		if t == nil {
			continue
		}
		k := kept{id: id, t: t}
		if s, _ := t["status"].(string); s == "done" || s == "cancel" {
			k.ended = true
			k.at, _ = t["statusAt"].(int64)
		} else {
			k.at, _ = t["created"].(int64)
		}
		list = append(list, k)
	}
	if len(list) > MaxAdventureTickets {
		sort.Slice(list, func(i, j int) bool {
			a, b := list[i], list[j]
			if a.ended != b.ended {
				return !a.ended
			}
			if a.at != b.at {
				return a.at > b.at
			}
			return a.id < b.id
		})
		list = list[:MaxAdventureTickets]
	}
	if len(list) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, k := range list {
		out[k.id] = k.t
	}
	return out
}

// advTicket is one ticket; nil if a listed key has the wrong type. note
// and decisions are cleaned item by item and never make it nil.
func advTicket(v any) map[string]any {
	in, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	simple := map[string]fieldSpec{
		"src": {fEnum, advSrc}, "ver": {kind: fNum}, "created": {kind: fNum},
		"admit": {fEnum, advAdmit}, "status": {fEnum, advStatus}, "statusAt": {kind: fNum},
	}
	for k, s := range simple {
		if raw, present := in[k]; present {
			val, ok := s.check(raw)
			if !ok {
				return nil
			}
			out[k] = val
		}
	}
	if raw, present := in["dungeon"]; present {
		d, ok := raw.(string)
		if !ok || !dungeonRE.MatchString(d) {
			return nil
		}
		out["dungeon"] = d
	}
	if raw, present := in["quests"]; present {
		q, ok := questList(raw)
		if !ok {
			return nil
		}
		out["quests"] = q
	}
	if raw, present := in["time"]; present {
		t, ok := advTime(raw)
		if !ok {
			return nil
		}
		out["time"] = t
	}
	if raw, present := in["slots"]; present {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil
		}
		slots := map[string]any{}
		for _, k := range []string{"tank", "healer", "dps", "size"} {
			if n, present := m[k]; present {
				c, ok := nonNegInt(n)
				if !ok {
					return nil
				}
				slots[k] = c
			}
		}
		out["slots"] = slots
	}
	if raw, present := in["tags"]; present {
		list, ok := raw.([]any)
		if !ok {
			return nil
		}
		tags := []any{}
		seen := map[string]bool{}
		for _, t := range list {
			// An unknown tag is dropped alone (a newer AddOn may add one).
			if s, ok := t.(string); ok && advTag[s] && !seen[s] {
				seen[s] = true
				tags = append(tags, s)
			}
		}
		out["tags"] = tags
	}
	if n, ok := cleanNote(in["note"]); ok {
		out["note"] = n
	}
	if d := advDecisions(in["decisions"]); d != nil {
		out["decisions"] = d
	}
	return out
}

// advDecisions keeps { [GUID] = { d, at } }; a bad one is dropped alone.
// Over MaxAdventureDecisions, the latest (by at) are kept.
func advDecisions(v any) map[string]any {
	in, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	type kept struct {
		guid string
		at   int64
		d    map[string]any
	}
	var list []kept
	for g, raw := range in {
		m, ok := raw.(map[string]any)
		if !ok || !guidRE.MatchString(g) {
			continue
		}
		d, ok1 := m["d"].(string)
		at, ok2 := nonNegInt(m["at"])
		if !ok1 || !advDecision[d] || !ok2 {
			continue
		}
		list = append(list, kept{g, at, map[string]any{"d": d, "at": at}})
	}
	if len(list) > MaxAdventureDecisions {
		sort.Slice(list, func(i, j int) bool {
			if list[i].at != list[j].at {
				return list[i].at > list[j].at
			}
			return list[i].guid < list[j].guid
		})
		list = list[:MaxAdventureDecisions]
	}
	if len(list) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, k := range list {
		out[k.guid] = k.d
	}
	return out
}

// advWorker is the one worker ticket; nil if a listed key is wrong.
func advWorker(v any) any {
	in, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if raw, present := in["dungeons"]; present {
		list, ok := raw.([]any)
		if !ok || len(list) > MaxAdventureDungeons {
			return nil
		}
		ds := []any{}
		for _, d := range list {
			s, ok := d.(string)
			if !ok || !dungeonRE.MatchString(s) {
				return nil
			}
			ds = append(ds, s)
		}
		out["dungeons"] = ds
	}
	if raw, present := in["quests"]; present {
		q, ok := questList(raw)
		if !ok {
			return nil
		}
		out["quests"] = q
	}
	if raw, present := in["allQuests"]; present {
		b, ok := raw.(bool)
		if !ok {
			return nil
		}
		out["allQuests"] = b
	}
	for _, k := range []string{"main", "alt"} {
		if raw, present := in[k]; present {
			s, ok := raw.(string)
			if !ok || !advRole[s] {
				return nil
			}
			out[k] = s
		}
	}
	if raw, present := in["times"]; present {
		list, ok := raw.([]any)
		if !ok || len(list) > MaxAdventureTimes {
			return nil
		}
		ts := []any{}
		for _, t := range list {
			tt, ok := advTime(t)
			if !ok {
				return nil
			}
			ts = append(ts, tt)
		}
		out["times"] = ts
	}
	if raw, present := in["expires"]; present {
		n, ok := nonNegInt(raw)
		if !ok {
			return nil
		}
		out["expires"] = n
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// advActions keeps { [ticket id] = { a, at, firstAt, role } }; a bad one
// is dropped alone. Over MaxAdventureActions, the latest (by at) are kept.
func advActions(v any) any {
	in, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	type kept struct {
		id string
		at int64
		a  map[string]any
	}
	var list []kept
	for id, raw := range in {
		m, ok := raw.(map[string]any)
		if !ok || !orderIDRE.MatchString(id) {
			continue
		}
		act, ok := m["a"].(string)
		if !ok || !advAction[act] {
			continue
		}
		out := map[string]any{"a": act}
		bad := false
		for _, k := range []string{"at", "firstAt"} {
			if raw, present := m[k]; present {
				n, ok := nonNegInt(raw)
				if !ok {
					bad = true
					break
				}
				out[k] = n
			}
		}
		if raw, present := m["role"]; present && !bad {
			s, ok := raw.(string)
			if !ok || !advRole[s] {
				bad = true
			} else {
				out["role"] = s
			}
		}
		if bad {
			continue
		}
		at, _ := out["at"].(int64)
		list = append(list, kept{id, at, out})
	}
	if len(list) > MaxAdventureActions {
		sort.Slice(list, func(i, j int) bool {
			if list[i].at != list[j].at {
				return list[i].at > list[j].at
			}
			return list[i].id < list[j].id
		})
		list = list[:MaxAdventureActions]
	}
	if len(list) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, k := range list {
		out[k.id] = k.a
	}
	return out
}

// advTime is { mode, from, to }; "now" has no from/to.
func advTime(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	mode, ok := m["mode"].(string)
	if !ok || !advMode[mode] {
		return nil, false
	}
	out := map[string]any{"mode": mode}
	if mode == "now" {
		return out, true
	}
	for _, k := range []string{"from", "to"} {
		if raw, present := m[k]; present {
			n, ok := nonNegInt(raw)
			if !ok {
				return nil, false
			}
			out[k] = n
		}
	}
	return out, true
}

// questList: up to MaxAdventureQuests positive integers.
func questList(v any) ([]any, bool) {
	list, ok := v.([]any)
	if !ok || len(list) > MaxAdventureQuests {
		return nil, false
	}
	out := []any{}
	for _, q := range list {
		n, ok := nonNegInt(q)
		if !ok || n < 1 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// cleanNote keeps a note as one line of plain text: invalid UTF-8,
// control characters (newlines, tabs included), invisible format
// characters (e.g. bidi overrides) and line/paragraph separators are
// removed, then it is cut to MaxNoteRunes characters. ok is false when
// it is not a string or nothing is left.
func cleanNote(v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	var b strings.Builder
	n := 0
	for len(s) > 0 && n < MaxNoteRunes {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		if (r == utf8.RuneError && size <= 1) || unicode.IsControl(r) ||
			unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			continue
		}
		b.WriteRune(r)
		n++
	}
	out := strings.TrimSpace(b.String())
	return out, out != ""
}
