// Command gfsync is 綠皮同步（GF Sync）: a tray app that uploads the
// GreenFangToolkit SavedVariables outbox to the guild website.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"

	"gfsync/internal/api"
	"gfsync/internal/engine"
	"gfsync/internal/icon"
	"gfsync/internal/secret"
	"gfsync/internal/winui"
)

// version is set at build time: -ldflags "-X main.version=0.1.0".
var version = "0.0.0-dev"

func dataDir() string {
	base, err := os.UserConfigDir() // %APPDATA%
	if err != nil || base == "" {
		base = os.Getenv("APPDATA")
	}
	return filepath.Join(base, "GFSync")
}

func main() {
	apiBase := flag.String("api", "", "API base URL (testing only; default "+api.DefaultBase+")")
	flag.Parse()

	mutex, first := winui.SingleInstance(`Local\GFSync-SingleInstance`)
	if !first {
		winui.Info(appName, "GF Sync 已經在執行了，請看螢幕右下角工作列的公會盾牌圖示。")
		return
	}
	defer func() { _ = mutex }()

	dir := dataDir()
	lf := newLogFile(dir)
	lg := log.New(lf, "", log.LstdFlags)
	lg.Printf("GF Sync %s starting", version)

	base := api.DefaultBase
	if v := os.Getenv("GFSYNC_API_BASE"); v != "" {
		base = v
	}
	if *apiBase != "" {
		base = *apiBase
	}
	client, err := api.New(base, version)
	if err != nil {
		winui.Warn(appName, "網站設定錯誤："+err.Error())
		return
	}
	if host, err := os.Hostname(); err == nil {
		client.Label = host
	}
	if base != api.DefaultBase {
		lg.Printf("using API base %s", base)
	}

	a := &app{lg: lg, dir: dir, site: strings.TrimRight(base, "/") + "/pages/forever"}
	a.eng = engine.New(engine.Config{
		DataDir:     dir,
		Client:      client,
		Tokens:      secret.NewDPAPI(dir),
		OpenBrowser: a.openPairingPage,
		Logf:        lg.Printf,
		LogErr:      lf.Err,
		AutoPair:    true, // first start after install opens the pairing page by itself
	})
	systray.Run(a.onReady, a.onExit)
}

type app struct {
	eng  *engine.Engine
	lg   *log.Logger
	dir  string
	site string

	cancel context.CancelFunc
	done   chan struct{}

	mState, mSync, mProblem                        *systray.MenuItem
	mSyncNow, mPause, mPair, mFolder, mSite, mLogs *systray.MenuItem
	mCopy                                          *systray.MenuItem
	mPrivacy, mUninstall, mQuit                    *systray.MenuItem

	once        sync.Once
	askedFolder bool
	shownCode   string
	pairDialog  *winui.Dialog // the open "browser opened, code is …" dialog
	rendered    bool          // render has run at least once
	prevPaired  bool          // Paired at the previous render
	mu          sync.Mutex
}

func (a *app) onReady() {
	systray.SetIcon(icon.Normal)
	systray.SetTitle(appName)
	systray.SetTooltip("綠皮同步")

	title := systray.AddMenuItem(appName+" "+version, "")
	title.Disable()
	a.mState = systray.AddMenuItem("狀態：啟動中…", "")
	a.mState.Disable()
	a.mSync = systray.AddMenuItem("上次同步：尚未同步", "")
	a.mSync.Disable()
	// Problem line: disabled text, or clickable when there is something to do
	// (pair again after the code expired, pick the game folder).
	a.mProblem = systray.AddMenuItem("", "")
	a.mProblem.Disable()
	a.mProblem.Hide()
	systray.AddSeparator()
	a.mSyncNow = systray.AddMenuItem("立即同步", "重新讀取存檔並上傳")
	a.mPause = systray.AddMenuItemCheckbox("暫停同步", "暫停後不會上傳，直到取消暫停", false)
	a.mPair = systray.AddMenuItem("配對這台電腦", "用 Discord 登入公會網站並允許這台電腦")
	a.mCopy = systray.AddMenuItem(copyButton, "只複製配對碼，可以貼到配對頁")
	a.mCopy.Hide()
	a.mFolder = systray.AddMenuItem("選擇遊戲資料夾…", "選擇 World of Warcraft 或 _forever_ 資料夾")
	a.mSite = systray.AddMenuItem("開啟公會網站", "")
	a.mLogs = systray.AddMenuItem("開啟記錄檔資料夾", "")
	a.mPrivacy = systray.AddMenuItem("隱私權政策", "GF Sync 會上傳哪些資料")
	systray.AddSeparator()
	a.mUninstall = systray.AddMenuItem("解除安裝 GF Sync…", "從這台電腦移除 GF Sync")
	a.mQuit = systray.AddMenuItem("結束 GF Sync", "")

	a.eng.OnChange(a.render)
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.done = make(chan struct{})
	go func() { a.eng.Run(ctx); close(a.done) }()
	go a.loop()
	go func() {
		for range time.Tick(30 * time.Second) {
			a.render(a.eng.Status()) // refresh "today HH:MM" texts
		}
	}()
}

