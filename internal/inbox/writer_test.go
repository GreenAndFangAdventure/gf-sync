package inbox

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
)

// A list as Writer sees it: already verified, only the alphabet matters.
const list1 = "GFIN1~abc~c2ln~H;realm=x;guild=y;at=5;n=0"
const list2 = "GFIN1~abd~c2lm~H;realm=x;guild=y;at=6;n=0"

// game makes a fake _forever_ with an AddOn and a WTF folder holding a
// SavedVariables file, and returns the folder.
func game(t *testing.T) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "_forever_")
	must(t, os.MkdirAll(filepath.Join(f, "Interface", "AddOns", "GreenFangToolkit"), 0o755))
	must(t, os.MkdirAll(filepath.Join(f, "WTF", "Account", "ACCT", "SavedVariables"), 0o755))
	must(t, os.WriteFile(filepath.Join(f, "WTF", "Account", "ACCT", "SavedVariables", "GreenFangToolkit.lua"), []byte("GreenFangToolkitDB = {}\n"), 0o644))
	return f
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// tree lists every file under root with its content, to prove nothing
// outside GreenFangToolkit_Inbox changed.
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			out[rel+"/"] = ""
			return nil
		}
		b, _ := os.ReadFile(p)
		out[rel] = string(b)
		return nil
	})
	return out
}

func keys(m map[string]string) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSyncWritesOnlyTheTwoFiles(t *testing.T) {
	f := game(t)
	before := tree(t, f)
	w := NewWriter(f)
	wrote, err := w.Sync(list1)
	if err != nil || strings.Join(wrote, ",") != TOCName+","+LuaName {
		t.Fatalf("Sync = %v, %v", wrote, err)
	}
	if got := read(t, filepath.Join(w.Dir(), LuaName)); got != `GreenFangToolkit_Inbox = "`+list1+"\"\n" {
		t.Fatalf("Inbox.lua = %q", got)
	}
	if strings.Count(read(t, filepath.Join(w.Dir(), LuaName)), "\n") != 1 {
		t.Fatal("Inbox.lua must be one line")
	}
	if read(t, filepath.Join(w.Dir(), TOCName)) != TOC {
		t.Fatal("toc content")
	}
	after := tree(t, f)
	for k, v := range before {
		if after[k] != v {
			t.Errorf("changed outside the inbox folder: %s", k)
		}
	}
	added := []string{}
	for _, k := range keys(after) {
		if _, ok := before[k]; !ok {
			added = append(added, filepath.ToSlash(k))
		}
	}
	want := "Interface/AddOns/GreenFangToolkit_Inbox/,Interface/AddOns/GreenFangToolkit_Inbox/GreenFangToolkit_Inbox.toc,Interface/AddOns/GreenFangToolkit_Inbox/Inbox.lua"
	if strings.Join(added, ",") != want {
		t.Fatalf("added %v", added)
	}
	// Nothing to do the second time.
	if wrote, err := w.Sync(list1); err != nil || len(wrote) != 0 {
		t.Fatalf("second Sync = %v, %v", wrote, err)
	}
	// No list yet: only the folder and .toc, so the game sees the AddOn.
	f2 := game(t)
	if wrote, err := NewWriter(f2).Sync(""); err != nil || strings.Join(wrote, ",") != TOCName {
		t.Fatalf("Sync(\"\") = %v, %v", wrote, err)
	}
}

func TestSyncRestoresChangedFiles(t *testing.T) {
	f := game(t)
	w := NewWriter(f)
	must(t, firstErr(w.Sync(list1)))
	toc := filepath.Join(w.Dir(), TOCName)
	lua := filepath.Join(w.Dir(), LuaName)
	must(t, os.WriteFile(toc, []byte(TOC+"Evil.lua\n"), 0o644))
	must(t, os.WriteFile(lua, []byte("os.execute('x')\n"), 0o644))
	wrote, err := w.Sync(list1)
	if err != nil || len(wrote) != 2 {
		t.Fatalf("Sync = %v, %v", wrote, err)
	}
	if read(t, toc) != TOC || read(t, lua) != string(LuaFile(list1)) {
		t.Fatal("not restored")
	}
	// An extra file someone drops in the folder is not ours to delete,
	// and the fixed .toc never loads it.
	must(t, os.WriteFile(filepath.Join(w.Dir(), "Evil.lua"), []byte("x"), 0o644))
	if _, err := w.Sync(list1); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(t, toc), "Evil") {
		t.Fatal("toc lists a foreign file")
	}
}

func TestSyncRefusesBadStrings(t *testing.T) {
	f := game(t)
	w := NewWriter(f)
	must(t, firstErr(w.Sync(list1)))
	for _, bad := range []string{
		`GFIN1~a~b~H;realm=..\..\..\WTF;at=1;n=0`,
		`GFIN1~a~b~H;realm=../../WTF;at=1;n=0`,
		"GFIN1~a~b~H;at=1;n=0\"\nos.execute('x')--",
		"GFIN1~a~b~H;realm=測試",
		strings.Repeat("A", MaxLen+1),
	} {
		if _, err := w.Sync(bad); err == nil {
			t.Errorf("wrote %q", bad[:20])
		}
	}
	if read(t, filepath.Join(w.Dir(), LuaName)) != string(LuaFile(list1)) {
		t.Fatal("refused string changed Inbox.lua")
	}
}

