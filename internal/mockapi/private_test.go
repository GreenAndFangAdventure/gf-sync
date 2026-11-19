package mockapi

import "testing"

func TestHasPrivateField(t *testing.T) {
	for _, c := range []struct {
		kind, raw string
		want      bool
	}{
		{"character.self", `{"payload":{"level":1}}`, false},
		{"character.self", `{"payload":{"location":{"zone":"z","subZone":"s"}}}`, false},
		{"character.self", `{"payload":{"location":{"zone":"z"}}}`, false},
		{"character.self", `{"payload":{"location":{"zone":"z","x":1}}}`, true},
		{"character.self", `{"payload":{"location":{"subZone":"s"}}}`, true},
		{"character.self", `{"payload":{"zone":"z"}}`, true},
		{"character.self", `{"location":{"zone":"z"}}`, true},
		{"character.self", `{"payload":{"online":true}}`, true},
		{"roster.snapshot", `{"payload":{"location":{"zone":"z"}}}`, true},
		{"roster.snapshot", `{"payload":{"members":[{"location":{"zone":"z"}}]}}`, true},
		{"roster.snapshot", `{"payload":{"members":[{"online":false}]}}`, true},
	} {
		if got := hasPrivateField([]byte(c.raw), c.kind); got != c.want {
			t.Errorf("%s %s: got %v", c.kind, c.raw, got)
		}
	}
}
