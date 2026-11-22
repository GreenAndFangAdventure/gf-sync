package svdoc

import "strings"

// sharePayload rebuilds a member.share payload from the whitelist in
// savedvariables §3.8. Nothing that is not listed there is kept, whatever
// the AddOn wrote. ok is false when the subject has no valid name (the
// entry is then malformed and skipped).
//
//   - subject: name (required), realm, guid, fullName.
//   - level, classToken, professions, location: { value, recordedAt, via }.
//     A field without a valid value or a numeric recordedAt is dropped.
//   - recipes[<skillLineID>]: { recordedAt, via, hash, ids }.
//   - withdrawn: { all, location, recipes[<skillLineID>] }, all numbers.
//     withdrawn.location is a time, not a place. Since D-015 the AddOn
//     only writes recipes; all and location may still come from older
//     AddOns and are passed on for the site to decide.
func sharePayload(v any) (map[string]any, bool) {
	in, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	subj := subjectOf(in["subject"])
	if subj == nil {
		return nil, false
	}
	out := map[string]any{"subject": subj}
	fields := []struct {
		key   string
		value func(any) (any, bool)
	}{
		{"level", numberValue},
		{"classToken", func(v any) (any, bool) { return boundedText(v, MaxSubjectLen) }},
		{"professions", professionsValue},
		{"location", func(v any) (any, bool) {
			l := ownLocation(v)
			return l, l != nil
		}},
	}
	for _, f := range fields {
		if field := shareField(in[f.key], f.value); field != nil {
			out[f.key] = field
		}
	}
	if r := shareRecipes(in["recipes"]); r != nil {
		out["recipes"] = r
	}
	if w := withdrawn(in["withdrawn"]); w != nil {
		out["withdrawn"] = w
	}
	return out, true
}

// shareField keeps { value, recordedAt, via } with a valid value and a
// numeric recordedAt; nil otherwise.
func shareField(v any, value func(any) (any, bool)) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	at, ok := numberValue(m["recordedAt"])
	if !ok {
		return nil
	}
	val, ok := value(m["value"])
	if !ok {
		return nil
	}
	out := map[string]any{"value": val, "recordedAt": at}
	keepVia(m, out)
	return out
}

// keepVia copies via only when it is "author" or "relay".
func keepVia(in, out map[string]any) {
	if via, _ := in["via"].(string); via == "author" || via == "relay" {
		out["via"] = via
	}
}

func numberValue(v any) (any, bool) {
	switch v.(type) {
	case int64, float64:
		return v, true
	}
	return nil, false
}

// professionsValue keeps skillLineID, name, rank and max of each
// profession. An empty list is a valid value (no professions).
func professionsValue(v any) (any, bool) {
	list, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := []any{}
	for _, p := range list {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		id, ok := numberValue(m["skillLineID"])
		if !ok {
			continue
		}
		q := map[string]any{"skillLineID": id}
		if name, ok := boundedText(m["name"], MaxSubjectLen); ok {
			q["name"] = name
		}
		for _, k := range []string{"rank", "max"} {
			if n, ok := numberValue(m[k]); ok {
				q[k] = n
			}
		}
		out = append(out, q)
	}
	return out, true
}

// MaxHashLen bounds the AddOn's recipe-list hash.
const MaxHashLen = 64

// shareRecipes keeps, per skillLineID, { recordedAt, via, hash, ids }.
// recordedAt is required; ids keeps only numbers. nil when nothing is left.
func shareRecipes(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	for id, r := range m {
		rm, ok := r.(map[string]any)
		if !ok || !isSkillLineID(id) {
			continue
		}
		at, ok := numberValue(rm["recordedAt"])
		if !ok {
			continue
		}
		q := map[string]any{"recordedAt": at}
		keepVia(rm, q)
		if h, ok := boundedText(rm["hash"], MaxHashLen); ok {
			q["hash"] = h
		}
		if ids, ok := rm["ids"].([]any); ok {
			nums := []any{}
			for _, n := range ids {
				if _, ok := numberValue(n); ok {
					nums = append(nums, n)
				}
			}
			q["ids"] = nums
		}
		out[id] = q
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// withdrawn keeps the tombstone times; nil when nothing is left.
func withdrawn(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	for _, k := range []string{"all", "location"} {
		if n, ok := numberValue(m[k]); ok {
			out[k] = n
		}
	}
	if r, ok := m["recipes"].(map[string]any); ok {
		rs := map[string]any{}
		for id, t := range r {
			if n, ok := numberValue(t); ok && isSkillLineID(id) {
				rs[id] = n
			}
		}
		if len(rs) > 0 {
			out["recipes"] = rs
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// isSkillLineID: the string keys of recipe tables are decimal ids.
func isSkillLineID(s string) bool {
	return s != "" && len(s) <= 10 && strings.Trim(s, "0123456789") == ""
}
