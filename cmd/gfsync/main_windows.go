// Command gfsync is 綠皮同步（GF Sync）: a tray app that uploads the
// GreenFangToolkit SavedVariables outbox to the guild website.
package main

import (
	"context"
	"flag"
	"fmt"
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

const maxLogBytes = 1 << 20

func dataDir() string {
	base, err := os.UserConfigDir() // %APPDATA%
	if err != nil || base == "" {
		base = os.Getenv("APPDATA")
	}
	return filepath.Join(base, "GFSync")
}

func openLog(dir string) *log.Logger {
	os.MkdirAll(dir, 0o700)
	p := filepath.Join(dir, "gfsync.log")
	if st, err := os.Stat(p); err == nil && st.Size() > maxLogBytes {
		os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return log.New(os.Stderr, "", log.LstdFlags)
	}
	return log.New(f, "", log.LstdFlags)
}

func main() {
	apiBase := flag.String("api", "", "API base URL (testing only; default "+api.DefaultBase+")")
	flag.Parse()

	mutex, first := winui.SingleInstance(`Local\GFSync-SingleInstance`)
	if !first {
		winui.Info(appName, "GF Sync 已經在執行了，請看螢幕右下角工作列的綠色圖示。")
		return
	}
	defer func() { _ = mutex }()

	dir := dataDir()
	lg := openLog(dir)
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

	eng := engine.New(engine.Config{
		DataDir:     dir,
		Client:      client,
		Tokens:      secret.NewDPAPI(dir),
		OpenBrowser: winui.OpenURL,
		Logf:        lg.Printf,
		AutoPair:    true,
	})
	a := &app{eng: eng, lg: lg, dir: dir, site: strings.TrimRight(base, "/") + "/pages/forever"}
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
	mPrivacy, mUninstall, mQuit                    *systray.MenuItem

	once        sync.Once
	askedFolder bool
	shownCode   string
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
	a.mProblem = systray.AddMenuItem("", "")
	a.mProblem.Disable()
	a.mProblem.Hide()
	systray.AddSeparator()
	a.mSyncNow = systray.AddMenuItem("立即同步", "重新讀取存檔並上傳")
	a.mPause = systray.AddMenuItemCheckbox("暫停同步", "暫停後不會上傳，直到取消暫停", false)
	a.mPair = systray.AddMenuItem("配對這台電腦", "用 Discord 登入公會網站並允許這台電腦")
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
		winui.Warn(appName, err.Error()+"\n\n請選擇 World of Warcraft 資料夾，或其中的 _forever_ 資料夾。")
	}
}

func (a *app) render(s engine.Status) {
	now := time.Now()
	a.mState.SetTitle(stateText(s))
	a.mSync.SetTitle(lastSyncText(s, now))
	if p := problemText(s); p != "" {
		a.mProblem.SetTitle("⚠ " + truncate(p, 60))
		a.mProblem.Show()
	} else {
		a.mProblem.Hide()
	}
	if s.Paused {
		a.mPause.Check()
	} else {
		a.mPause.Uncheck()
	}
	if s.Paired {
		a.mPair.SetTitle("重新配對")
	} else {
		a.mPair.SetTitle("配對這台電腦")
	}
	switch {
	case s.Paused:
		systray.SetIcon(icon.Paused)
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
	if s.PairingCode != "" && s.PairingCode != a.shownCode {
		a.shownCode = s.PairingCode
		code := formatCode(s.PairingCode)
		go winui.Info(appName, fmt.Sprintf(
			"已經幫你開啟瀏覽器。\n\n請用 Discord 登入公會網站，確認畫面上的配對碼是\n\n%s\n\n再按「允許這台電腦同步」。完成後這個視窗可以關掉。", code))
	}
}
