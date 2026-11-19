package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gfsync/internal/engine"
)

func TestBlockedIsNotNotFound(t *testing.T) {
	denied := engine.Status{Ready: true, Paired: true, GameDirDenied: true, GameDir: `D:\x\_forever_`}
	p := problemText(denied)
	if strings.Contains(p, "找不到") || strings.Contains(p, "請選擇") || !strings.Contains(p, "防毒") {
		t.Fatalf("denied text: %q", p)
	}
	if problemAction(denied) != actionHelp {
		t.Fatal("denied: click opens the help, not the folder picker")
	}
	if !blocked(denied) {
		t.Fatal("blocked icon")
	}
	notFound := engine.Status{Ready: true, Paired: true, NeedGameDir: true}
	if !strings.Contains(problemText(notFound), "找不到") || problemAction(notFound) != actionFolder {
		t.Fatal("really not found still asks for the folder")
	}
	if tt := tooltipText(denied, time.Now()); !strings.Contains(tt, "沒有權限") {
		t.Fatalf("tooltip: %q", tt)
	}
}

func TestStorageProblemShown(t *testing.T) {
	s := engine.Status{Ready: true, TokenUnreadable: true, Storage: []string{engine.StorageLog, engine.StorageToken}}
	p := problemText(s)
	for _, want := range []string{"記錄檔", "配對資料", "防毒"} {
		if !strings.Contains(p, want) {
			t.Fatalf("%q missing %q", p, want)
		}
	}
	if pairText(s) == "尚未配對" {
		t.Fatal("an unreadable token is not 'not paired'")
	}
	if problemAction(s) != actionHelp {
		t.Fatal("click opens help, not pairing")
	}
	if !strings.Contains(avHelpURL, "#%E5%A6%82%E6%9E%9C") {
		t.Fatalf("help anchor: %s", avHelpURL)
	}
	for _, k := range []string{engine.StorageLog, engine.StorageState, engine.StorageToken} {
		if storageNames[k] == "" {
			t.Fatalf("no name for %s", k)
		}
	}
}

func TestLogFileProblemIsReported(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gfsync.log")
	// gfsync.log cannot be opened (a directory stands in for a blocked file).
	if err := os.Mkdir(p, 0o700); err != nil {
		t.Fatal(err)
	}
	l := newLogFile(dir)
	if l.Err() == nil {
		t.Fatal("open failure must be reported")
	}
	if n, err := l.Write([]byte("lost\n")); n != 5 || err != nil {
		t.Fatalf("write while blocked: %d %v", n, err)
	}
	if l.Err() == nil {
		t.Fatal("still blocked")
	}
	// Unblocked: the next write opens the file.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	if l.Err() != nil {
		t.Fatalf("recovered: %v", l.Err())
	}
	l.mu.Lock()
	l.f.Close()
	l.mu.Unlock()
	b, _ := os.ReadFile(p)
	if string(b) != "hello\n" {
		t.Fatalf("log: %q", b)
	}
}
