# 自動更新設計（GF-001-J）

> 狀態：**設計草稿，還沒有實作。** 這份文件只描述打算怎麼做，GF Sync v0.1.0 目前沒有任何更新機制。標「待確認」的地方要等實機、SignPath 核准或 owner 決定。
>
> 相關文件：[CODE_SIGNING_POLICY.md](../CODE_SIGNING_POLICY.md)、[RELEASE.md](../RELEASE.md)、[PRIVACY.md](../PRIVACY.md)、[installer/gfsync.iss](../installer/gfsync.iss)、[.github/workflows/build.yml](../.github/workflows/build.yml)。

## 0. 原則

1. **不執行沒有簽章的程式。** 自動安裝只在檔案有 SignPath Foundation 的 Authenticode 簽章、而且 SHA-256 也對得上時才做。簽章還沒核准之前，GF Sync 只**通知**有新版，不自動下載或執行（見 §4.3）。
2. **只用 HTTPS**，只連固定的主機：公會網站、`api.github.com`、`github.com` 的 Release 下載（以及它轉址到的 GitHub 下載主機）。
3. **不影響遊戲。** 檢查和下載都在背景做；自動安裝 GF Sync 只會讓 GF Sync 自己重新啟動。更新插件時，遊戲**一定不能在執行中**。
4. **不需要系統管理員權限。** GF Sync 已經安裝在 `%LOCALAPPDATA%\Programs\GFSync`（`PrivilegesRequired=lowest`），更新也一樣。
5. **不要求防毒例外。** 不加例外、不換資料夾躲偵測、不關閉任何防護。遇到誤判就照 RELEASE.md 第 3 步回報。
6. **出錯時不丟資料。** 更新失敗或被版本握手擋下時，outbox 和已上傳紀錄都保持原樣，之後再送。

## 1. GF Sync 自我更新

### 1.1 去哪裡檢查：建議用網站上的清單，再用 GitHub 交叉確認

| 方案 | 優點 | 缺點 |
| --- | --- | --- |
| A. GitHub Releases API（`GET https://api.github.com/repos/GreenAndFangAdventure/gf-sync/releases/latest`） | 不用改網站；Release 是 CI 建置的，來源清楚 | 沒有登入時每個 IP 每小時只能呼叫 60 次；沒辦法分批推送或緊急撤回；「最新版」就等於「全部人都要更新」 |
| B. **網站上的清單**（建議） | 可以分批推送、緊急撤回、指定最低版本；跟插件清單用同一種格式；本來就要連網站 | 網站要新增一個檔案或端點；網站被入侵時清單也可能被改（所以還需要簽章和交叉確認，見下面） |

**建議用 B 當主要來源，A 當交叉確認：**

- 網站提供 `GET https://greenandfangadventure.com/api/sync/client/latest`（也可以是靜態檔案，路徑由網站決定）。格式和插件清單一致：

  ```json
  {
    "product": "gfsync",
    "version": "0.2.0",
    "url": "https://github.com/GreenAndFangAdventure/gf-sync/releases/download/v0.2.0/GFSync-Setup-0.2.0.exe",
    "sha256": "<64 位十六進位，小寫>",
    "minGfSync": "0.1.1",
    "rolloutPercent": 100,
    "notes": "這一版修正了……（短短一句，顯示在通知裡）",
    "privacyChanged": false
  }
  ```

  - `minGfSync`：比這個版本還舊的 GF Sync 就**一定要**更新（見 §3）。
  - `rolloutPercent`：分批推送。GF Sync 用 `sha256("gfsync-rollout|" + deviceId)` 的前 2 個位元組換算成 0–99，小於這個數字才更新。沒有 deviceId（還沒配對）時當成 0，也就是最先拿到更新。
  - `privacyChanged`：這一版的 PRIVACY.md 有實質修改時是 `true`，見 §1.6。
- GF Sync 會檢查：
  1. `url` 一定要是 `https://github.com/GreenAndFangAdventure/gf-sync/releases/download/v<version>/GFSync-Setup-<version>.exe`。repo 名稱寫死在程式裡，不接受其他網址。
  2. 再到 `https://github.com/GreenAndFangAdventure/gf-sync/releases/download/v<version>/SHA256SUMS.txt` 拿雜湊，必須和清單的 `sha256` **一模一樣**。這樣只入侵網站，或只入侵 GitHub 帳號，都不夠。
  3. 下載完成後再算一次 SHA-256，並驗 Authenticode 簽章（§1.3）。
