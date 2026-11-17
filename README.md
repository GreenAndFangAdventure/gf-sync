# GF Sync（綠皮同步）

「綠皮與獠牙冒險事務所」公會的 Windows 工作列小程式。它會把遊戲插件「綠皮冒險工具箱」存下來的角色與公會名冊資料，自動上傳到公會網站 [greenandfangadventure.com](https://greenandfangadventure.com)，讓網站的名冊不用手動更新。

- 只支援 Windows 10／11（64 位元），以及《魔獸世界：永恆》（`_forever_`）。
- **只讀取**插件的存檔，不會修改任何遊戲檔案，也不會安裝或更新插件。
- 以 MIT 授權開源（[`LICENSE`](LICENSE)）。

## 安裝

1. 從 [GitHub Releases](https://github.com/GreenAndFangAdventure/gf-sync/releases) 下載 `GFSync-Setup-<版本>.exe`。
2. （建議）核對 SHA-256：在 PowerShell 執行 `Get-FileHash .\GFSync-Setup-<版本>.exe`，和 Release 頁的 `SHA256SUMS.txt` 比對。
3. 執行安裝檔。安裝過程會先顯示[隱私權政策](PRIVACY.md)。不需要系統管理員權限，程式會裝在 `%LOCALAPPDATA%\Programs\GFSync`。
4. 「開機時自動啟動 GF Sync」預設勾選，做法是在你的「啟動」資料夾放一個捷徑（可以在「工作管理員 → 啟動應用程式」關掉）。

目前的版本還**沒有數位簽章**（申請中，見 [Code signing policy](#code-signing-policy)），所以 Windows SmartScreen 可能顯示「Windows 已保護您的電腦」，請按「其他資訊」→「仍要執行」。部分防毒軟體也可能誤判，請先核對 SHA-256。

## 配對（只需要做一次）

1. 第一次啟動時，GF Sync 會自動開啟瀏覽器，並顯示一組 8 碼的配對碼。
2. 用 Discord 登入公會網站，確認畫面上的配對碼一樣，再按「允許這台電腦同步」。
3. 完成。之後每次你在遊戲中登出或 `/reload`，GF Sync 就會在幾秒內上傳新資料。

工作列綠色圖示的右鍵選單可以看到狀態、上次同步時間，也可以「立即同步」「重新配對」「選擇遊戲資料夾…」。找不到遊戲時，GF Sync 會請你選擇 `World of Warcraft` 或其中的 `_forever_` 資料夾。

## 會上傳哪些資料

角色名稱、伺服器、陣營、公會與階級、等級、職業、種族、專業，以及公會名冊快照（成員名稱、等級、職業、種族、階級、專業）。**不會上傳所在地和在線狀態。** 完整欄位與保存方式見 [`PRIVACY.md`](PRIVACY.md)，技術格式見 [`docs/FORMAT.md`](docs/FORMAT.md)。

## 暫停或關閉

- 遊戲內輸入 `/gf sync off`：插件標記不同步，GF Sync 不會上傳這個帳號的資料。
- 工作列選單勾選「暫停同步」：重新開機後仍然有效，取消勾選才會恢復。
- 到 <https://greenandfangadventure.com/pages/sync-devices> 撤銷這台電腦。

## 解除安裝

下列任一種方式都可以：

- 工作列選單的「解除安裝 GF Sync…」
- 「開始」選單的「解除安裝 綠皮同步（GF Sync）」
- Windows「設定 → 應用程式 → 已安裝的應用程式」

解除安裝會刪除程式和 `%APPDATA%\GFSync`（配對 token、上傳紀錄、記錄檔），不會動到遊戲資料夾。

## 從原始碼建置

需要 [Go](https://go.dev/)（版本見 `go.mod`）和 [Inno Setup 6](https://jrsoftware.org/isinfo.php)。

```powershell
powershell -ExecutionPolicy Bypass -File build.ps1 -Version 0.1.1   # 測試 + exe + 安裝檔 + SHA256SUMS.txt，輸出到 dist\
go test ./...                                                       # 只跑測試
```

正式發布的檔案由 GitHub Actions（[`.github/workflows/build.yml`](.github/workflows/build.yml)）在推送 `v*` tag 時建置並上傳到 Releases。發布流程見 [`RELEASE.md`](RELEASE.md)，實機測試步驟見 [`MANUAL.md`](MANUAL.md)。

| 路徑 | 內容 |
| --- | --- |
| `internal/luasv` | Lua 表格字面值解析器，絕不執行程式碼；有深度／大小／節點數上限與 fuzz 測試 |
| `internal/svdoc` | 解讀 `GreenFangToolkitDB`：只接受 `schemaVersion` 2、遵守 `sync.enabled`、忽略未知 kind、上傳前移除 `zone`／`online` |
| `internal/gamedir` | 找 `_forever_`（登錄檔、各磁碟常見路徑），列出 SavedVariables 檔案 |
| `internal/watch` | 檔案停止變動 5 秒後才讀 |
| `internal/api` | 網站 API 用戶端；只接受 https（loopback 例外，供測試） |
| `internal/engine` | 監看、配對、上傳佇列、退避重試、每小時上限；`e2e_test.go` 是對模擬伺服器的端到端測試 |
| `internal/state` | `%APPDATA%\GFSync\state.json` |
| `internal/secret` | token 用 Windows DPAPI 加密存在 `%APPDATA%\GFSync\token.dat` |
| `internal/mockapi`、`cmd/gfsync-mock` | 離線模擬網站 API |
| `internal/winui`、`internal/icon`、`cmd/gfsync` | 工作列介面（繁體中文） |
| `cmd/mkres` | 產生 exe 的版本資訊、`asInvoker` manifest、圖示，以及安裝檔圖示 |
| `installer/gfsync.iss` | Inno Setup 6 安裝檔 |

測試用參數：`GFSync.exe --api http://127.0.0.1:8787`（或環境變數 `GFSYNC_API_BASE`）改連本機模擬伺服器 `go run ./cmd/gfsync-mock`。非 loopback 位址一律要求 https。

## Code signing policy

Free code signing provided by [SignPath.io](https://about.signpath.io/), certificate by [SignPath Foundation](https://signpath.org/)（申請中）。

完整政策與團隊角色見 [`CODE_SIGNING_POLICY.md`](CODE_SIGNING_POLICY.md)，隱私權政策見 [`PRIVACY.md`](PRIVACY.md)。

## 授權

MIT，著作權人「綠皮與獠牙冒險事務所」。第三方套件授權見 [`THIRD_PARTY_LICENSES.md`](THIRD_PARTY_LICENSES.md)。

---

## English summary

**GF Sync** is a small Windows tray app for the World of Warcraft guild 綠皮與獠牙冒險事務所 ("Green and Fang Adventure"). It reads the SavedVariables file written by the guild's in-game AddOn (read-only, never executing Lua and never modifying game files) and uploads character and roster data to the guild website, greenandfangadventure.com, after a one-time device pairing via Discord login.

- **Install:** download `GFSync-Setup-<version>.exe` from GitHub Releases, verify its SHA-256, run it (no admin rights needed). The privacy policy is shown during setup. Autostart uses a shortcut in the user's Startup folder.
- **Data:** character name, realm, faction, guild and rank, level, class, race, professions, and guild roster snapshots. Location and online status are never uploaded. See [PRIVACY.md](PRIVACY.md).
- **Stop syncing:** `/gf sync off` in game, "暫停同步" (Pause) in the tray menu, or revoke the device on the website.
- **Uninstall:** tray menu "解除安裝 GF Sync…", the Start menu uninstall shortcut, or Windows Settings → Apps.
- **Build:** Go + Inno Setup 6, `build.ps1 -Version <x.y.z>`; releases are built by GitHub Actions.
- **License:** MIT. Code signing: see [CODE_SIGNING_POLICY.md](CODE_SIGNING_POLICY.md).
