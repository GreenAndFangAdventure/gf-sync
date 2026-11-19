# GF Sync（綠皮同步）實機測試步驟

對象：owner。離線測試已由 `go test ./...` 涵蓋，這份清單是只有在真的 Windows 桌面、遊戲和瀏覽器上才能驗證的部分。每一步後面的「預期」沒有出現，請把 `%APPDATA%\GFSync\gfsync.log` 的內容附在 issue 裡（記錄檔不會有 token，但有遊戲資料夾路徑和帳號資料夾名稱，公開前可以先遮掉）。

需要：Windows 10/11、Go 1.27、Inno Setup 6（`winget install --id GoLang.Go -e`、`winget install --id JRSoftware.InnoSetup -e`）。

## 0. 建置

```powershell
powershell -ExecutionPolicy Bypass -File build.ps1 -Version 0.1.1
```

預期：測試全部 `ok`，產生 `dist\GFSync.exe`、`dist\GFSync-Setup-0.1.1.exe`、`dist\SHA256SUMS.txt`。

## 1. 對模擬伺服器配對（不需要網站）

1. 開一個 PowerShell，在 repo 根目錄執行：`go run ./cmd/gfsync-mock`（只聽 127.0.0.1:8787）。
2. 另一個 PowerShell：`dist\GFSync.exe --api http://127.0.0.1:8787`
3. 預期：
   - 工作列右下角出現公會盾牌圖示（右下角黃點＝尚未配對）。
   - 瀏覽器自動開啟 `http://127.0.0.1:8787/pages/sync-pair?code=XXXXXXXX`。
   - 跳出「已經幫你開啟瀏覽器」視窗（在最上層，不會被瀏覽器蓋住），顯示 `XXXX-XXXX` 格式的配對碼，和網頁上的一致，並有「重新開啟配對頁」和「關閉」兩個按鈕。按「重新開啟配對頁」會再開一次網頁，視窗不會關掉。另外有「複製配對碼」按鈕：按下後視窗底部顯示「已複製配對碼 XXXXXXXX…」，在記事本貼上時只會出現 8 個英數字（沒有連字號、空格或說明文字）。
   - 工作列選單在配對中多一項「複製配對碼（XXXX-XXXX）」，效果和按鈕一樣，並跳出「已複製配對碼」的通知。配對完成或過期後，這一項會消失。
   - `%APPDATA%\GFSync\gfsync.log` 依序出現 `pair: starting`、`pair: start ok, code expires in …s`、`pair: browser opened`、`pair: poll: waiting for approval`。記錄檔裡**不應該**出現配對碼。
4. 在網頁按「允許這台電腦同步」。
5. 預期：幾秒內配對視窗自己關掉、圖示的黃點消失，出現一次通知「GF Sync 已連結：之後下線就會自動同步…」；右鍵選單顯示「狀態：已配對」；記錄檔有 `pair: device paired`。之後重新啟動 GF Sync 不會再出現這個通知。
5a. 配對碼過期：刪掉 `%APPDATA%\GFSync\token.dat`，用 `go run ./cmd/gfsync-mock -expires 30` 重新啟動模擬伺服器和 GF Sync，不要按「允許」。預期：時間到後配對視窗自己關掉；滑鼠移到圖示上顯示「配對碼已過期，點這裡重新配對」；選單最上面的 ⚠ 那行和「配對」項目也是這句話，點下去會重新開始配對；記錄檔有 `pair: code expired` 或 `pair: ended by the website`。
6. 再開一次 `GFSync.exe`：預期跳出「GF Sync 已經在執行了」，不會出現第二個圖示。

## 2. 遊戲存檔 → 上傳

前提：遊戲裡已安裝綠皮冒險工具箱 v0.4 以上（SavedVariables schemaVersion 2）。

1. 保持步驟 1 的兩個程式在執行。登入永恆服，`/reload` 或登出。
2. 預期：遊戲寫完檔大約 5–10 秒後，模擬伺服器的視窗印出 `received <id> (character.self)` 等行；選單「上次同步：今天 HH:MM」。
3. 再登出一次，資料沒變：預期不會重複上傳同一個 id（伺服器視窗沒有新的 `POST /api/sync/upload`，或只上傳新的 id）。
4. 在遊戲輸入 `/gf sync off` 後登出：預期不再上傳。`/gf sync on` 再登出：恢復。
5. 確認 GF Sync **沒有寫入** 遊戲資料夾：`WTF\Account\*\SavedVariables\GreenFangToolkit.lua` 的修改時間只會在遊戲登出時改變；`Interface\AddOns` 沒有任何新檔。

## 3. 工作列選單

