// Package winui holds the few Win32 calls the tray app needs: message
// boxes, opening a URL or folder, a folder picker and a single-instance
// mutex. No cgo.
package winui

import (
	"errors"
	"net/url"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	shell32          = windows.NewLazySystemDLL("shell32.dll")
	ole32            = windows.NewLazySystemDLL("ole32.dll")
	procTaskMemFree  = ole32.NewProc("CoTaskMemFree")
	procMessageBoxW  = user32.NewProc("MessageBoxW")
	procBrowseFolder = shell32.NewProc("SHBrowseForFolderW")
	procPathFromID   = shell32.NewProc("SHGetPathFromIDListW")
)

const (
	mbOK          = 0x0
	mbYesNo       = 0x4
	mbIconInfo    = 0x40
	mbIconWarning = 0x30
	mbTopMost     = 0x40000
	mbSetFg       = 0x10000
	idYes         = 6
	idNo          = 7
	mbYesNoCancel = 0x3
)

func msg(text, title string, flags uintptr) int {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(title)
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags|mbTopMost|mbSetFg)
	return int(r)
}

// Info shows an information box (blocks the calling goroutine).
func Info(title, text string) { msg(text, title, mbOK|mbIconInfo) }

// Warn shows a warning box.
func Warn(title, text string) { msg(text, title, mbOK|mbIconWarning) }

// YesNo asks a question.
func YesNo(title, text string) bool { return msg(text, title, mbYesNo|mbIconInfo) == idYes }

func yesNoCancel(title, text string) int { return msg(text, title, mbYesNoCancel|mbIconInfo) }

// OpenURL opens an http(s) URL in the default browser. Other schemes are
// refused so a URL from the network can never launch a program.
func OpenURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("winui: only http(s) URLs can be opened")
	}
	return shellOpen(u.String())
}

// OpenFolder opens a folder in Explorer.
func OpenFolder(path string) error { return shellOpen(path) }

func shellOpen(target string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	t, _ := windows.UTF16PtrFromString(target)
	return windows.ShellExecute(0, verb, t, nil, nil, windows.SW_SHOWNORMAL)
}

type browseInfo struct {
	Owner       uintptr
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	Callback    uintptr
	LParam      uintptr
	Image       int32
}

// PickFolder shows the Windows folder picker. ok is false on cancel.
func PickFolder(title string) (path string, ok bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil {
		defer windows.CoUninitialize()
	}
	display := make([]uint16, windows.MAX_PATH)
	t, _ := windows.UTF16PtrFromString(title)
	bi := browseInfo{
		DisplayName: &display[0],
		Title:       t,
		Flags:       0x1 | 0x40 | 0x200, // RETURNONLYFSDIRS | NEWDIALOGSTYLE | NONEWFOLDERBUTTON
	}
	pidl, _, _ := procBrowseFolder.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", false
	}
	defer procTaskMemFree.Call(pidl)
	buf := make([]uint16, 32768)
	r, _, _ := procPathFromID.Call(pidl, uintptr(unsafe.Pointer(&buf[0])))
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

// SingleInstance returns false if another GF Sync is already running in
// this Windows session. Keep the returned handle open for the process life.
func SingleInstance(name string) (windows.Handle, bool) {
	n, _ := windows.UTF16PtrFromString(name)
	h, err := windows.CreateMutex(nil, false, n)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return h, false
	}
	return h, true
}
