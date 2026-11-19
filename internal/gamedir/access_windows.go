package gamedir

import (
	"errors"

	"golang.org/x/sys/windows"
)

// Codes Windows and security software return when a program is not
// allowed to open something that exists.
var deniedErrnos = []windows.Errno{
	windows.ERROR_ACCESS_DENIED,             // 5
	windows.ERROR_VIRUS_INFECTED,            // 225
	windows.ERROR_VIRUS_DELETED,             // 226
	windows.ERROR_ACCESS_DISABLED_BY_POLICY, // 1260
	windows.Errno(1920),                     // ERROR_CANT_ACCESS_FILE
	windows.ERROR_PRIVILEGE_NOT_HELD,        // 1314
}

func isDeniedErrno(err error) bool {
	var en windows.Errno
	if !errors.As(err, &en) {
		return false
	}
	for _, d := range deniedErrnos {
		if en == d {
			return true
		}
	}
	return false
}
