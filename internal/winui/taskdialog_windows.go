package winui

import (
	"encoding/binary"
	"errors"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Dialog is a Windows task dialog with one extra command button (for
// example 「重新開啟配對頁」) and a 「關閉」 button. Pressing the extra
// button calls OnButton and keeps the dialog open. Close dismisses it from
// another goroutine (for example when pairing finishes).
//
// Task dialogs need Common Controls v6, which GFSync.exe's manifest asks
// for. Without it (go run, tests) Show falls back to a message box.
type Dialog struct {
	Title       string
	Instruction string // large first line
	Content     string
	Button      string // extra command button label
	OnButton    func()

	mu     sync.Mutex
	hwnd   uintptr
	closed bool
}

var (
	comctl32             = windows.NewLazySystemDLL("comctl32.dll")
	procTaskDialogIndir  = comctl32.NewProc("TaskDialogIndirect")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
	procSetForegroundWin = user32.NewProc("SetForegroundWindow")
	procPostMessage      = user32.NewProc("PostMessageW")

	dialogsMu sync.Mutex
	dialogs   = map[uintptr]*tdCall{}
	nextRef   uintptr
	tdCB      = syscall.NewCallback(taskDialogCallback)
)

const (
	tdfAllowCancel   = 0x0008
	tdcbfCloseButton = 0x0020
	tdIconInfo       = 0xFFFD // TD_INFORMATION_ICON = MAKEINTRESOURCEW(-3)
	idClose          = 8
	buttonID         = 100

	tdnCreated       = 0
	tdnButtonClicked = 2
	tdnDestroyed     = 5

	wmClose      = 0x0010
	hwndTopmost  = ^uintptr(0) // (HWND)-1
	swpNoSize    = 0x0001
	swpNoMove    = 0x0002
	swpShowWin   = 0x0040
	sFalse       = 1
	tdConfigSize = 160 // sizeof(TASKDIALOGCONFIG) on amd64 (packed)
)

// Show blocks until the dialog is closed.
func (d *Dialog) Show() {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()
	if err := d.showTaskDialog(); err != nil {
		// Fallback: a plain message box. Yes = the extra button.
		for {
			text := d.Instruction + "\n\n" + d.Content + "\n\n按「是」" + d.Button + "，按「否」關閉。"
			if !YesNo(d.Title, text) || d.isClosed() {
				return
			}
			if d.OnButton != nil {
				d.OnButton()
			}
		}
	}
}

// Close dismisses the dialog if it is showing, or stops it from showing.
func (d *Dialog) Close() {
	d.mu.Lock()
	d.closed = true
	h := d.hwnd
	d.mu.Unlock()
	if h != 0 {
		procPostMessage.Call(h, wmClose, 0, 0)
	}
}

func (d *Dialog) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

// tdCall holds everything Windows points into while the dialog is open.
// It lives in the dialogs map, so it is on the heap and never moves (the
// callback runs on this goroutine and could otherwise grow its stack).
type tdCall struct {
	d                            *Dialog
	cfg                          [tdConfigSize]byte
	btn                          [12]byte
	title, instr, content, label []uint16
}

func (d *Dialog) showTaskDialog() error {
	if procTaskDialogIndir.Find() != nil {
		return errors.New("TaskDialogIndirect not available")
	}
	t := &tdCall{d: d}
	t.title, _ = syscall.UTF16FromString(d.Title)
	t.instr, _ = syscall.UTF16FromString(d.Instruction)
	t.content, _ = syscall.UTF16FromString(d.Content)
	t.label, _ = syscall.UTF16FromString(d.Button)
	title, instr, content, label, btn, c := t.title, t.instr, t.content, t.label, &t.btn, &t.cfg

	// TASKDIALOG_BUTTON, packed: int nButtonID; PCWSTR pszButtonText.
	binary.LittleEndian.PutUint32(btn[0:], buttonID)
	binary.LittleEndian.PutUint64(btn[4:], uint64(uintptr(unsafe.Pointer(&label[0]))))

	dialogsMu.Lock()
	nextRef++
	ref := nextRef
	dialogs[ref] = t
	dialogsMu.Unlock()
	defer func() {
		dialogsMu.Lock()
		delete(dialogs, ref)
		dialogsMu.Unlock()
	}()

	// TASKDIALOGCONFIG, packed (pshpack1.h), amd64 offsets.
	le := binary.LittleEndian
	le.PutUint32(c[0:], tdConfigSize)                                  // cbSize
	le.PutUint32(c[20:], tdfAllowCancel)                               // dwFlags
	le.PutUint32(c[24:], tdcbfCloseButton)                             // dwCommonButtons
	le.PutUint64(c[28:], uint64(uintptr(unsafe.Pointer(&title[0]))))   // pszWindowTitle
	le.PutUint64(c[36:], tdIconInfo)                                   // pszMainIcon
	le.PutUint64(c[44:], uint64(uintptr(unsafe.Pointer(&instr[0]))))   // pszMainInstruction
	le.PutUint64(c[52:], uint64(uintptr(unsafe.Pointer(&content[0])))) // pszContent
	le.PutUint32(c[60:], 1)                                            // cButtons
	le.PutUint64(c[64:], uint64(uintptr(unsafe.Pointer(&btn[0]))))     // pButtons
	le.PutUint32(c[72:], idClose)                                      // nDefaultButton
	le.PutUint64(c[140:], uint64(tdCB))                                // pfCallback
	le.PutUint64(c[148:], uint64(ref))                                 // lpCallbackData

	var pressed int32
	hr, _, _ := procTaskDialogIndir.Call(uintptr(unsafe.Pointer(&c[0])), uintptr(unsafe.Pointer(&pressed)), 0, 0)
	runtime.KeepAlive(t)
	if int32(hr) < 0 {
		return errors.New("TaskDialogIndirect failed")
	}
	return nil
}

func taskDialogCallback(hwnd, msg, wParam, lParam, ref uintptr) uintptr {
	dialogsMu.Lock()
	t := dialogs[ref]
	dialogsMu.Unlock()
	if t == nil {
		return 0
	}
	d := t.d
	switch msg {
	case tdnCreated:
		d.mu.Lock()
		d.hwnd = hwnd
		closed := d.closed
		d.mu.Unlock()
		if closed {
			procPostMessage.Call(hwnd, wmClose, 0, 0)
			return 0
		}
		// GF Sync runs in the background, so Windows may put the dialog
		// behind the browser it just opened. Keep it on top.
		procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWin)
		procSetForegroundWin.Call(hwnd)
	case tdnButtonClicked:
		if wParam == buttonID {
			if d.OnButton != nil {
				go d.OnButton()
			}
			return sFalse // keep the dialog open
		}
	case tdnDestroyed:
		d.mu.Lock()
		d.hwnd = 0
		d.mu.Unlock()
	}
	return 0
}
