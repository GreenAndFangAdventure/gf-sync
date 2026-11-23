package inbox

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// The only folder and files GF Sync ever writes in the game folder
// (D-018 item 1). They are constants: nothing the site sends is used to
// build a path.
const (
	Folder   = "GreenFangToolkit_Inbox"
	TOCName  = "GreenFangToolkit_Inbox.toc"
	LuaName  = "Inbox.lua"
	tmpExt   = ".gfsync-tmp" // next to the target, renamed over it
	luaStart = `GreenFangToolkit_Inbox = "`
	luaEnd   = "\"\n"
)

// TOC is the fixed .toc (draft §5, variant A: LoadOnDemand). GF Sync puts
// it back whenever it differs.
const TOC = "## Interface: 16001\n" +
	"## Title: GreenFangToolkit Inbox\n" +
	"## Notes: Data for GreenFangToolkit. Written by GF Sync.\n" +
	"## LoadOnDemand: 1\n" +
	LuaName + "\n"

// MaxFileLen bounds Inbox.lua: the string plus the fixed Lua around it.
const MaxFileLen = MaxLen + len(luaStart) + len(luaEnd)

var (
	ErrNoAddOns = errors.New("inbox: Interface/AddOns not found")
	ErrNotPlain = errors.New("inbox: not a plain folder or file (link or junction)")
	ErrReadBack = errors.New("inbox: file reads back different from what was written")
)

// LuaFile is the whole Inbox.lua for s: one line, the string in quotes.
// s must pass CheckChars, so it has no quote, backslash or newline.
func LuaFile(s string) []byte { return []byte(luaStart + s + luaEnd) }

// Writer keeps GreenFangToolkit_Inbox in one game folder (_forever_).
type Writer struct {
	Forever string
	// write writes b to f; tests replace it (e.g. a full disk).
	write func(f *os.File, b []byte) (int, error)
}

// NewWriter returns a writer for the game folder forever.
func NewWriter(forever string) *Writer {
	return &Writer{Forever: forever, write: func(f *os.File, b []byte) (int, error) { return f.Write(b) }}
}

// Dir is <forever>/Interface/AddOns/GreenFangToolkit_Inbox.
func (w *Writer) Dir() string { return filepath.Join(w.Forever, "Interface", "AddOns", Folder) }

// Sync makes the folder hold exactly the fixed .toc and, when s is not
// "", Inbox.lua for s. Files already right are left alone. It returns
// which files it wrote. It never removes or touches anything else; an
// existing Inbox.lua stays when s is "" (no verified list yet).
func (w *Writer) Sync(s string) (wrote []string, err error) {
	if s != "" {
		if err := CheckChars(s); err != nil {
			return nil, err
		}
	}
	// Every folder from _forever_ down must be a real folder, so that no
	// link or junction can send the write anywhere else (e.g. into WTF).
	for _, p := range []string{
		w.Forever,
		filepath.Join(w.Forever, "Interface"),
		filepath.Join(w.Forever, "Interface", "AddOns"),
	} {
		if err := plainDir(p); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, ErrNoAddOns
			}
			return nil, err
		}
	}
	dir := w.Dir()
	if err := plainDir(dir); errors.Is(err, fs.ErrNotExist) {
		if err := os.Mkdir(dir, 0o755); err != nil {
			return nil, err
		}
		if err := plainDir(dir); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	files := []struct {
		name string
		data []byte
	}{{TOCName, []byte(TOC)}}
	if s != "" {
		files = append(files, struct {
			name string
			data []byte
		}{LuaName, LuaFile(s)})
	}
	for _, f := range files {
		changed, err := w.ensure(filepath.Join(dir, f.name), f.data)
		if err != nil {
			return wrote, fmt.Errorf("%s: %w", f.name, err)
		}
		if changed {
			wrote = append(wrote, f.name)
		}
	}
	return wrote, nil
}

// ensure makes path hold data: temp file, read back, rename, read back.
func (w *Writer) ensure(path string, data []byte) (bool, error) {
	if fi, err := os.Lstat(path); err == nil {
		if !fi.Mode().IsRegular() {
			return false, ErrNotPlain
		}
		if fi.Size() == int64(len(data)) {
			if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, data) {
				return false, nil
			}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	tmp := path + tmpExt
	if fi, err := os.Lstat(tmp); err == nil {
		if !fi.Mode().IsRegular() {
			return false, ErrNotPlain
		}
		if err := os.Remove(tmp); err != nil {
			return false, err
		}
	}
	if err := w.writeTemp(tmp, data); err != nil {
		os.Remove(tmp)
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return false, err
	}
	if back, err := os.ReadFile(path); err != nil || !bytes.Equal(back, data) {
		return true, ErrReadBack
	}
	return true, nil
}

func (w *Writer) writeTemp(tmp string, data []byte) error {
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	n, err := w.write(f, data)
	if err == nil && n != len(data) {
		err = errors.New("inbox: short write")
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	back, err := os.ReadFile(tmp)
	if err != nil {
		return err
	}
	if !bytes.Equal(back, data) {
		return ErrReadBack
	}
	return nil
}

// plainDir: path exists and is a real directory (not a symlink or
// junction that could point into WTF or elsewhere).
func plainDir(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 || !fi.IsDir() {
		return ErrNotPlain
	}
	return nil
}
