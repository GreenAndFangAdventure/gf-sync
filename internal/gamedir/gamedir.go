// Package gamedir finds the World of Warcraft: Forever folder (`_forever_`)
// and the GreenFangToolkit SavedVariables files under it. Read-only.
package gamedir

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gfsync/internal/svdoc"
)

// ForeverDir is the client folder for World of Warcraft: Forever.
const ForeverDir = "_forever_"

// Normalize accepts a folder the player picked: the WoW root, `_forever_`
// itself, or something inside `_forever_` (WTF, Interface…). It returns the
// `_forever_` path, or "" when the folder does not look like one.
func Normalize(dir string) string {
	if dir == "" {
		return ""
	}
	dir = filepath.Clean(dir)
	for i := 0; i < 4 && dir != ""; i++ {
		if strings.EqualFold(filepath.Base(dir), ForeverDir) && looksLikeForever(dir) {
			return dir
		}
		if f := filepath.Join(dir, ForeverDir); looksLikeForever(f) {
			return f
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func looksLikeForever(dir string) bool {
	if st, err := os.Stat(filepath.Join(dir, "WTF")); err == nil && st.IsDir() {
		return true
	}
	if st, err := os.Stat(filepath.Join(dir, "Wow.exe")); err == nil && !st.IsDir() {
		return true
	}
	return false
}

// CommonRoots lists the usual WoW install folders on every fixed drive.
func CommonRoots(drives []string) []string {
	var out []string
	for _, d := range drives {
		d = strings.TrimRight(d, `\/:`) + `:\`
		out = append(out,
			filepath.Join(d, "World of Warcraft"),
			filepath.Join(d, "Program Files (x86)", "World of Warcraft"),
			filepath.Join(d, "Program Files", "World of Warcraft"),
			filepath.Join(d, "Games", "World of Warcraft"),
			filepath.Join(d, "Battle.net", "World of Warcraft"),
			filepath.Join(d, "Blizzard", "World of Warcraft"),
		)
	}
	return out
}

// Detect returns the first `_forever_` found among registry hints and
// common folders, or "".
func Detect() string {
	for _, c := range append(registryHints(), CommonRoots(fixedDrives())...) {
		if f := Normalize(c); f != "" {
			return f
		}
	}
	return ""
}

// SavedVariablesFiles returns every
// `<forever>/WTF/Account/*/SavedVariables/GreenFangToolkit.lua`, sorted.
func SavedVariablesFiles(forever string) []string {
	pattern := filepath.Join(forever, "WTF", "Account", "*", "SavedVariables", svdoc.FileName)
	m, _ := filepath.Glob(pattern)
	sort.Strings(m)
	return m
}

// AccountOf returns the account folder name for a SavedVariables path.
func AccountOf(svPath string) string {
	return filepath.Base(filepath.Dir(filepath.Dir(svPath)))
}