- 下載時跟著 HTTPS 轉址走，但只接受 GitHub 自己的下載主機（實際主機名稱要用第一個 Release 確認，**待確認**）。檔案最多 50 MB。

### 1.2 多久檢查一次

- GF Sync 啟動 2 分鐘後檢查一次，之後每 24 小時一次，再加最多 2 小時的隨機延遲，避免所有人同一時間打網站。
- 工作列選單加「檢查更新」，按了就立刻檢查（同樣受下面的限制）。
- 檢查失敗（連不上、格式錯）只記在記錄檔，不跳通知，下次照時間再試。
- 一天最多 4 次清單請求、1 次安裝檔下載；GitHub API 不需要（只抓 `releases/download/` 的檔案，不用 API）。

### 1.3 下載和驗證

1. 下載到 `%LOCALAPPDATA%\GFSync\updates\<version>\GFSync-Setup-<version>.exe.part`，完成後才改名。不放 `%TEMP%`。（注意：配對 token 等資料目前在 `%APPDATA%\GFSync`，安裝檔不需要漫遊，所以放 `%LOCALAPPDATA%`。）
2. 算 SHA-256，和清單、`SHA256SUMS.txt` 比對。不一致就刪檔，記一行記錄，24 小時內不再試這一版。
3. 驗 Authenticode：
   - 呼叫 `WinVerifyTrust`（`WINTRUST_ACTION_GENERIC_VERIFY_V2`），只接受 `ERROR_SUCCESS`。撤銷檢查用 `WTD_REVOKE_WHOLECHAIN`；連不上 CRL/OCSP 時當作失敗，下次再試。
   - 取出簽署者憑證，檢查主體名稱是 **SignPath Foundation**（確切字串要用第一個簽章版本確認，**待確認**）。不釘選憑證指紋，因為憑證會換；改成「名稱 + 這個名稱也必須和正在執行的 GFSync.exe 的簽署者相同」。
   - 正在執行的 GF Sync 本身沒有簽章時（v0.1.0），不做自動安裝，見 §4.3。
4. 全部通過後才執行。驗證和執行之間，檔案用不共用寫入的方式開著，避免被換掉。

### 1.4 安裝和重新啟動

GF Sync 用一般的 `CreateProcess` 直接執行安裝檔（不透過 cmd、PowerShell 或腳本）：

```
GFSync-Setup-0.2.0.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /CLOSEAPPLICATIONS /LOG="%LOCALAPPDATA%\GFSync\updates\0.2.0\setup.log" /GFUPDATE
```

- 安裝目錄、「開機自動啟動」「桌面捷徑」都沿用上次的選擇（Inno 預設 `UsePreviousTasks=yes`）。
- 現在的 `[Run]` 有 `skipifsilent`，靜默安裝後**不會**重新啟動 GF Sync。
- 建議的重新啟動方式：GF Sync 啟動時呼叫 `RegisterApplicationRestart`，安裝檔改成 `RestartApplications=yes`。GF Sync 執行安裝檔後**不自己結束**，而是讓安裝檔透過 Restart Manager 關掉它（GF Sync 收到關閉訊息時先存好 `state.json`）。安裝成功時 Restart Manager 啟動同一路徑的新版；安裝失敗時 Inno 先復原檔案，再啟動舊版。這樣兩種情況都會回到有 GF Sync 在跑的狀態。**待確認**：Restart Manager 對沒有視窗的工作列程式的實際行為，要在實機測。
- 備案：在 `gfsync.iss` 加一行只在 `/GFUPDATE` 時執行的 `[Run]`（用 `Check:` 讀命令列參數），安裝成功後啟動新版；失敗時則要靠下次開機的「啟動」資料夾捷徑。
- 新版用 `--updated-from=<舊版本>` 參數知道自己是剛更新的。單一執行個體的 mutex（`Local\GFSync-SingleInstance`）不變。
- 時機：不在上傳中途；遊戲執行中也可以裝（GF Sync 本來就不碰遊戲檔案），但為了少打擾，預設等 GF Sync 閒置（沒有待上傳）時才裝。

### 1.5 復原

