// Package svdoc interprets GreenFangToolkit.lua according to
// the SavedVariables format (docs/FORMAT.md). It never writes anything.
package svdoc

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"

	"gfsync/internal/luasv"
)

// VarName is the account-level SavedVariables table (never changes).
const VarName = "GreenFangToolkitDB"

// FileName is the SavedVariables file GF Sync watches.
const FileName = "GreenFangToolkit.lua"

// Known entry kinds. Unknown kinds are ignored (contract §3.5).
var KnownKinds = map[string]bool{
	"character.self":  true,
	"roster.snapshot": true,
	// Another member shared their own data in game (decision D-011);
	// the uploader is the local character that received it.
	KindMemberReport: true,
	// Data relayed in game, one entry per member with a time per field
	// (decision D-014). The payload is rebuilt from a whitelist.
	KindMemberShare: true,
	// Recipe ID -> name table, so the site can show recipe names.
	KindRecipeNames: true,
}

// KindMemberShare and KindRecipeNames are the D-014 kinds
// (savedvariables v2 extension).
const (
	KindMemberShare = "member.share"
	KindRecipeNames = "recipe.names"
)

// KindMemberReport is data a guild member shared with this player in game.
const KindMemberReport = "member.report"

// ownLocationKinds may carry a location: the player's own character, and
// a member's own report (each player can turn sharing off in game).
var ownLocationKinds = map[string]bool{"character.self": true, KindMemberReport: true}

// Guild is the only guild whose entries GF Sync uploads (upload API v1
// §3.1). Entries whose uploader.guild is anything else, or missing, never
// leave the computer, even if an old or edited AddOn wrote them.
const Guild = "綠皮與獠牙冒險事務所"

// MaxIDLen bounds entry ids; real ids are "GUID:time:kind" (< 80 bytes).
const MaxIDLen = 256

// MaxMemberShare is a safety ceiling on member.share entries taken from one
// file. A WoW guild has at most 1,000 members and the AddOn writes at most
// one entry per member, so any count the AddOn writes up to this is taken
// in full (EVAL-1000). Above it the AddOn is misbehaving: the newest
// MaxMemberShare (by capturedAt) are kept, the rest counted in OverLimit.
const MaxMemberShare = 1200

// Status of a parsed document.
type Status int

const (
	StatusOK          Status = iota
	StatusDisabled           // sync.enabled ~= true
	StatusUnsupported        // schemaVersion not 2
	StatusNoTable            // file has no GreenFangToolkitDB table
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusDisabled:
		return "disabled"
	case StatusUnsupported:
		return "unsupported"
	case StatusNoTable:
		return "no-table"
	}
	return "unknown"
}

// Entry is one outbox item ready for upload.
type Entry struct {
	ID   string
	Kind string
	JSON json.RawMessage // the entry converted to JSON (privacy fields removed)
}

// Doc is the interpretation of one SavedVariables file.
type Doc struct {
	Status        Status
	SchemaVersion any // raw value, for messages
	AddonVersion  string
	Entries       []Entry
	Skipped       int // malformed or unknown-kind entries
	OtherGuild    int // valid entries whose uploader is not in Guild (not uploaded)
	OverLimit     int // member.share entries beyond MaxMemberShare (not uploaded)
}

// ErrParse wraps parser failures.
var ErrParse = errors.New("svdoc: cannot parse SavedVariables")

// Parse interprets a SavedVariables file's bytes.
func Parse(src []byte) (*Doc, error) {
	vars, err := luasv.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParse, err)
	}
	db, ok := vars[VarName].(*luasv.Table)
	if !ok {
		return &Doc{Status: StatusNoTable}, nil
	}
	d := &Doc{SchemaVersion: db.Get("schemaVersion")}
	if s, ok := db.Get("addonVersion").(string); ok {
		d.AddonVersion = s
	}
	switch v := db.Get("schemaVersion").(type) {
	case int64:
		// Only v2 exists (contract §3.5); v3 will be added here when it ships.
		if v != 2 {
			d.Status = StatusUnsupported
			return d, nil
		}
	default:
		d.Status = StatusUnsupported
		return d, nil
	}
	sync, _ := db.Get("sync").(*luasv.Table)
	if sync == nil || sync.Get("enabled") != true {
		d.Status = StatusDisabled
		return d, nil
	}
	outbox, _ := sync.Get("outbox").(*luasv.Table)
	if outbox == nil {
		return d, nil
	}
	var shareAt []float64
	for _, item := range outbox.Array() {
		e, ok := toEntry(item)
		if !ok {
			d.Skipped++
			continue
		}
		if !ourGuild(item.(*luasv.Table)) {
			d.OtherGuild++
			continue
		}
		d.Entries = append(d.Entries, e)
		if e.Kind == KindMemberShare {
			shareAt = append(shareAt, capturedAt(item.(*luasv.Table)))
		}
	}
	d.capShares(shareAt)
	return d, nil
}

func capturedAt(t *luasv.Table) float64 {
	switch v := t.Get("capturedAt").(type) {
	case int64:
		return float64(v)
	case float64:
		return v
	}
	return 0
}

