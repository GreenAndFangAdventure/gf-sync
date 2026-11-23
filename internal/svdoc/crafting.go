package svdoc

import (
	"math"
	"regexp"
	"sort"
	"strconv"
)

// Limits on crafting (savedvariables §crafting "上限"), per author: one
// crafting table belongs to one author (the uploader in character.self,
// the subject in member.share).
const (
	MaxCraftingOrders   = 100
	MaxCraftingServices = 12
	MaxCraftingQuotes   = 500
	MaxOrderIDLen       = 64
)

var (
	orderIDRE = regexp.MustCompile(`^[0-9a-z.]{1,64}$`)
	// A WoW character GUID, e.g. "Player-1234-0A1B2C3D".
	guidRE = regexp.MustCompile(`^Player-[0-9]{1,10}-[0-9A-Fa-f]{1,16}$`)
)

// The enumerations listed in §crafting. No other string is ever kept.
var (
	enumSrc      = set("g", "w", "d") // where the order was made, as the id prefix
	enumCst      = set("open", "review", "deal", "cancel", "returned")
	enumJst      = set("accept", "decline", "drop", "delivered")
	enumDelivery = set("trade", "mail", "manual")
	enumClose    = set("deal", "cancel")
	enumClosedBy = set("customer", "crafter", "web")
	enumCloseHow = set("trade", "mail", "manual", "timeout")
)

func set(s ...string) map[string]bool {
	m := map[string]bool{}
	for _, v := range s {
		m[v] = true
	}
	return m
}

// fieldKind is the one type a crafting key may have.
type fieldKind int

const (
	fNum      fieldKind = iota // non-negative integer
	fPositive                  // integer >= 1
	fGUID                      // character GUID
	fGUIDStar                  // character GUID or "*"
	fBool
	fEnum
	fCounts // { ["<id>"] = non-negative integer }
)

type fieldSpec struct {
	kind fieldKind
	enum map[string]bool
}

// closing fields, written by whichever side closed the order.
var closeFields = map[string]fieldSpec{
	"close":    {fEnum, enumClose},
	"closedBy": {fEnum, enumClosedBy},
	"closeHow": {fEnum, enumCloseHow},
	"closedAt": {kind: fNum},
}

// customerHalf is order.w; note (version 2) and ext (website orders) are
// text and are never uploaded from the AddOn.
var customerHalf = merge(map[string]fieldSpec{
	"recordedAt": {kind: fNum},
	"src":        {fEnum, enumSrc},
	"ver":        {kind: fNum},
	"created":    {kind: fNum},
	"customer":   {kind: fGUID},
	"crafter":    {kind: fGUIDStar},
	"lock":       {kind: fGUID},
	"lockedAt":   {kind: fNum},
	"recipe":     {kind: fNum},
	"item":       {kind: fNum},
	"qty":        {kind: fPositive},
	"mats":       {kind: fCounts},
	"fee":        {kind: fNum},
	"feeQuoted":  {kind: fNum},
	"quote":      {kind: fNum},
	"cst":        {fEnum, enumCst},
	"got":        {kind: fNum},
}, closeFields)

// crafterHalf is order.j.
var crafterHalf = merge(map[string]fieldSpec{
	"recordedAt":   {kind: fNum},
	"jst":          {fEnum, enumJst},
	"gave":         {kind: fNum},
	"acceptedAt":   {kind: fNum},
	"deliveredAt":  {kind: fNum},
	"deliveredHow": {fEnum, enumDelivery},
}, closeFields)