| 失敗點 | 處理 |
| --- | --- |
| 下載、雜湊、簽章失敗 | 不執行，舊版照常跑。 |
| 安裝檔執行失敗 | Inno 自己會復原到原本的檔案，Restart Manager 再啟動舊版（§1.4）。重新啟動的舊版看到 `updates\pending.json` 的版本比自己新、`setup.log` 裡有失敗，就知道更新失敗了，記一行記錄，這一版 24 小時內不再試。用備案時，舊版要等下次開機才會再啟動。 |
| 新版啟動後一直當掉 | 安裝前把**目前版本**的安裝檔留在 `updates\previous\`（只留一份，從上次自動更新的下載保留，或者向 GitHub 重抓同版本並同樣驗證）。新版啟動時把「啟動次數」記在 `updates\pending.json`，成功跑完一次讀檔和一次 API 呼叫（或 5 分鐘沒當掉）後才清掉。連續 3 次啟動都沒清掉，就靜默執行 `previous` 的安裝檔（Inno 允許裝舊版），並在通知裡說「新版有問題，已經回到 0.1.1」，這一版 7 天內不再自動安裝。 |
| 整批版本有問題 | owner 把網站清單的 `version` 改回上一版，或把 `rolloutPercent` 調成 0。已經裝新版的人不會自動降級，除非清單加上 `"rollback": true`（這時用上面的 `previous` 流程）。 |

**資料格式相容：** `state.json`、token 檔案每次改格式時，舊一版必須還讀得懂（或者那一版的清單標 `"rollback": false`）。目前 `state.json` 有 `Version: 1`，之後改版要遵守這條。

### 1.6 玩家看到什麼

- 不會跳出視窗，也不會搶焦點，遊戲畫面不受影響。
- 工作列氣球通知（沿用現有的 notify_windows.go），全部用中文：
  - 有新版、還沒簽章（§4.3）：「GF Sync 有新版本 0.2.0，請到公會網站下載。」點一下開下載頁。
  - 自動更新完成：「GF Sync 已經更新到 0.2.0。」只在新版啟動後顯示一次。
  - 被要求一定要更新（§3）：「這版 GF Sync 太舊，網站暫時不收資料。請更新 GF Sync（資料會先留在電腦上）。」
  - 回到舊版：「新版 GF Sync 有問題，已經回到 0.1.1。」
- 工作列選單加「關於／版本 0.2.0」和「檢查更新」。可以另外加「自動更新」勾選項，預設勾選；取消就只通知。
- `privacyChanged = true` 的版本：**不靜默安裝**，只通知並開 PRIVACY 頁，讓玩家看過後按「更新」。理由：安裝檔的隱私權頁（`InfoBeforeFile`）在 `/VERYSILENT` 時不會顯示，而隱私權政策有實質修改時應該讓人看到。是否需要重新同意，由 owner 決定（**待 owner**）。

### 1.7 Trend Micro 行為監控可能的反應

這段是推測，**沒有實機測試過**。v0.1.0 的 `GFSync.exe` 曾被 Trend Micro 的機器學習偵測（`TROJ.Win32.TRX.XXPE50FFF111*`）隔離過，自我更新只會讓情況更敏感：

- **「常駐程式從網路下載執行檔，然後執行」本身就是下載器（dropper）的典型行為。** 沒有簽章的程式這樣做，很可能被行為監控攔下或隔離，所以簽章之前不做自動安裝。
- 有簽章之後風險比較低，但每一版的雜湊都不同，新版剛發布時信譽是零，機器學習偵測仍然可能誤判新的安裝檔或新的 `GFSync.exe`。
- 安裝檔結束舊的 GF Sync、替換 `Programs\GFSync` 裡的 exe，也可能觸發「修改執行檔」的規則。
- 被攔下時的結果：下載的安裝檔被隔離 → 雜湊比對失敗或檔案不見 → GF Sync 只記錄「更新檔不見了（可能被防毒軟體隔離）」，舊版照常跑，**不重試同一版**，並在通知裡請玩家到網站手動下載。不嘗試換路徑、換檔名或任何躲避偵測的做法。

降低誤判、但**不要求例外**的做法：

1. 安裝檔和 `GFSync.exe` 都簽章，簽署者一致。
2. 從 GitHub 的 HTTPS 下載，放在 GF Sync 自己的資料夾，用一般的 `CreateProcess` 執行，不經過 cmd、PowerShell 或腳本，不改登錄檔的 Run 機碼（沿用「啟動」資料夾捷徑）。
3. **分批推送：** 每一版先在 owner 有裝 Trend 的電腦上手動安裝並跑一次自動更新（`rolloutPercent` 先設 0，只有 owner 的裝置用一個測試旗標更新），再照 RELEASE.md 第 3 步送誤判回報，最後才把 `rolloutPercent` 調高。
4. 記錄檔寫清楚每一步（下載、雜湊、簽章、執行、結果），方便回報時附上。

## 2. 插件更新（選用，只給從公會網站安裝的玩家）

### 2.1 範圍

- 只更新 `Interface\AddOns\GreenFangToolkit`（`_forever_` 底下，由現有的 gamedir 偵測）。
- 只在玩家**在 GF Sync 裡啟用「讓 GF Sync 更新綠皮冒險工具箱」**時才做，預設關閉，或者只在 GF Sync 自己裝過插件之後才預設開啟（**待 owner**）。
- 從 CurseForge 或其他管理工具安裝的，一律不碰（§2.4）。

### 2.2 清單

用插件團隊設計的清單（`version`、`url`、`sha256`、`minGfSync`）。GF Sync 這邊的要求：

- 清單從公會網站拿（固定網址，**待插件團隊和網站決定**）。
- `url` 只接受 GitHub Release（插件 repo 名稱寫死，**待確認** repo）或公會網站的固定路徑，一律 HTTPS。
- `minGfSync` 比目前的 GF Sync 新時，先更新 GF Sync，不裝插件。
- 建議清單裡再加 `files`（zip 裡每個檔案的路徑和 SHA-256），讓 GF Sync 可以檢查本機的插件是不是自己裝的（§2.4）。

### 2.3 流程

1. 下載 zip 到 `%LOCALAPPDATA%\GFSync\updates\addon\<version>\`，檢查 SHA-256（和清單一致）。zip 不是執行檔，沒有 Authenticode；所以 zip 的雜湊一定要能和第二個來源交叉確認（插件 Release 的雜湊檔，**待插件團隊**）。
2. 解壓縮前檢查 zip：只能有 `GreenFangToolkit/` 底下的檔案，不能有 `..`、絕對路徑、符號連結；副檔名只允許 `.lua`、`.toc`、`.xml`、`.tga`、`.blp`、`.ogg`、`.mp3`、`.ttf`、`.txt`、`.md`（**待插件團隊**確認清單）；總大小上限 20 MB。解壓縮到暫存資料夾。
3. **確認遊戲沒有在執行：** 列出執行中的程式，找映像路徑在 `_forever_` 資料夾底下的 `Wow.exe`（用 `QueryFullProcessImageName`）。找到就等，每 5 分鐘再看一次，不提示玩家關遊戲。
4. 備份：把現在的 `GreenFangToolkit` 改名成 `%LOCALAPPDATA%\GFSync\updates\addon\backup\GreenFangToolkit-<舊版本>`（同一個磁碟用搬移，不同磁碟就複製）。只留最近 2 份。
5. 把新版搬進 `Interface\AddOns\GreenFangToolkit`。搬移中途失敗就把備份搬回來。
6. 寫入標記檔 `Interface\AddOns\GreenFangToolkit\.gfsync-installed.json`（版本、來源網址、每個檔案的 SHA-256），並在通知裡說「綠皮冒險工具箱已經更新到 0.10.1，下次進遊戲生效」。
7. **不碰 `WTF\` 底下的 SavedVariables。** 插件的資料遷移由插件自己在遊戲裡做。
8. 玩家可以在選單選「還原上一版工具箱」，用備份換回來（同樣要遊戲沒在執行）。

### 2.4 偵測 CurseForge（或其他管理工具）

不依賴 CurseForge app 的內部檔案格式（沒有公開文件，隨時可能改）。改成「**只有確定是 GF Sync 自己裝的，才更新**」：

1. **標記檔：** 只有 `GreenFangToolkit\.gfsync-installed.json` 存在，而且資料夾裡每個檔案的 SHA-256 都和標記檔一致時，才當成 GF Sync 管理的。有任何檔案被改過、多出來或少了，就當成別人管理的，不碰，只通知「有新版」。
2. **發布管道寫在 .toc：** 請插件團隊打包時依管道寫一行，例如 `## X-GF-Channel: site` 或 `## X-GF-Channel: curseforge`。是 `curseforge` 就一定不碰。（CurseForge 的打包工具通常會讓 .toc 帶 `## X-Curse-Project-ID`；那一行如果是插件作者自己寫的，各管道的 .toc 都會有，所以不能單獨拿來判斷，只能當輔助。）
3. **第一次：** 玩家原本是手動或從 CurseForge 安裝的（沒有標記檔），GF Sync 只會在玩家**明確按「讓 GF Sync 接手更新」**後，才用網站的版本覆蓋（同樣先備份）。

