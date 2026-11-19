package gamedir

import (
	"errors"
	"io/fs"
	"os"
)

// ErrAccessDenied means a folder or file exists but GF Sync may not read it,
// most often because security software is restricting the program. It is
// reported separately from "not found" so the player is not asked to pick a
// folder they already have.
var ErrAccessDenied = errors.New("access denied")

// deniedError keeps the path for the log; errors.Is(err, ErrAccessDenied).
type deniedError struct {
	Path string
	Err  error
}

func (e *deniedError) Error() string   { return "access denied: " + e.Path + ": " + e.Err.Error() }
func (e *deniedError) Unwrap() []error { return []error{ErrAccessDenied, e.Err} }

// IsAccessDenied reports whether err is a permission error (including the
// Windows codes security software returns), as opposed to "does not exist".
func IsAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrAccessDenied) || errors.Is(err, fs.ErrPermission) {
		return true
	}
	return isDeniedErrno(err)
}

// Replaced in tests to simulate security software refusing access.
var (
	osStat    = os.Stat
	osReadDir = os.ReadDir
)

// stat is os.Stat that turns permission errors into ErrAccessDenied and
// everything else (not found, bad name…) into plain "missing".
func stat(path string) (os.FileInfo, error) {
	st, err := osStat(path)
	if err != nil && IsAccessDenied(err) {
		return nil, &deniedError{Path: path, Err: err}
	}
	return st, err
}