func merge(a, b map[string]fieldSpec) map[string]fieldSpec {
	out := map[string]fieldSpec{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// craftingPayload rebuilds a crafting table (savedvariables §crafting,
// §3.8 and §3.11) from only the listed keys and types. Strings are only
// GUIDs, "*" and the listed enumerations; numbers are non-negative
// integers. A profession or order with a listed key of the wrong type is
// dropped alone; keys that are not listed are dropped. A missing fee stays
// missing (it is not 0: 0 means free). nil when nothing is left.
func craftingPayload(v any) map[string]any {
	in, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if s := craftingServices(in["services"]); s != nil {
		out["services"] = s
	}
	if o := craftingOrders(in["orders"]); o != nil {
		out["orders"] = o
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// craftingServices keeps { recordedAt, value[<skillLineID>] = { active,
// fee, recipes[<recipe id>] } }. Over MaxCraftingServices professions or
// MaxCraftingQuotes quotes, the lowest ids are kept (the contract does not
// say which to drop; this keeps the result stable between uploads).
func craftingServices(v any) map[string]any {
	in, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	at, ok := nonNegInt(in["recordedAt"])
	if !ok {
		return nil
	}
	vals, ok := in["value"].(map[string]any)
	if !ok {
		return nil
	}
	profs := map[string]any{}
	for _, id := range sortedIDs(vals) {
		if len(profs) == MaxCraftingServices {
			break
		}
		if p := craftingService(vals[id]); p != nil {
			profs[id] = p
		}
	}
	if len(profs) == 0 {
		return nil
	}
	return map[string]any{"recordedAt": at, "value": profs}
}

// craftingService is one profession; nil if a listed key has the wrong
// type. A recipe quote with a bad id or price is dropped alone.
func craftingService(v any) map[string]any {
	in, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if a, present := in["active"]; present {
		b, ok := a.(bool)
		if !ok {
			return nil
		}
		out["active"] = b
	}
	if f, present := in["fee"]; present {
		n, ok := nonNegInt(f)
		if !ok {
			return nil
		}
		out["fee"] = n
	}
	if r, present := in["recipes"]; present {
		rm, ok := r.(map[string]any)
		if !ok {
			return nil
		}
		quotes := map[string]any{}
		for _, id := range sortedIDs(rm) {
			if len(quotes) == MaxCraftingQuotes {
				break
			}
			if n, ok := nonNegInt(rm[id]); ok {
				quotes[id] = n
			}
		}
		if len(quotes) > 0 {
			out["recipes"] = quotes
		}
	}
	return out
}

// craftingOrders keeps orders with a valid id and at least one valid half
// (w, j). Over MaxCraftingOrders, the closed orders with the oldest
// closedAt go first, then (if still too many) the open ones with the
// oldest w.created (recordedAt is the author's whole record, not the
// order, so it says nothing about the order's age).
func craftingOrders(v any) map[string]any {
	in, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	type kept struct {
		id     string
		order  map[string]any
		closed bool
		at     int64 // closedAt for closed orders, else w.created (0 if unknown)
	}
	var list []kept
	for id, o := range in {
		if !orderIDRE.MatchString(id) {
			continue
		}
		order := craftingOrder(o)
		if order == nil {
			continue
		}
		k := kept{id: id, order: order}
		for _, half := range []string{"w", "j"} {
			h, _ := order[half].(map[string]any)
			if c, ok := h["closedAt"].(int64); ok && c >= k.at {
				k.closed, k.at = true, c
			}
		}
		if !k.closed {
			w, _ := order["w"].(map[string]any)
			k.at, _ = w["created"].(int64)
		}
		list = append(list, k)
	}
	if len(list) > MaxCraftingOrders {
		// Keep: open before closed, then newest first; ties by id.
		sort.Slice(list, func(i, j int) bool {
			a, b := list[i], list[j]
			if a.closed != b.closed {
				return !a.closed
			}
			if a.at != b.at {
				return a.at > b.at
			}
			return a.id < b.id
		})
		list = list[:MaxCraftingOrders]
	}
	if len(list) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, k := range list {
		out[k.id] = k.order
	}
	return out
}

// craftingOrder is { w, j }. An invalid half is dropped alone and the
// other one kept; nil when neither half is valid.
func craftingOrder(v any) map[string]any {
	in, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	for half, spec := range map[string]map[string]fieldSpec{"w": customerHalf, "j": crafterHalf} {
		h, present := in[half]
		if !present {
			continue
		}
		if clean := craftingHalf(h, spec); clean != nil {
			out[half] = clean
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// craftingHalf keeps the listed keys of one half; nil (the half is
// dropped) if one of them has the wrong type or recordedAt is missing.
func craftingHalf(v any, spec map[string]fieldSpec) map[string]any {
	in, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	if _, ok := nonNegInt(in["recordedAt"]); !ok {
		return nil
	}
	out := map[string]any{}
	for k, s := range spec {
		raw, present := in[k]
		if !present {
			continue
		}
		val, ok := s.check(raw)
		if !ok {
			return nil
		}
		out[k] = val
	}
	return out
}

func (s fieldSpec) check(v any) (any, bool) {
	switch s.kind {
	case fNum:
		return nonNegInt(v)
	case fPositive:
		n, ok := nonNegInt(v)
		return n, ok && n >= 1
	case fGUID:
		g, ok := v.(string)
		return g, ok && guidRE.MatchString(g)
	case fGUIDStar:
		g, ok := v.(string)
		return g, ok && (g == "*" || guidRE.MatchString(g))
	case fBool:
		b, ok := v.(bool)
		return b, ok
	case fEnum:
		e, ok := v.(string)
		return e, ok && s.enum[e]
	case fCounts:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		out := map[string]any{}
		for id, n := range m {
			c, ok := nonNegInt(n)
			if !ok || !isSkillLineID(id) {
				return nil, false
			}
			out[id] = c
		}
		return out, true
	}
	return nil, false
}

// nonNegInt accepts integers >= 0 (a float only when it is a whole number
// below 2^53, as Lua may write large numbers).
func nonNegInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, n >= 0
	case float64:
		if n >= 0 && n < 1<<53 && n == math.Trunc(n) {
			return int64(n), true
		}
	}
	return 0, false
}

// sortedIDs returns the decimal-id keys of m in numeric order; other keys
// are left out.
func sortedIDs(m map[string]any) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		if isSkillLineID(id) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, _ := strconv.ParseUint(ids[i], 10, 64)
		b, _ := strconv.ParseUint(ids[j], 10, 64)
		if a != b {
			return a < b
		}
		return ids[i] < ids[j]
	})
	return ids
}