## 3. 最低版本握手

### 3.1 現有程式對錯誤碼的反應（v0.1.0，已經在玩家電腦上，改不了）

這決定了網站「拒絕舊版」時**一定要**用哪個狀態碼：

| 網站回應 | v0.1.0 的反應 | 適合用來擋舊版嗎 |
| --- | --- | --- |
| 401、403 | 刪掉 token，要求重新配對 | ❌ 玩家會以為壞了，而且重新配對也沒用 |
| 400、413 | 把一批切成一半重送，切到 1 筆還失敗就把那筆**記成 rejected，不再送** | ❌ **資料會不見** |
| 429 | 依 `Retry-After` 等待 | 可以但意思不對 |
| 200 加 `rejected` | 記成已回覆，不再送 | ❌ 資料會不見 |
| 其他（例如 **426**、409、410、5xx） | 退避重試，資料留著 | ✅ |

**結論：網站擋太舊的 GF Sync 時，回 `426 Upgrade Required`。** body：

```json
{ "error": "client_too_old", "minVersion": "0.2.0", "download": "https://greenandfangadventure.com/pages/sync" }
```

- v0.1.0 看到 426 會退避重試（最多 `BackoffMax`），工作列顯示「無法連線到公會網站」。訊息不精準，但資料不會丟。網站的下載頁和 Discord 公告要說明。
- 新版 GF Sync 看到 426 + `client_too_old`：停止上傳（不退避、不浪費每小時次數），顯示 §1.6 的「一定要更新」通知，馬上檢查更新；更新完成後 outbox 照常送出。
- 判斷依據：上傳請求本來就有 `client.version`（upload-api v1）。配對的 `/api/sync/device/start` 也有 client 資訊時，網站同樣回 426，新版會顯示說明而不是一直重試。
- **426 寫進 upload-api 合約**（GreenFang_Toolkit 的 `docs/contracts/upload-api.md`，由 pm 修改；這邊不改合約）。

