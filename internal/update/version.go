package update

import (
	"strconv"
	"strings"
)

// Compare orders versions "MAJOR.MINOR.PATCH[-pre][+build]": -1, 0 or +1.
// A pre-release is older than its release ("0.2.0-beta1" < "0.2.0"), and
// pre-release labels compare as plain strings. Anything unparsable counts
// as 0.0.0, so a broken version never looks newer.
func Compare(a, b string) int {
	ac, ap := split(a)
	bc, bp := split(b)
	for i := 0; i < 3; i++ {
		if ac[i] != bc[i] {
			if ac[i] < bc[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case ap == bp:
		return 0
	case ap == "":
		return 1
	case bp == "":
		return -1
	case ap < bp:
		return -1
	}
	return 1
}

func split(v string) ([3]int, string) {
	var n [3]int
	v, _, _ = strings.Cut(strings.TrimPrefix(strings.TrimSpace(v), "v"), "+")
	core, pre, _ := strings.Cut(v, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return [3]int{}, ""
	}
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 {
			return [3]int{}, ""
		}
		n[i] = x
	}
	return n, pre
}
