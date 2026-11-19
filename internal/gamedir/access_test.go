package gamedir

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// denyUnder makes every stat / readdir at or below dir fail the way
// Windows does when security software blocks the program.
func denyUnder(t *testing.T, dir string, code windows.Errno) {
	t.Helper()
	oldStat, oldRead := osStat, osReadDir
	t.Cleanup(func() { osStat, osReadDir = oldStat, oldRead })
	blocked := func(p string) bool {
		p, d := strings.ToLower(filepath.Clean(p)), strings.ToLower(filepath.Clean(dir))
		return p == d || strings.HasPrefix(p, d+string(filepath.Separator))
	}
	osStat = func(p string) (os.FileInfo, error) {
		if blocked(p) {
			return nil, &fs.PathError{Op: "GetFileAttributesEx", Path: p, Err: code}
		}
		return oldStat(p)
	}
	osReadDir = func(p string) ([]os.DirEntry, error) {
		if blocked(p) {
			return nil, &fs.PathError{Op: "open", Path: p, Err: code}
		}
		return oldRead(p)
	}
}

func TestIsAccessDenied(t *testing.T) {
	for _, c := range []windows.Errno{windows.ERROR_ACCESS_DENIED, windows.ERROR_VIRUS_INFECTED, windows.ERROR_ACCESS_DISABLED_BY_POLICY} {
		if !IsAccessDenied(&fs.PathError{Op: "open", Path: "x", Err: c}) {
			t.Errorf("errno %d should count as denied", c)
		}
	}
	for _, err := range []error{nil, os.ErrNotExist, &fs.PathError{Op: "open", Path: "x", Err: windows.ERROR_FILE_NOT_FOUND}, &fs.PathError{Op: "open", Path: "x", Err: windows.ERROR_PATH_NOT_FOUND}} {
		if IsAccessDenied(err) {
			t.Errorf("%v is not access denied", err)
		}
	}
}

func TestDetectDeniedIsNotNotFound(t *testing.T) {
	root, forever := mkFake(t)
	missing := filepath.Join(t.TempDir(), "nothing here", "World of Warcraft")

	// Readable: found.
	if f, err := DetectIn([]string{missing, root}); f != forever || err != nil {
		t.Fatalf("readable: %q %v", f, err)
	}
	// Really not there: "" and no error.
	if f, err := DetectIn([]string{missing}); f != "" || err != nil {
		t.Fatalf("missing: %q %v", f, err)
	}
	// The folder exists but security software blocks reading it.
	denyUnder(t, forever, windows.ERROR_ACCESS_DENIED)
	f, err := DetectIn([]string{missing, root})
	if f != "" || !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("denied: %q %v", f, err)
	}
	if _, err := Check(forever); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("Check of a saved folder: %v", err)
	}
	if Normalize(forever) != "" {
		t.Fatal("Normalize still returns only usable folders")
	}
}

func TestScanSavedVariablesDenied(t *testing.T) {
	_, forever := mkFake(t)
	if files, err := ScanSavedVariables(forever); len(files) != 2 || err != nil {
		t.Fatalf("%v %v", files, err)
	}
	// Game folder without WTF\Account yet: no files, no error.
	empty := filepath.Join(t.TempDir(), ForeverDir)
	os.MkdirAll(filepath.Join(empty, "WTF"), 0o755)
	if files, err := ScanSavedVariables(empty); len(files) != 0 || err != nil {
		t.Fatalf("empty: %v %v", files, err)
	}
	// One account's file is blocked: the other is still returned.
	denyUnder(t, filepath.Join(forever, "WTF", "Account", "ACCOUNT3"), windows.ERROR_VIRUS_INFECTED)
	files, err := ScanSavedVariables(forever)
	if len(files) != 1 || AccountOf(files[0]) != "ACCOUNT1" || !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("partial: %v %v", files, err)
	}
	// The whole Account folder is blocked.
	denyUnder(t, filepath.Join(forever, "WTF"), windows.ERROR_ACCESS_DENIED)
	if files, err := ScanSavedVariables(forever); len(files) != 0 || !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("all: %v %v", files, err)
	}
}