| 動作 | 預期 |
| --- | --- |
| 立即同步 | 不用等存檔變動，馬上重新讀取並上傳待傳資料 |
| 暫停同步（打勾） | 圖示變灰；登出遊戲也不上傳；取消勾選後補傳 |
| 重新配對 | 先問「要重新配對嗎？」，按「是」再走一次步驟 1 |
| 選擇遊戲資料夾… | 開啟資料夾選擇器；選 `World of Warcraft` 或 `_forever_` 都可以；選錯會提示 |
| 開啟公會網站 | 開啟 `/pages/forever` |
| 開啟記錄檔資料夾 | 開啟 `%APPDATA%\GFSync` |
| 結束 GF Sync | 圖示消失 |

錯誤情境：

- 關掉模擬伺服器再登出遊戲：預期圖示右下角出現紅點，選單顯示「無法連線到公會網站，稍後自動重試」；重開模擬伺服器後（最長約 30 分鐘，按「立即同步」可馬上重試）自動補傳，紅點消失。
- `go run ./cmd/gfsync-mock -fail 3`：前三次上傳失敗，之後自動成功。
- 找不到遊戲：把 `%APPDATA%\GFSync\state.json` 刪掉，並暫時把 `_forever_` 改名（或在沒有遊戲的電腦上測），啟動後預期跳出「找不到魔獸世界：永恆的資料夾」並開啟資料夾選擇器。

## 4. 安裝檔

1. 先核對 SHA-256：`Get-FileHash dist\GFSync-Setup-0.1.1.exe -Algorithm SHA256`，和 `SHA256SUMS.txt` 一致。
2. 執行 `GFSync-Setup-0.1.1.exe`。因為沒有數位簽章，預期 Windows SmartScreen 顯示「Windows 已保護您的電腦」：按「其他資訊」→「仍要執行」。**請截圖給網站下載頁的說明用。**
3. 預期：先顯示「隱私權政策」頁（內容是 `PRIVACY.md`，中文正常顯示、沒有亂碼）；不會跳 UAC（安裝在 `%LOCALAPPDATA%\Programs\GFSync`）；「開機時自動啟動 GF Sync（建議）」預設勾選；完成頁可勾「現在啟動 GF Sync」。
4. 檢查開機啟動：按 Win+R 輸入 `shell:startup`，預期有「綠皮同步（GF Sync）」捷徑，指向 `...\Programs\GFSync\GFSync.exe`；登錄檔 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` **不應該**有 `GFSync`（舊版 0.1.0 裝過的除外）。重新開機後圖示自動出現。
5. 重新安裝並取消勾選自動啟動：預期「啟動」資料夾裡的捷徑被移除。
6. 防毒：Trend Micro、Windows Defender 有沒有警告或隔離？請記錄偵測名稱。在 `GFSync.exe` 按右鍵 → 內容 → 詳細資料，預期看到公司「綠皮與獠牙冒險事務所」、產品名稱「GF Sync」、檔案描述「GF Sync」、檔案版本與著作權；安裝檔的產品名稱也是「GF Sync」。
7. 解除安裝：分別試「工作列選單 → 解除安裝 GF Sync…」、「開始」選單的「解除安裝 綠皮同步（GF Sync）」、「設定 → 應用程式」三種方式。預期程式被關閉、`%LOCALAPPDATA%\Programs\GFSync`、`%APPDATA%\GFSync`、「啟動」資料夾的捷徑都移除；魔獸世界資料夾不受影響。

8. 如果遇到 GF Sync 讀不到遊戲資料夾或自己的檔案（例如被防毒軟體限制）：
   - 工作列選單的 ⚠ 那一行應該寫「找到遊戲資料夾，但 GF Sync 沒有權限讀取…」或「GF Sync 無法開啟自己的記錄檔／設定與上傳紀錄／配對資料…」，**不應該**寫「找不到魔獸世界：永恆的資料夾」，也不應該跳出選擇資料夾的視窗；點這一行會開啟 README 的「如果被防毒擋下」。
   - 圖示右下角是紅點；配對資料讀不到時，狀態寫「配對資料無法讀取」，不會自動開配對頁。
   - `%APPDATA%\GFSync\state.json`、`token.dat` 的修改時間不變（GF Sync 不會重設或刪除它們）。
   - 防毒放行後重新啟動 GF Sync，⚠ 消失，`gfsync.log` 有「readable again」或正常的啟動紀錄。
   - 這一項只記錄結果；**不要**為了測試而修改防毒設定。

注意：安裝版沒有 `--api` 參數，會連正式網站。要讓安裝版連模擬伺服器，先設定使用者環境變數 `GFSYNC_API_BASE=http://127.0.0.1:8787` 再啟動（測完記得刪掉）。

## 5. 正式網站

1. 刪掉環境變數 `GFSYNC_API_BASE`，執行 `GFSync.exe`（不帶參數）。
2. 預期瀏覽器開啟 `https://greenandfangadventure.com/pages/sync-pair?code=...`，用 Discord 登入後允許。
3. 登出遊戲，確認網站名冊有更新；在 `/pages/sync-devices` 撤銷裝置後，GF Sync 預期顯示「這台電腦的配對已失效，請重新配對」。
