package winui

import (
	"errors"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Notify shows a one-off Windows notification (a tray balloon, which
// Windows 10/11 shows as a normal notification) on GF Sync's own tray icon.
//
// fyne.io/systray does not expose notifications, so this finds the tray's
// hidden window in this process (class "SystrayClass", icon ID 100 — see
// systray_windows.go in fyne.io/systray v1.12.x) and adds NIF_INFO to the
// existing icon. Nothing else about the icon changes. If the window is not
// found (for example a different systray version), it returns an error and
// the caller simply stays quiet.
func Notify(title, text string) error {
	hwnd := findTrayWindow()
	if hwnd == 0 {
		return errors.New("tray window not found")
	}
	nid := notifyIconData{
		Wnd:       hwnd,
		ID:        systrayIconID,
		Flags:     nifInfo,
		InfoFlags: niifInfo | niifRespectQuietTime,
	}
	nid.Size = uint32(unsafe.Sizeof(nid))
	copyUTF16(nid.Info[:], text)
	copyUTF16(nid.InfoTitle[:], title)
	r, _, err := procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
	if r == 0 {
		return err
	}
	return nil
}

const (
	systrayClass  = "SystrayClass"
	systrayIconID = 100

	nimModify            = 0x1
	nifInfo              = 0x10
	niifInfo             = 0x1
	niifRespectQuietTime = 0x80
)

var (
	procShellNotifyIcon  = shell32.NewProc("Shell_NotifyIconW")
	procEnumWindows      = user32.NewProc("EnumWindows")
	procGetClassName     = user32.NewProc("GetClassNameW")
	procGetWindowThreadP = user32.NewProc("GetWindowThreadProcessId")
)

// NOTIFYICONDATAW (Vista and later layout).
type notifyIconData struct {
	Size            uint32
	Wnd             windows.Handle
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            windows.Handle
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Timeout         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        windows.GUID
	BalloonIcon     windows.Handle
}

func copyUTF16(dst []uint16, s string) {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		return
	}
	if len(u) > len(dst) {
		u = u[:len(dst)]
		u[len(u)-1] = 0
	}
	copy(dst, u)
}

// findTrayWindow returns the systray window that belongs to this process.
func findTrayWindow() windows.Handle {
	pid := uint32(os.Getpid())
	var found windows.Handle
	cb := syscall.NewCallback(func(h windows.Handle, _ uintptr) uintptr {
		var wpid uint32
		procGetWindowThreadP.Call(uintptr(h), uintptr(unsafe.Pointer(&wpid)))
		if wpid != pid {
			return 1
		}
		var buf [64]uint16
		n, _, _ := procGetClassName.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n > 0 && syscall.UTF16ToString(buf[:n]) == systrayClass {
			found = h
			return 0
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return found
}
