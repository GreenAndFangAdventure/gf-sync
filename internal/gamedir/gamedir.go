// Package gamedir finds the World of Warcraft: Forever folder (`_forever_`)
// and the GreenFangToolkit SavedVariables files under it. Read-only.
package gamedir

import (
	"path/filepath"
	"sort"
	"strings"

	"gfsync/internal/svdoc"
)

// ForeverDir is the client folder for World of Warcraft: Forever.
const ForeverDir = "_forever_"

// Normalize accepts a folder the player picked: the WoW root, `_forever_`
// itself, or something inside `_forever_` (WTF, Interface…). It returns the
// `_forever_` path, or "" when the folder does not look like one (or
// cannot be read; use Check to tell the two apart).
func Normalize(dir string) string {
	f, _ := Check(dir)
	return f
}

// Check is Normalize with a reason. When no `_forever_` is found and some
// candidate path exists but could not be read, err satisfies
// errors.Is(err, ErrAccessDenied). Otherwise err is nil.
func Check(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	var denied error
	note := func(err error) {
		if denied == nil && IsAccessDenied(err) {
			denied = err
		}
	}
	dir = filepath.Clean(dir)
	if _, err := stat(dir); err != nil {
		note(err)
	}
	for i := 0; i < 4 && dir != ""; i++ {
		if strings.EqualFold(filepath.Base(dir), ForeverDir) {
			ok, err := looksLikeForever(dir)
			if ok {
				return dir, nil
			}
			note(err)
		}
		f := filepath.Join(dir, ForeverDir)
		ok, err := looksLikeForever(f)
		if ok {
			return f, nil
		}
		note(err)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", denied
}

// looksLikeForever reports whether dir has WTF or Wow.exe. err is an
// access-denied error when one of them (or dir) exists but cannot be read.
func looksLikeForever(dir string) (bool, error) {
	var denied error
	if _, err := stat(dir); err != nil {
		return false, deniedOnly(err)
	}
	st, err := stat(filepath.Join(dir, "WTF"))
	if err == nil && st.IsDir() {
		return true, nil
	}
	if denied == nil {
		denied = deniedOnly(err)
	}
	st, err = stat(filepath.Join(dir, "Wow.exe"))
	if err == nil && !st.IsDir() {
		return true, nil
	}
	if denied == nil {
		denied = deniedOnly(err)
	}
	return false, denied
}

func deniedOnly(err error) error {
	if IsAccessDenied(err) {
		return err
	}
	return nil
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
// common folders. When none is found but a candidate exists and could not
// be read, it returns "" and an ErrAccessDenied error; "" and nil means
// the game folder really is not in any usual place.
func Detect() (string, error) {
	return DetectIn(append(registryHints(), CommonRoots(fixedDrives())...))
}

// DetectIn is Detect over the given candidates.
func DetectIn(candidates []string) (string, error) {
	var denied error
	for _, c := range candidates {
		f, err := Check(c)
		if f != "" {
			return f, nil
		}
		if denied == nil {
			denied = err
		}
	}
	return "", denied
}

// SavedVariablesFiles returns every
// `<forever>/WTF/Account/*/SavedVariables/GreenFangToolkit.lua`, sorted.
func SavedVariablesFiles(forever string) []string {
	m, _ := ScanSavedVariables(forever)
	return m
}

// ScanSavedVariables is SavedVariablesFiles with a reason: err satisfies
// errors.Is(err, ErrAccessDenied) when the folder or a file exists but
// cannot be read. Files that could be found are still returned. A missing
// WTF\Account folder (the game never ran) is not an error.
func ScanSavedVariables(forever string) ([]string, error) {
	accounts := filepath.Join(forever, "WTF", "Account")
	ents, err := osReadDir(accounts)
	if err != nil {
		if IsAccessDenied(err) {
			return nil, &deniedError{Path: accounts, Err: err}
		}
		if _, serr := stat(forever); serr != nil && IsAccessDenied(serr) {
			return nil, serr
		}
		return nil, nil
	}
	var out []string
	var denied error
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(accounts, e.Name(), "SavedVariables", svdoc.FileName)
		st, err := stat(p)
		switch {
		case err == nil && !st.IsDir():
			out = append(out, p)
		case err != nil && denied == nil && IsAccessDenied(err):
			denied = err
		}
	}
	sort.Strings(out)
	return out, denied
}

// AccountOf returns the account folder name for a SavedVariables path.
func AccountOf(svPath string) string {
	return filepath.Base(filepath.Dir(filepath.Dir(svPath)))
}
