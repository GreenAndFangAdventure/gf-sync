package luasv

import (
	"sort"
	"strconv"
)

// ToJSON converts a parsed value into plain Go values suitable for
// encoding/json: tables whose keys are exactly 1..n become []any (an empty
// table becomes an empty array), every other table becomes map[string]any
// with integer keys written in decimal.
func ToJSON(v any) any {
	t, ok := v.(*Table)
	if !ok {
		return v
	}
	if t.IsArray() {
		arr := t.Array()
		for i := range arr {
			arr[i] = ToJSON(arr[i])
		}
		return arr
	}
	m := make(map[string]any, len(t.Str)+len(t.Int))
	keys := make([]int64, 0, len(t.Int))
	for k := range t.Int {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		m[strconv.FormatInt(k, 10)] = ToJSON(t.Int[k])
	}
	for k, val := range t.Str {
		m[k] = ToJSON(val)
	}
	return m
}
