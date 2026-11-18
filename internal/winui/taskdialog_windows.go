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

// DialogButton is an extra command button. Pressing it calls OnClick on the
// dialog's own thread and keeps the dialog open; a non-empty return value
// replaces the footer text (for example 「已複製」). Do slow work in a
// goroutine.
type DialogButton struct {
	Label   string
	OnClick func() (footer string)
}

// Dialog is a Windows task dialog with up to two extra command buttons and
// a 「關閉」 button. Close dismisses it from another goroutine (for example
// when pairing finishes).
//
// Task dialogs need Common Controls v6, which GFSync.exe's manifest asks
// for. Without it (go run, tests) Show falls back to message boxes.
type Dialog struct {
	Title       string
	Instruction string // large first line
	Content     string
	Footer      string // small text at the bottom; must be non-empty to be updatable
	Buttons     []DialogButton

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
	procSendMessage      = user32.NewProc("SendMessageW")

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
	firstButtonID    = 100

	tdnCreated       = 0
	tdnButtonClicked = 2
	tdnDestroyed     = 5

	tdmSetElementText = 0x0400 + 108 // TDM_SET_ELEMENT_TEXT
	tdeFooter         = 2

	wmClose      = 0x0010
	hwndTopmost  = ^uintptr(0) // (HWND)-1
	swpNoSize    = 0x0001
	swpNoMove    = 0x0002
	swpShowWin   = 0x0040
	sFalse       = 1
	tdConfigSize = 160 // sizeof(TASKDIALOGCONFIG) on amd64 (packed)
	tdButtonSize = 12  // sizeof(TASKDIALOG_BUTTON) on amd64 (packed)
)

// Show blocks until the dialog is closed.
func (d *Dialog) Show() {
	if d.isClosed() {
		return
	}
	if err := d.showTaskDialog(); err != nil {
		d.showFallback()
	}
}

// showFallback uses message boxes: 是 = first button, 否 = second button.
func (d *Dialog) showFallback() {
	for !d.isClosed() {
		text := d.Instruction + "\n\n" + d.Content
		if d.Footer != "" {
			text += "\n\n" + d.Footer
		}
		var choice int
		switch len(d.Buttons) {
		case 0:
			Info(d.Title, text)
			return
		case 1:
			if YesNo(d.Title, text+"\n\n按「是」"+d.Buttons[0].Label+"，按「否」關閉。") {
				choice = 0
			} else {
				return
			}
		default:
			switch yesNoCancel(d.Title, text+"\n\n按「是」"+d.Buttons[0].Label+"，按「否」"+d.Buttons[1].Label+"，按「取消」關閉。") {
			case idYes:
				choice = 0
			case idNo:
				choice = 1
			default:
				return
			}
		}
		if d.isClosed() {
			return
		}
		if b := d.Buttons[choice]; b.OnClick != nil {
			if f := b.OnClick(); f != "" {
				d.Footer = f
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
	d                             *Dialog
	cfg                           [tdConfigSize]byte
	btns                          []byte
	title, instr, content, footer []uint16
	labels                        [][]uint16
	newFooter                     []uint16 // last footer set by a button
}

func ptr(u []uint16) uint64 {
	if len(u) == 0 {
		return 0
	}
	return uint64(uintptr(unsafe.Pointer(&u[0])))
}

func (d *Dialog) showTaskDialog() error {
	if procTaskDialogIndir.Find() != nil {
		return errors.New("TaskDialogIndirect not available")
	}
	t := &tdCall{d: d}
	t.title, _ = syscall.UTF16FromString(d.Title)
	t.instr, _ = syscall.UTF16FromString(d.Instruction)
	t.content, _ = syscall.UTF16FromString(d.Content)
	if d.Footer != "" {
		t.footer, _ = syscall.UTF16FromString(d.Footer)
	}
	le := binary.LittleEndian

	// TASKDIALOG_BUTTON[], packed: int nButtonID; PCWSTR pszButtonText.
	t.btns = make([]byte, tdButtonSize*len(d.Buttons)+1)
	for i, b := range d.Buttons {
		l, _ := syscall.UTF16FromString(b.Label)
		t.labels = append(t.labels, l)
		le.PutUint32(t.btns[i*tdButtonSize:], uint32(firstButtonID+i))
		le.PutUint64(t.btns[i*tdButtonSize+4:], ptr(l))
	}

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
	c := &t.cfg
	le.PutUint32(c[0:], tdConfigSize)            // cbSize
	le.PutUint32(c[20:], tdfAllowCancel)         // dwFlags
	le.PutUint32(c[24:], tdcbfCloseButton)       // dwCommonButtons
	le.PutUint64(c[28:], ptr(t.title))           // pszWindowTitle
	le.PutUint64(c[36:], tdIconInfo)             // pszMainIcon
	le.PutUint64(c[44:], ptr(t.instr))           // pszMainInstruction
	le.PutUint64(c[52:], ptr(t.content))         // pszContent
	le.PutUint32(c[60:], uint32(len(d.Buttons))) // cButtons
	if len(d.Buttons) > 0 {
		le.PutUint64(c[64:], uint64(uintptr(unsafe.Pointer(&t.btns[0])))) // pButtons
	}
	le.PutUint32(c[72:], idClose)        // nDefaultButton
	le.PutUint64(c[132:], ptr(t.footer)) // pszFooter
	le.PutUint64(c[140:], uint64(tdCB))  // pfCallback
	le.PutUint64(c[148:], uint64(ref))   // lpCallbackData

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
		i := int(wParam) - firstButtonID
		if i < 0 || i >= len(d.Buttons) {
			return 0
		}
		if f := d.Buttons[i].OnClick; f != nil {
			if text := f(); text != "" && len(t.footer) > 0 {
				t.newFooter, _ = syscall.UTF16FromString(text)
				procSendMessage.Call(hwnd, tdmSetElementText, tdeFooter, uintptr(unsafe.Pointer(&t.newFooter[0])))
			}
		}
		return sFalse // keep the dialog open
	case tdnDestroyed:
		d.mu.Lock()
		d.hwnd = 0
		d.mu.Unlock()
	}
	return 0
}
