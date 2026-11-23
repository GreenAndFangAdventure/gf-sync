# GF Sync 更新清單與版本握手（GF Sync 這邊的合約）

> 狀態：GF-001-J 第 1–4 步已實作（只檢查、只通知）。自動下載和安裝要等 SignPath 簽章（D-017 第 5 點），見 [AUTO_UPDATE.md](AUTO_UPDATE.md)。
> 這份是 GF Sync 和網站之間的約定；工具箱的 `addon-manifest.json` 由插件那邊的合約（`docs/contracts/addon-manifest.md`）定義，欄位盡量一致。

## 1. `gfsync-manifest.json`

由 GitHub Actions（`.github/workflows/build.yml` 的「Update manifest」步驟）產生，是 GitHub Release 的一個檔案，和安裝檔、`SHA256SUMS.txt` 放在一起。**UTF-8、沒有 BOM、一行 JSON 加換行。**

```json
{"schema":1,"product":"GFSync","version":"0.2.0","channel":"release","released":"2026-11-20T10:17:05Z","url":"https://github.com/GreenAndFangAdventure/gf-sync/releases/download/v0.2.0/GFSync-Setup-0.2.0.exe","sha256":"<64 位十六進位，小寫>","size":7340032,"minGFSync":"","changelog":"https://github.com/GreenAndFangAdventure/gf-sync/releases/tag/v0.2.0","privacyChanged":false,"notes":""}
```

| 欄位 | 說明 | GF Sync 的檢查 |
| --- | --- | --- |
| `schema` | 固定 `1` | 不是 1 就不接受 |
| `product` | 固定 `"GFSync"` | 不一致就不接受 |
| `version` | 版本，和 tag（去掉 `v`）一致 | `x.y.z` 或 `x.y.z-標籤`；`0.0.0-*`（CI 測試版）不接受 |
| `channel` | 固定 `"release"` | 其他值不接受 |
| `released` | 建置時間（UTC） | 只記錄 |
| `url` | 安裝檔 | **一定要**是 `https://github.com/GreenAndFangAdventure/gf-sync/releases/download/v<version>/GFSync-Setup-<version>.exe` |
| `sha256` | 安裝檔（簽章後）的 SHA-256 | 必須和同一個 Release 的 `SHA256SUMS.txt` 裡的安裝檔那一行一致 |
| `size` | 安裝檔大小（位元組） | 只記錄（之後下載時用） |
| `minGFSync` | 比這個版本舊的 GF Sync，網站可能會拒收（§3）；`""` = 沒有 | 格式要對；GF Sync 比它舊時，通知改成「請更新」 |
| `changelog` | Release 頁面 | 只記錄 |
| `privacyChanged` | 這一版的隱私權政策有實質修改 | 通知裡加一句；不需要重新同意（D-017 第 6 點） |
| `notes` | 一句話，顯示在通知裡 | 只取一行、最多 80 字 |

`minGFSync`、`privacyChanged`、`notes` 來自 repo 根目錄的 `release-manifest.json`，**發布前**手動修改（見 RELEASE.md）。其他欄位由 CI 填。

### 1.1 `SHA256SUMS.txt`

CI 在簽章之後產生，格式是 `sha256sum` 的格式（`<雜湊>  <檔名>`），**最後一行是清單自己的雜湊**：

```
<雜湊>  GFSync.exe
<雜湊>  GFSync-Setup-0.2.0.exe
<雜湊>  gfsync-manifest.json
```

### 1.2 GF Sync 怎麼核對

1. 先跟網站拿清單：`GET https://greenandfangadventure.com/api/sync/gfsync-manifest.json`。
2. 網站回 **404** 時，改跟 GitHub 拿：`https://github.com/GreenAndFangAdventure/gf-sync/releases/latest/download/gfsync-manifest.json`。其他錯誤（5xx、逾時）就等下次。
3. 依清單的 `version` 下載 `https://github.com/GreenAndFangAdventure/gf-sync/releases/download/v<version>/SHA256SUMS.txt`。
4. 清單**原始位元組**的 SHA-256 必須等於 `SHA256SUMS.txt` 裡 `gfsync-manifest.json` 那一行；`sha256` 欄位必須等於安裝檔那一行。任何一項不符就整份不採用，只寫記錄檔。
5. 版本比較：`x.y.z` 依數字比，`x.y.z-標籤` 比 `x.y.z` 舊。比目前新才通知；**不會**降級。

repo 名稱、網址都寫死在程式裡（`internal/update`），不從網路讀。只用 HTTPS；轉址可以，但不能轉到 http。

### 1.3 頻率和行為

- GF Sync 啟動 2 分鐘後檢查一次，之後每 24 小時（加最多 2 小時的隨機延遲）。工作列選單「檢查更新」會立刻檢查。
- 開發版和 CI 測試版（`0.0.0-*`）不檢查。
- 找到新版時：工作列選單多一行「⬆ 有新版本 GF Sync 0.2.0（點這裡下載）」，並跳一次 Windows 通知（每個版本一次）。點了開 Release 頁面。
- **不下載、不執行任何檔案。** 有簽章之後才會加自動安裝（AUTO_UPDATE.md §1.3–1.5）。

## 2. 網站要做的事（給 GearBot）

### 2.1 提供清單