func TestSyncNeedsAddOnsFolder(t *testing.T) {
	f := filepath.Join(t.TempDir(), "_forever_")
	must(t, os.MkdirAll(f, 0o755))
	if _, err := NewWriter(f).Sync(list1); !errors.Is(err, ErrNoAddOns) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(f, "Interface")); err == nil {
		t.Fatal("created Interface")
	}
}

// A junction (or link) in place of the inbox folder or a parent could
// point the write into WTF: refused, WTF unchanged.
func TestSyncRefusesJunctions(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are Windows only")
	}
	f := game(t)
	wtf := filepath.Join(f, "WTF", "Account", "ACCT", "SavedVariables")
	before := tree(t, filepath.Join(f, "WTF"))
	link := filepath.Join(f, "Interface", "AddOns", Folder)
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, wtf).CombinedOutput(); err != nil {
		t.Skipf("mklink /J: %v %s", err, out)
	}
	if _, err := NewWriter(f).Sync(list1); !errors.Is(err, ErrNotPlain) {
		t.Fatalf("inbox folder junction: err = %v", err)
	}
	must(t, os.Remove(link))
	// AddOns itself a junction to WTF.
	addons := filepath.Join(f, "Interface", "AddOns")
	must(t, os.RemoveAll(addons))
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", addons, wtf).CombinedOutput(); err != nil {
		t.Skipf("mklink /J: %v %s", err, out)
	}
	if _, err := NewWriter(f).Sync(list1); !errors.Is(err, ErrNotPlain) {
		t.Fatalf("AddOns junction: err = %v", err)
	}
	after := tree(t, filepath.Join(f, "WTF"))
	if len(after) != len(before) {
		t.Fatalf("WTF changed: %v", keys(after))
	}
	for k, v := range before {
		if after[k] != v {
			t.Fatalf("WTF changed: %s", k)
		}
	}
}

// Disk full half way: the old Inbox.lua stays whole and no temp file is
// left behind.
func TestSyncDiskFull(t *testing.T) {
	f := game(t)
	w := NewWriter(f)
	must(t, firstErr(w.Sync(list1)))
	w.write = func(fh *os.File, b []byte) (int, error) {
		n, _ := fh.Write(b[:len(b)/2])
		return n, syscall.Errno(112) // ERROR_DISK_FULL
	}
	if _, err := w.Sync(list2); err == nil {
		t.Fatal("disk full not reported")
	}
	if read(t, filepath.Join(w.Dir(), LuaName)) != string(LuaFile(list1)) {
		t.Fatal("old file damaged")
	}
	assertNoTemp(t, w.Dir())
	w.write = func(fh *os.File, b []byte) (int, error) { return fh.Write(b) }
	if _, err := w.Sync(list2); err != nil || read(t, filepath.Join(w.Dir(), LuaName)) != string(LuaFile(list2)) {
		t.Fatalf("after space is back: %v", err)
	}
}

// The game (or anything) holding Inbox.lua open: the file is either the
// old one or the new one, never a mix; the write is retried later.
func TestSyncWhileFileOpen(t *testing.T) {
	f := game(t)
	w := NewWriter(f)
	must(t, firstErr(w.Sync(list1)))
	lua := filepath.Join(w.Dir(), LuaName)
	h, err := os.Open(lua)
	must(t, err)
	_, err = w.Sync(list2)
	got := read(t, lua)
	h.Close()
	if got != string(LuaFile(list1)) && got != string(LuaFile(list2)) {
		t.Fatalf("half-written file: %q", got)
	}
	assertNoTemp(t, w.Dir())
	if err != nil {
		t.Logf("write while open refused as expected: %v", err)
		if _, err := w.Sync(list2); err != nil {
			t.Fatalf("retry after close: %v", err)
		}
	}
	if read(t, lua) != string(LuaFile(list2)) {
		t.Fatal("new list not written after close")
	}
}

func TestStaleTempFileIsReplaced(t *testing.T) {
	f := game(t)
	w := NewWriter(f)
	must(t, firstErr(w.Sync("")))
	must(t, os.WriteFile(filepath.Join(w.Dir(), LuaName+tmpExt), []byte("junk"), 0o644))
	if _, err := w.Sync(list1); err != nil {
		t.Fatal(err)
	}
	assertNoTemp(t, w.Dir())
}

func assertNoTemp(t *testing.T, dir string) {
	t.Helper()
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), tmpExt) {
			t.Fatalf("temp file left: %s", e.Name())
		}
	}
}

func firstErr(_ []string, err error) error { return err }
