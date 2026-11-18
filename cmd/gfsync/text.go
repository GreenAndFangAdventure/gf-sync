package main

import (
	"fmt"
	"path/filepath"
	"time"

	"gfsync/internal/engine"
)

// Display texts (繁體中文). Kept free of Win32 calls so they can be tested.

const appName = "綠皮同步（GF Sync）"

// privacyURL is the public privacy policy (PRIVACY.md in the gf-sync repo).
const privacyURL = "https://github.com/GreenAndFangAdventure/gf-sync/blob/main/PRIVACY.md"

// Notifications. Kept short: Windows cuts long notification text.
const (
	pairingTitle = "請在瀏覽器按「允許」"
	pairedTitle  = "GF Sync 已連結"
	pairedText   = "之後下線就會自動同步，不用再做任何事。"
)

func pairingText(code string) string {
	return "用 Discord 登入公會網站，確認配對碼 " + code + "，再按「允許這台電腦同步」。"
}

func browserFailedText(url, code string) string {
	return "無法自動開啟瀏覽器。\n\n請自己開啟下面的網址，用 Discord 登入後確認配對碼是 " + code +
		"，再按「允許這台電腦同步」：\n\n" + url
}

const uninstallPrompt = "要從這台電腦移除 GF Sync 嗎？\n\n" +
	"這台電腦的配對資料（token）、上傳紀錄和記錄檔會一起刪除。魔獸世界的資料夾不會被修改。\n" +
	"已經上傳到公會網站的資料不會因此刪除；如果不再使用，也可以到公會網站的「同步裝置」頁面撤銷這台電腦。"

func formatCode(c string) string {
	if len(c) == 8 {
		return c[:4] + "-" + c[4:]
	}
	return c
}

func pairText(s engine.Status) string {
	switch {
	case s.PairingCode != "":
		return "配對中，配對碼 " + formatCode(s.PairingCode)
	case s.Paired:
		return "已配對"
	default:
		return "尚未配對"
	}
}

func lastSyncText(s engine.Status, now time.Time) string {
	if s.LastSyncAt.IsZero() {
		return "上次同步：尚未同步"
	}
	t := s.LastSyncAt.Local()
	if y, m, d := t.Date(); func() bool { ny, nm, nd := now.Local().Date(); return y == ny && m == nm && d == nd }() {
		return "上次同步：今天 " + t.Format("15:04")
	}
	return "上次同步：" + t.Format("2006-01-02 15:04")
}

// problemText is the most important thing the player should know, or "".
func problemText(s engine.Status) string {
	switch {
	case s.NeedGameDir:
		return "找不到魔獸世界：永恆的資料夾，請選擇遊戲資料夾"
	case s.LastError != "":
		return s.LastError
	case len(s.Notices) > 0:
		return s.Notices[0]
	case s.GameDir != "" && s.Files == 0:
		return "還沒有找到插件存檔，請先在遊戲中載入綠皮冒險工具箱並登出一次"
	}
	return ""
}

func stateText(s engine.Status) string {
	switch {
	case s.Paused:
		return "狀態：已暫停（" + pairText(s) + "）"
	case s.Pending > 0 && s.Paired:
		return fmt.Sprintf("狀態：%s，%d 筆等待上傳", pairText(s), s.Pending)
	default:
		return "狀態：" + pairText(s)
	}
}

func tooltipText(s engine.Status, now time.Time) string {
	t := "綠皮同步：" + pairText(s)
	if s.Paused {
		t = "綠皮同步：已暫停"
	}
	if p := problemText(s); p != "" {
		t += "\n" + p
	} else {
		t += "\n" + lastSyncText(s, now)
	}
	r := []rune(t)
	if len(r) > 120 { // Windows tooltip limit is 128 UTF-16 units
		r = append(r[:119], '…')
	}
	return string(r)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// findUninstaller returns the newest Inno Setup uninstaller (unins000.exe,
// unins001.exe, …) in dir, or "".
func findUninstaller(dir string) string {
	m, _ := filepath.Glob(filepath.Join(dir, "unins[0-9][0-9][0-9].exe"))
	if len(m) == 0 {
		return ""
	}
	return m[len(m)-1]
}
