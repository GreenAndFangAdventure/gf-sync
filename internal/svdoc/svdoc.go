// Package svdoc interprets GreenFangToolkit.lua according to
// the SavedVariables format (docs/FORMAT.md). It never writes anything.
package svdoc

import (
	"encoding/json"
	"errors"
	"fmt"
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
}

// Guild is the only guild whose entries GF Sync uploads (upload API v1
// §3.1). Entries whose uploader.guild is anything else, or missing, never
// leave the computer, even if an old or edited AddOn wrote them.
const Guild = "綠皮與獠牙冒險事務所"

// MaxIDLen bounds entry ids; real ids are "GUID:time:kind" (< 80 bytes).
const MaxIDLen = 256

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
	}
	return d, nil
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
	stripPrivate(obj, kind)
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
// a character.self payload may keep its own location, rebuilt from only
// the zone and subZone strings. The player can turn this off in game.
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
	if kind == "character.self" {
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

func locationText(v any) (string, bool) {
	s, ok := v.(string)
	if !ok || s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > MaxLocationLen {
		return "", false
	}
	return s, true
}
