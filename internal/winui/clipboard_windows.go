package winui

import (
	"errors"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procMoveMemory       = kernel32.NewProc("RtlMoveMemory")
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// CopyText puts s on the clipboard as plain Unicode text (nothing else).
func CopyText(s string) error {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		return err
	}
	// Another program may hold the clipboard for a moment.
	opened := false
	for i := 0; i < 10; i++ {
		if r, _, _ := procOpenClipboard.Call(0); r != 0 {
			opened = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !opened {
		return errors.New("clipboard is busy")
	}
	defer procCloseClipboard.Call()
	if r, _, err := procEmptyClipboard.Call(); r == 0 {
		return err
	}
	size := uintptr(len(u) * 2)
	h, _, err := procGlobalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return err
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return err
	}
	procMoveMemory.Call(p, uintptr(unsafe.Pointer(&u[0])), size)
	procGlobalUnlock.Call(h)
	if r, _, err := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		procGlobalFree.Call(h)
		return err
	}
	return nil // the clipboard owns h now
}
