package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gfsync/internal/engine"
	"gfsync/internal/update"
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

func TestPairingTexts(t *testing.T) {
	// Windows limits notification titles to 63 and text to 255 characters.
	if len([]rune(pairedTitle)) > 63 || len([]rune(pairedText)) > 255 {
		t.Fatal("paired notification too long")
	}
	ok := pairDialogText("ABCD-EFGH", "https://x/pages/sync-pair?code=ABCDEFGH", true)
	if !strings.Contains(ok, "ABCD-EFGH") || !strings.Contains(ok, reopenButton) || !strings.Contains(ok, "^") {
		t.Fatalf("dialog text: %s", ok)
	}
	if strings.Contains(ok, "https://") {
		t.Fatal("no address needed when the browser opened")
	}
	failed := pairDialogText("ABCD-EFGH", "https://x/pages/sync-pair?code=ABCDEFGH", false)
	if !strings.Contains(failed, "https://x/pages/sync-pair?code=ABCDEFGH") {
		t.Fatal("fallback must show the address")
	}
}

func TestExpiredPairing(t *testing.T) {
	s := engine.Status{PairEnded: true, PairExpired: true, LastError: engine.MsgPairExpired}
	if pairMenuText(s) != "配對碼已過期，點這裡重新配對" {
		t.Fatalf("menu: %q", pairMenuText(s))
	}
	if problemText(s) != engine.MsgPairExpired || problemAction(s) != actionPair {
		t.Fatalf("problem: %q %v", problemText(s), problemAction(s))
	}
	if tt := tooltipText(s, time.Now()); !strings.Contains(tt, "已過期") || strings.Contains(tt, "配對中") {
		t.Fatalf("tooltip: %q", tt)
	}
	if problemAction(engine.Status{Pairing: true, PairingCode: "ABCDEFGH"}) != actionNone {
		t.Fatal("nothing to click while waiting for approval")
	}
	if problemAction(engine.Status{NeedGameDir: true}) != actionFolder {
		t.Fatal("folder")
	}
}

func TestClipboardCodeIsOnlyTheCode(t *testing.T) {
	for in, want := range map[string]string{
		"ABCDEFGH":        "ABCDEFGH",
		"abcd-efgh":       "ABCDEFGH",
		" AB CD-EF\tGH\n": "ABCDEFGH",
		"":                "",
		"AB":              "",
	} {
		if got := clipboardCode(engine.Status{PairingCode: in}); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	s := engine.Status{PairingCode: "ABCDEFGH", PairingURL: "https://x/pages/sync-pair?code=ABCDEFGH", Paired: false}
	got := clipboardCode(s)
	if got != "ABCDEFGH" || strings.ContainsAny(got, " -\t\n") {
		t.Fatalf("clipboard %q", got)
	}
	if copyMenuText(s) != "複製配對碼（ABCD-EFGH）" {
		t.Fatalf("menu %q", copyMenuText(s))
	}
	if !strings.Contains(copiedText(got), "已複製") {
		t.Fatal("copied text")
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

func TestUpdateTexts(t *testing.T) {
	u := &engine.Update{Version: "0.2.0", Notes: "修正同步"}
	if got := updateMenuText(u); !strings.Contains(got, "0.2.0") || !strings.Contains(got, "下載") {
		t.Errorf("%q", got)
	}
	title, text := updateNotice(u)
	if !strings.Contains(title, "0.2.0") || !strings.Contains(text, "修正同步") || strings.Contains(text, "隱私") {
		t.Errorf("%q %q", title, text)
	}
	u.Required, u.PrivacyChanged = true, true
	if _, text := updateNotice(u); !strings.Contains(text, "很快就不能上傳") || !strings.Contains(text, "隱私權政策") {
		t.Errorf("%q", text)
	}
	for _, c := range []struct {
		u    *engine.Update
		err  error
		want string
	}{
		{nil, nil, "最新版本"},
		{nil, update.ErrNotFound, "最新版本"},
		{nil, engine.ErrUpdatesOff, "測試版"},
		{nil, errors.New("x"), "無法檢查"},
		{u, nil, "很快就不能上傳"},
	} {
		if _, text := checkResultText(c.u, c.err); !strings.Contains(text, c.want) {
			t.Errorf("%v %v: %q", c.u, c.err, text)
		}
	}
	s := engine.Status{ClientTooOld: true, MinVersion: "0.2.0", LastError: engine.MsgClientTooOld}
	if problemAction(s) != actionDownload || !strings.Contains(tooOldText(s), "0.2.0") {
		t.Errorf("426: action %v text %q", problemAction(s), tooOldText(s))
	}
	// Pairing refused with 426 also offers the download, not pairing again.
	s.Paired = false
	if problemAction(s) != actionDownload {
		t.Error("426 before pairing must offer the download")
	}
}
