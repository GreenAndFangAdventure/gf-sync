# GF Sync 發布檢查清單

每次發布新版 `GFSync-Setup-<版本>.exe` 都照這份做。GF Sync 沒有數位簽章，新版本的檔案雜湊每次都不同，防毒軟體容易誤判（Trend Micro Maximum Security 曾以機器學習偵測 `TROJ.Win32.TRX.XXPE50FFF111*` 隔離 `GFSync.exe`）。取得 SignPath 簽章前，每一版都要做第 3 步。

> 誤判回報要由 **owner 本人**提交，不要交給自動化工具代為登入、填表或上傳檔案。

## 1. 建置

- [ ] 正式版由 GitHub Actions 建置：在 `main` 上推送 tag `v<版本>`，workflow 會建立**草稿** Release。本機試建可在 repo 根目錄執行 `powershell -ExecutionPolicy Bypass -File build.ps1 -Version <版本>`。內容包含：
  - `go test ./...` 全部通過
  - exe 保留符號（沒有 `-s -w`），帶版本資訊、`asInvoker` manifest 和圖示
  - 安裝檔帶版本資訊
- [ ] 確認 `dist\`（或 workflow 的 artifact）裡有 `GFSync.exe`、`GFSync-Setup-<版本>.exe`、`SHA256SUMS.txt`。如果 `GFSync.exe` 不見了，代表被防毒軟體隔離了：到防毒軟體的隔離區記下偵測名稱，然後做下面第 3 步。**不要**把 `dist` 加入防毒例外來掩蓋問題。
- [ ] 對兩個檔案按右鍵 → 內容 → 詳細資料，確認公司「綠皮與獠牙冒險事務所」、產品名稱、版本號都正確。

## 2. 實機測試

- [ ] 照 `MANUAL.md` 測試安裝、開機自動啟動（「啟動」資料夾捷徑）、配對、上傳和解除安裝。
- [ ] 記錄 SmartScreen 的畫面，以及各防毒軟體有沒有警告。

## 3. 提交誤判回報（owner 手動，每個版本都要做）

只上傳「這次要發布的」安裝檔（必要時連 `GFSync.exe` 一起），說明：這是公會自製的開源同步工具，只讀取魔獸世界的 SavedVariables，再透過 HTTPS 上傳到公會網站。

- [ ] **Trend Micro**：<https://www.trendmicro.com/en_us/about/legal/detection-reevaluation.html>
  - 寫上偵測名稱（從 Trend Micro 的安全報告／隔離區取得）和檔案的 SHA-256。
- [ ] **Microsoft Defender**：<https://www.microsoft.com/wdsi/filesubmission>
  - 選「Software developer」→ 提交為誤判（Incorrectly detected）。
- [ ] 把提交編號和日期記在公會內部的工作記錄，有回覆後再補記結果。

## 4. 發布

- [ ] 確認草稿 Release 的檔案和 `SHA256SUMS.txt`，再按「Publish release」。
- [ ] 把安裝檔放上網站的下載頁（或連到 GitHub Release）。
- [ ] **在下載頁更新 SHA-256**，內容要和 `SHA256SUMS.txt` 一致；舊版本的雜湊要一起換掉。
- [ ] 下載頁保留「防毒誤判」說明：沒有簽章所以 SmartScreen 會警告，以及如何核對 SHA-256、如何回報誤判。
- [ ] 在公會內部的工作記錄記下版本、SHA-256 和發布日期。