// capShares applies MaxMemberShare. at[i] is the capturedAt of the i-th
// member.share entry in d.Entries. Order of the kept entries is unchanged.
func (d *Doc) capShares(at []float64) {
	if len(at) <= MaxMemberShare {
		return
	}
	idx := make([]int, len(at))
	for i := range idx {
		idx[i] = i
	}
	// Newest first; ties keep the later entry in the outbox (written last).
	sort.SliceStable(idx, func(a, b int) bool {
		if at[idx[a]] != at[idx[b]] {
			return at[idx[a]] > at[idx[b]]
		}
		return idx[a] > idx[b]
	})
	keep := make(map[int]bool, MaxMemberShare)
	for _, i := range idx[:MaxMemberShare] {
		keep[i] = true
	}
	out := d.Entries[:0]
	n := 0
	for _, e := range d.Entries {
		if e.Kind == KindMemberShare {
			k := keep[n]
			n++
			if !k {
				d.OverLimit++
				continue
			}
		}
		out = append(out, e)
	}
	d.Entries = out
}

// ourGuild reports whether the entry's uploader.guild is exactly Guild.
func ourGuild(t *luasv.Table) bool {
	up, _ := t.Get("uploader").(*luasv.Table)
	if up == nil {
		return false
	}
	g, _ := up.Get("guild").(string)
	return g == Guild
}

func toEntry(item any) (Entry, bool) {
	t, ok := item.(*luasv.Table)
	if !ok {
		return Entry{}, false
	}
	id, _ := t.Get("id").(string)
	kind, _ := t.Get("kind").(string)
	if id == "" || len(id) > MaxIDLen || !KnownKinds[kind] {
		return Entry{}, false
	}
	switch t.Get("capturedAt").(type) {
	case int64, float64:
	default:
		return Entry{}, false
	}
	obj, ok := luasv.ToJSON(t).(map[string]any)
	if !ok {
		return Entry{}, false
	}
	// Optional; a bad value is dropped alone (savedvariables §3.9).
	switch obj["capturedAtServer"].(type) {
	case nil, int64, float64:
	default:
		delete(obj, "capturedAtServer")
	}
	var share map[string]any
	if kind == KindMemberShare {
		if share, ok = sharePayload(obj["payload"]); !ok {
			return Entry{}, false
		}
	}
	stripPrivate(obj, kind)
	switch kind {
	case KindMemberReport:
		if !keepSubject(obj) {
			return Entry{}, false
		}
	case KindMemberShare:
		obj["payload"] = share
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return Entry{}, false
	}
	return Entry{ID: id, Kind: kind, JSON: raw}, true
}

// privateKeys are never uploaded, even if an AddOn version writes them:
// no online status, and no location except the one allowed below.
var privateKeys = []string{"zone", "online", "location"}

// MaxLocationLen bounds location.zone and location.subZone, in characters.
// Real zone names are far shorter; anything longer is dropped, not cut.
const MaxLocationLen = 60

// stripPrivate removes zone, online and location everywhere (the entry,
// its payload, every roster member). The one exception (decision D-009):
// a character.self or member.report payload may keep its own location,
// rebuilt from only the zone and subZone strings. Players can turn this
// off in game. roster.snapshot never keeps one.
func stripPrivate(entry map[string]any, kind string) {
	for _, k := range privateKeys {
		delete(entry, k)
	}
	payload, ok := entry["payload"].(map[string]any)
	if !ok {
		return
	}
	loc := payload["location"]
	for _, k := range privateKeys {
		delete(payload, k)
	}
	if ownLocationKinds[kind] {
		if l := ownLocation(loc); l != nil {
			payload["location"] = l
		}
	}
	if members, ok := payload["members"].([]any); ok {
		for _, m := range members {
			if mm, ok := m.(map[string]any); ok {
				for _, k := range privateKeys {
					delete(mm, k)
				}
			}
		}
	}
}

// ownLocation keeps only {zone, subZone}. zone must be a non-empty string
// of at most MaxLocationLen characters, or there is no location at all;
// subZone is optional and dropped alone when it is not such a string.
func ownLocation(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	zone, ok := locationText(m["zone"])
	if !ok {
		return nil
	}
	out := map[string]any{"zone": zone}
	if sub, ok := locationText(m["subZone"]); ok {
		out["subZone"] = sub
	}
	return out
}

func locationText(v any) (string, bool) { return boundedText(v, MaxLocationLen) }

// MaxSubjectLen bounds member.report subject fields, in characters.
const MaxSubjectLen = 64

// keepSubject rebuilds a member.report payload.subject from only name,
// realm and guid. name is required (a non-empty string of at most
// MaxSubjectLen characters), or the entry is malformed; realm and guid are
// kept only when they are such strings.
func keepSubject(entry map[string]any) bool {
	payload, ok := entry["payload"].(map[string]any)
	if !ok {
		return false
	}
	out := subjectOf(payload["subject"])
	if out == nil {
		return false
	}
	payload["subject"] = out
	return true
}

// subjectOf rebuilds a subject from only name (required), realm and guid;
// nil when there is no valid name.
func subjectOf(v any) map[string]any {
	subj, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	name, ok := boundedText(subj["name"], MaxSubjectLen)
	if !ok {
		return nil
	}
	out := map[string]any{"name": name}
	for _, k := range []string{"realm", "guid"} {
		if v, ok := boundedText(subj[k], MaxSubjectLen); ok {
			out[k] = v
		}
	}
	return out
}

func boundedText(v any, max int) (string, bool) {
	s, ok := v.(string)
	if !ok || s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return "", false
	}
	return s, true
}
