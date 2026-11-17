package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gfsync/internal/engine"
)

func TestTexts(t *testing.T) {
	now := time.Date(2026, 11, 17, 20, 0, 0, 0, time.Local)
	s := engine.Status{Paired: true, GameDir: `D:\x\_forever_`, Files: 1, LastSyncAt: now.Add(-time.Hour)}
	if got := tooltipText(s, now); got != "綠皮同步：已配對\n上次同步：今天 19:00" {
		t.Errorf("%q", got)
	}
	s.LastSyncAt = time.Date(2026, 11, 1, 8, 5, 0, 0, time.Local)
	if got := lastSyncText(s, now); got != "上次同步：2026-11-01 08:05" {
		t.Errorf("%q", got)
	}
	if got := pairText(engine.Status{PairingCode: "ABCD2345"}); got != "配對中，配對碼 ABCD-2345" {
		t.Errorf("%q", got)
	}
	if got := stateText(engine.Status{Paired: true, Pending: 3}); got != "狀態：已配對，3 筆等待上傳" {
		t.Errorf("%q", got)
	}
	if got := problemText(engine.Status{NeedGameDir: true, LastError: "x"}); !strings.Contains(got, "選擇遊戲資料夾") {
		t.Errorf("%q", got)
	}
	if got := problemText(engine.Status{GameDir: "x"}); !strings.Contains(got, "登出一次") {
		t.Errorf("%q", got)
	}
	long := engine.Status{LastError: strings.Repeat("錯", 300)}
	if n := len([]rune(tooltipText(long, now))); n > 120 {
		t.Errorf("tooltip too long: %d", n)
	}
}

func TestFindUninstaller(t *testing.T) {
	dir := t.TempDir()
	if findUninstaller(dir) != "" {
		t.Fatal("empty dir")
	}
	for _, n := range []string{"unins000.exe", "unins001.exe", "unins000.dat", "uninstall.exe"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := findUninstaller(dir); filepath.Base(got) != "unins001.exe" {
		t.Fatalf("got %q", got)
	}
}