- 路徑：**`GET /api/sync/gfsync-manifest.json`**（不用登入、不用 token）。
- 內容：**最新一個已發布（不是草稿）的 GF Sync Release 的 `gfsync-manifest.json`，位元組完全一樣**。不要重新排版、不要改換行、不要加 BOM、不要轉碼；GF Sync 會用 SHA-256 核對，改一個位元組就不會被採用。
- `Content-Type: application/json`。可以快取（建議 `Cache-Control: max-age=3600`）。
- 還沒有任何清單時回 **404**（GF Sync 會改去 GitHub 拿）。
- 怎麼同步由網站決定：例如定時抓 `https://github.com/GreenAndFangAdventure/gf-sync/releases/latest/download/gfsync-manifest.json` 存起來，或 owner 發布後手動上傳。**網站可以晚一點換新版本**（用來分批推送或暫緩），但不能改內容。
- 工具箱的 `addon-manifest.json` 也照同樣的原則（原樣提供），路徑由插件那邊的合約決定。

### 2.2 擋太舊的 GF Sync：HTTP 426

網站要停止接受某個版本以下的 GF Sync 時，**只能用 426**。理由見下表（v0.1.0 已經在玩家電腦上，行為改不了）：

| 網站回應 | v0.1.0 會怎樣 |
| --- | --- |
| 400、413 | 把一批切半重送，切到 1 筆還失敗就把那筆記成 rejected，**資料不見** |
| 401、403 | 刪掉配對 token，要求重新配對 |
| 200 + `rejected` | 記成已回覆，**資料不見** |
| **426** | 當成伺服器錯誤，退避重試，**資料留著** |

**適用的端點：** `POST /api/sync/upload` 和 `POST /api/sync/device/start`。`POST /api/sync/device/poll` **不要**回 426（舊版會把它當成配對失敗，訊息不對）；擋 start 就夠了。

**怎麼判斷版本：** upload 看 body 的 `client.version`，start 看 body 的 `clientVersion`。比較規則同 §1.2 第 5 點（`0.1.0-beta1` < `0.1.0`）。沒有版本或看不懂的版本，**不要**擋（當成可以）。

**回應：**

```http
HTTP/1.1 426 Upgrade Required
Content-Type: application/json; charset=utf-8

{"error":"client_too_old","minVersion":"0.2.0","download":"https://greenandfangadventure.com/pages/sync"}
```

- `error`：固定 `"client_too_old"`。
- `minVersion`：網站接受的最低版本。
- `download`：下載頁。**必須和 API 同一個網域**（`https://greenandfangadventure.com/...`），否則 GF Sync 改開 GitHub 的 Release 頁。路徑由網站決定。
- 網站可以不把 426 算進每小時 60 次的上限；GF Sync 看到 426 之後 6 小時內不會再送。

**新版 GF Sync（GF-001-J 之後）看到 426：** 不記任何資料為已回覆，停止上傳 6 小時（不退避、不浪費每小時次數），工作列顯示「這版 GF Sync 太舊，網站暫時不收資料。請更新 GF Sync（資料會先留在電腦上）」，點了開 `download`，並立刻檢查更新。配對時遇到 426 也一樣，不開瀏覽器。

**上線方式：** 先只**記錄**會被擋的版本（不擋）一段時間，確認大家都更新了，再開始回 426。上線順序照舊：網站 → GF Sync → 插件。

### 2.3 插件太舊：單筆 `addon_too_old`

網站要拒收太舊的插件寫的資料時，在 upload 的結果裡對那一筆回：

```json
{"id":"<id>","status":"rejected","reason":"addon_too_old"}
```

- 新版 GF Sync：**不記成已回覆**，24 小時後重送（和 `unknown_kind` 一樣），並在工作列顯示「綠皮冒險工具箱的版本太舊……請更新工具箱」。之後有一批被接受、而且沒有 `addon_too_old` 時，提示就消失。
- v0.1.0：會記成已回覆，那一筆不再送。插件太舊的資料丟掉可以接受。
- 和 `unknown_kind` 一樣，網站**不要記住**這些 id 的狀態，之後同一個 id 要能被接受。
- **網站怎麼知道插件版本（pm 決定）：** GF Sync 上傳時在請求的 `client` 裡放 `addonVersion`，值是 SavedVariables 最上層的 `addonVersion`：

  ```json
  { "client": { "name": "GFSync", "version": "0.2.0", "addonVersion": "0.10.0" }, "items": [ ... ] }
  ```

  - **同一個請求的每一筆都來自同一個插件版本**（GF Sync 分批時依版本切開，通常就是同一個檔案），所以一個值就夠。
  - 檔案沒有 `addonVersion`，或者它不是短的版本字串（最多 32 個字元，只有英數字和 `.+-`）時，**不送這個欄位**。網站看到沒有 `addonVersion` 時要怎麼處理由網站決定（很舊的插件，或 v0.1.0 的 GF Sync）。
  - 舊的插件不用改。v0.1.0 的 GF Sync 不會送這個欄位。
  - 網站依 `client.addonVersion` 判斷，再對每一筆回 `addon_too_old`。

## 3. 測試

- `internal/update`：清單從網站、從 GitHub（網站 404 時）、各種被改過的清單（網站那份改了一個位元組、安裝檔雜湊不符、別的 repo、http、別的檔名、CI 版本、壞格式、沒有 `SHA256SUMS.txt`）都不採用；版本比較。
- `internal/engine/e2e_update_test.go`：上傳遇到 426 不丟資料、不重試、立刻檢查更新、6 小時後網站接受就送出；配對遇到 426 不開瀏覽器；`addon_too_old` 24 小時後重送；檢查更新只通知（狀態帶 Release 頁面）；開發版不檢查。
- `cmd/gfsync/text_test.go`：通知和選單文字、426 時點 ⚠ 是開下載頁。
- 本機只跑了 `go vet` 和 `go build`（Trend 擋 `go test`），測試要等 CI。