### 3.2 插件版本

- 插件已經寫 `addonVersion`，GF Sync 讀得到（`svdoc.Doc.AddonVersion`）。
- **插件太舊：** 網站對單筆回 `rejected` + `reason = "addon_too_old"`。舊版 GF Sync 會把它記成已回覆（太舊的插件資料丟掉可以接受）；新版 GF Sync 把它當成 `unknown_kind` 一樣處理（不記成已回覆，24 小時後再試），並通知「請更新綠皮冒險工具箱」。插件更新後，新的資料會用新的 id 進來。
- **插件需要較新的 GF Sync：** 插件的清單有 `minGfSync`；另外建議插件在 SavedVariables 寫 `sync.minGfSync = "0.2.0"`。GF Sync 讀到比自己新的版本時，顯示「綠皮冒險工具箱需要較新的 GF Sync」並檢查更新（未知欄位本來就會被忽略，舊版 GF Sync 不受影響）。這要改 savedvariables 合約（**待 pm**）。
- 上線順序照舊：**網站 → GF Sync → 插件**。網站要先能接受新舊兩種，再開始擋舊版。

## 4. 安全

### 4.1 威脅和對策

| 威脅 | 對策 |
| --- | --- |
| 網站被入侵，清單被改成惡意檔案 | 網址只接受固定 repo 的 GitHub Release；雜湊要和 GitHub 的 `SHA256SUMS.txt` 一致；Authenticode 簽署者必須是 SignPath Foundation。 |
| GitHub 帳號被入侵，上傳惡意 Release | 雜湊要和網站清單一致；簽章要 SignPath 簽（每次都要 Approver 人工核准）。組織已經強制 MFA。 |
| 中間人攻擊 | 只用 HTTPS，使用 Windows 的憑證驗證，不關閉驗證，不接受 http 轉址。 |
| 降級攻擊（叫你裝舊的有漏洞版本） | 只接受比目前新的版本；降級只在 §1.5 的復原流程、而且清單 `rollback: true` 時才做。 |
| 下載後到執行前被換掉 | 驗證完到執行都不共用寫入；檔案放在使用者自己的資料夾。 |
| 惡意 zip（路徑穿越） | §2.3 第 2 步的檢查。 |

### 4.2 固定的設定（寫死在程式裡，不從網路讀）