func (a *app) onExit() {
	if a.cancel != nil {
		a.cancel()
		select {
		case <-a.done:
		case <-time.After(5 * time.Second):
		}
	}
	a.lg.Printf("GF Sync stopped")
}

func (a *app) loop() {
	for {
		select {
		case <-a.mProblem.ClickedCh:
			switch problemAction(a.eng.Status()) {
			case actionPair:
				a.eng.StartPairing()
			case actionFolder:
				go a.pickFolder()
			case actionHelp:
				_ = winui.OpenURL(avHelpURL)
			}
		case <-a.mCopy.ClickedCh:
			if f := a.copyCode(); f != "" {
				go a.notify("已複製配對碼", f)
			}
		case <-a.mSyncNow.ClickedCh:
			a.eng.SyncNow()
		case <-a.mPause.ClickedCh:
			a.eng.SetPaused(!a.eng.Status().Paused)
		case <-a.mPair.ClickedCh:
			s := a.eng.Status()
			if s.Paired && !winui.YesNo(appName, "這台電腦已經配對過了。要重新配對嗎？") {
				continue
			}
			a.eng.StartPairing()
		case <-a.mFolder.ClickedCh:
			go a.pickFolder()
		case <-a.mSite.ClickedCh:
			_ = winui.OpenURL(a.site)
		case <-a.mLogs.ClickedCh:
			_ = winui.OpenFolder(a.dir)
		case <-a.mPrivacy.ClickedCh:
			_ = winui.OpenURL(privacyURL)
		case <-a.mUninstall.ClickedCh:
			if a.uninstall() {
				systray.Quit()
				return
			}
		case <-a.mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

// uninstall starts the Inno Setup uninstaller installed next to GFSync.exe.
// It returns true when the uninstaller was started and GF Sync should exit.
func (a *app) uninstall() bool {
	exe, err := os.Executable()
	if err != nil {
		winui.Warn(appName, "找不到解除安裝程式："+err.Error())
		return false
	}
	un := findUninstaller(filepath.Dir(exe))
	if un == "" {
		winui.Warn(appName, "找不到解除安裝程式。\n\n請到 Windows「設定 → 應用程式 → 已安裝的應用程式」移除 GF Sync。")
		return false
	}
	if !winui.YesNo(appName, uninstallPrompt) {
		return false
	}
	if err := exec.Command(un).Start(); err != nil {
		winui.Warn(appName, "無法啟動解除安裝程式："+err.Error())
		return false
	}
	a.lg.Printf("uninstaller started")
	return true
}

func (a *app) pickFolder() {
	p, ok := winui.PickFolder("請選擇魔獸世界的安裝資料夾（World of Warcraft 或其中的 _forever_）")
	if !ok {
		return
	}
	if err := a.eng.SetGameDir(p); err != nil {
		if err.Error() == engine.MsgFolderDenied {
			winui.Warn(appName, err.Error()+"\n\n請看說明「"+avHelpTitle+"」（點工作列選單 ⚠ 那一行）。")
			return
		}
		winui.Warn(appName, err.Error()+"\n\n請選擇 World of Warcraft 資料夾，或其中的 _forever_ 資料夾。")
	}
}

func (a *app) render(s engine.Status) {
	now := time.Now()
	a.mState.SetTitle(stateText(s))
	a.mSync.SetTitle(lastSyncText(s, now))
	if p := problemText(s); p != "" {
		a.mProblem.SetTitle("⚠ " + truncate(p, 60))
		if problemAction(s) != actionNone {
			a.mProblem.Enable()
		} else {
			a.mProblem.Disable()
		}
		a.mProblem.Show()
	} else {
		a.mProblem.Hide()
	}
	if s.Paused {
		a.mPause.Check()
	} else {
		a.mPause.Uncheck()
	}
	a.mPair.SetTitle(pairMenuText(s))
	if clipboardCode(s) != "" {
		a.mCopy.SetTitle(copyMenuText(s))
		a.mCopy.Show()
	} else {
		a.mCopy.Hide()
	}
	switch {
	case s.Paused:
		systray.SetIcon(icon.Paused)
	case blocked(s):
		systray.SetIcon(icon.Error)
	case s.PairingCode != "" || !s.Paired:
		systray.SetIcon(icon.Pairing)
	case problemText(s) != "":
		systray.SetIcon(icon.Error)
	default:
		systray.SetIcon(icon.Normal)
	}
	systray.SetTooltip(tooltipText(s, now))

	a.mu.Lock()
	defer a.mu.Unlock()
	if s.NeedGameDir && !a.askedFolder {
		a.askedFolder = true
		go func() {
			winui.Info(appName, "找不到魔獸世界：永恆（_forever_）的資料夾。\n\n按「確定」後請選擇遊戲安裝資料夾。")
			a.pickFolder()
		}()
	}
	if a.pairDialog != nil && s.PairingCode == "" {
		// Paired, expired, denied or cancelled: the code is no longer valid.
		a.pairDialog.Close()
		a.pairDialog = nil
	}
	if s.Ready {
		if a.rendered && !a.prevPaired && s.Paired {
			// The one and only "it worked" message; after this GF Sync stays quiet.
			go a.notify(pairedTitle, pairedText)
		}
		a.rendered, a.prevPaired = true, s.Paired
	}
}

// openPairingPage is the engine's OpenBrowser, used both for the automatic
// pairing on first start and for 「配對這台電腦」 in the menu. It opens the
// browser and, once per code, shows a dialog with the code and a
// 「重新開啟配對頁」 button. The dialog closes itself when pairing ends.
func (a *app) openPairingPage(url string) error {
	err := winui.OpenURL(url)
	code := formatCode(a.eng.Status().PairingCode)
	a.mu.Lock()
	first := code != a.shownCode
	a.shownCode = code
	var d *winui.Dialog
	if first {
		if a.pairDialog != nil {
			a.pairDialog.Close()
		}
		d = &winui.Dialog{
			Title:       appName,
			Instruction: pairDialogInstruction(err == nil),
			Content:     pairDialogText(code, url, err == nil),
			Footer:      pairFooter,
			Buttons: []winui.DialogButton{
				{Label: reopenButton, OnClick: func() string {
					a.lg.Printf("pair: reopen pairing page")
					if u := a.eng.Status().PairingURL; u != "" {
						go func() {
							if err := winui.OpenURL(u); err != nil {
								a.lg.Printf("pair: reopen: %v", err)
							}
						}()
					}
					return ""
				}},
				{Label: copyButton, OnClick: a.copyCode},
			},
		}
		a.pairDialog = d
	}
	a.mu.Unlock()
	if d != nil {
		go d.Show()
	}
	return err
}

// copyCode puts only the pairing code on the clipboard and returns the
// 「已複製」 text to show, or "" when there is no code.
func (a *app) copyCode() string {
	code := clipboardCode(a.eng.Status())
	if code == "" {
		return ""
	}
	if err := winui.CopyText(code); err != nil {
		a.lg.Printf("pair: copy code: %v", err)
		return "無法複製，請手動輸入配對碼。"
	}
	a.lg.Printf("pair: code copied to clipboard")
	return copiedText(code)
}

// notify shows a Windows notification, falling back to a dialog when the
// notification cannot be shown.
func (a *app) notify(title, text string) {
	if err := winui.Notify(title, text); err != nil {
		a.lg.Printf("notify: %v", err)
		winui.Info(appName, title+"\n\n"+text)
	}
}