- GitHub repo：`GreenAndFangAdventure/gf-sync`（插件 repo **待確認**）。
- 清單網址：`https://greenandfangadventure.com/...`（和現有的 `api.DefaultBase` 一致；測試時可以用 `--api` 指到本機模擬伺服器，但**只有**指到 loopback 時才允許跳過簽章檢查，給 e2e 測試用）。
- 簽署者名稱：SignPath Foundation（**待確認**確切字串）。

### 4.3 簽章之前（v0.1.0 → v0.1.1）

- **v0.1.0 沒有更新功能，玩家一定要手動裝 v0.1.1。** 網站下載頁和 Discord 公告要說明。
- **建議 v0.1.1 就帶「檢查更新、只通知」的功能**：看到新版時通知並開下載頁，但不下載、不執行。這樣即使 v0.1.1 簽章來不及，也不會執行沒簽章的程式。
- 自動安裝的程式碼要等到「正在執行的 GF Sync 有簽章」**而且**「下載的安裝檔有簽章」都成立才會啟用。所以第一個會自動安裝的升級，最早是 v0.1.x（有簽章、有更新功能）→ 下一版。
- 沒有簽章的 CI 測試版（`0.0.0-ci.*`）不會出現在清單上，GF Sync 也不會把它當成新版。

## 5. 實作步驟

工作量是粗估（一個人、含測試，不含等待審核的時間）。

| # | 步驟 | 誰 | 工作量 | 需要 owner |
| --- | --- | --- | --- | --- |
| 1 | 把 426 `client_too_old`、`addon_too_old`、`sync.minGfSync` 寫進合約 | pm | 0.5 天 | 核准合約 |
| 2 | 網站：清單端點、426 握手（先只記錄不擋）、下載頁說明 | 網站 session | 1 天 | 決定清單路徑；上線 |
| 3 | GF Sync：426 處理（停止上傳、通知、不浪費次數）；`addon_too_old` 照 `unknown_kind` 處理；讀 `sync.minGfSync` | claude-sync | 1 天 | — |
| 4 | GF Sync：檢查更新、只通知（清單、交叉確認雜湊、工作列選單「檢查更新」「關於」）。**建議放進 v0.1.1** | claude-sync | 1.5 天 | 決定是不是放進 v0.1.1 |
| 5 | SignPath 核准後：打開 build.yml 的簽章步驟，在 CI 驗證簽署者名稱（`Get-AuthenticodeSignature`） | owner + claude-sync | 0.5 天 | SignPath 設定、secret、每次核准 |
| 6 | GF Sync：Authenticode 驗證（`WinVerifyTrust` + 簽署者名稱）、下載、靜默安裝、`gfsync.iss` 加 `/GFUPDATE` 的 `[Run]` | claude-sync | 2 天 | — |
| 7 | GF Sync：復原（`previous`、啟動次數、自動回到舊版）、`privacyChanged` | claude-sync | 1.5 天 | 決定隱私權變更要不要重新同意 |
| 8 | 實機測試：Trend 電腦上跑完整的自動更新和復原，記錄結果 | owner | 0.5 天 | 實機；誤判回報 |
| 9 | 插件更新（選用）：清單、zip 檢查、遊戲執行偵測、備份和還原、標記檔、CurseForge 判斷 | claude-sync + 插件 session | 2.5 天 | 決定要不要做、預設開或關 |
| 10 | 文件：PRIVACY（會連 GitHub 下載，GitHub 會看到 IP）、README、MANUAL、RELEASE（發布時更新清單、分批推送） | claude-sync | 0.5 天 | 核准隱私權政策修改 |

步驟 1–4 不需要簽章，可以先做。5–8 要等 SignPath。9 和插件團隊的打包一起做。

### 需要 owner 決定或處理的事

1. 更新清單放在網站哪裡、誰在發布時更新它（建議：RELEASE.md 第 4 步發布 Release 之後，手動改清單，先 `rolloutPercent: 0`）。
2. SignPath：核准後的 organization id、project、signing policy，以及 secret `SIGNPATH_API_TOKEN`；每次發布在 SignPath 核准。另外請確認 SignPath Foundation 的條款對「程式會自動下載並安裝自己的新版」有沒有額外要求（**待確認**）。
3. GitHub：Release 發布後不要再換檔案（如果 GitHub 有「不可變更的 Release」設定，建議打開，**待確認**有沒有這個選項）；`main` 的分支保護。
4. 自動安裝預設開或只通知；插件更新要不要做、預設開或關。
5. 隱私權政策有實質修改時，要不要讓玩家重新同意。
6. 每一版在有 Trend 的電腦上實測自動更新，並送誤判回報後再擴大推送。
