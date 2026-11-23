# 網站訂單清單：API 與簽章建議（給 GearBot，經 claude-pm 轉交）

> 撰寫：claude-sync，2026-11-23。**建議稿**：網站的 API 和簽章方式還沒定，定案由 claude-pm 和 GearBot 決定。
> 依據：D-018、claude-addon 的格式草案 `docs/research/GF-006-C_inbox-format.md`（公開 repo，93e0350）§3、§7。
> GF Sync 這邊已經照這份實作（分支 `claude-sync/GF-006-C-inbox-writer`），用 `gfsync-mock` 測過；**預設關閉**，要等網站好了、有正式公鑰才開。

## 1. 簽章：Ed25519

- **演算法**：Ed25519（RFC 8032，純 Ed25519，不是 Ed25519ph）。理由：簽章固定 64 bytes（base64url 後 86 字元，放得進格式的 1～200 字元）；沒有參數可以選錯；Go、Node（`crypto.sign(null, …)`）、Cloudflare Workers（WebCrypto `Ed25519`）都內建；驗證快。
- **簽什麼**：整個字串去掉第 3 段（`<sig>`）之後的內容，也就是位元組完全等於

  ```
  "GFIN1~" + <sum> + "~" + <第 4 段起的內容>
  ```

  `<sum>` 是草案 §3d 的雜湊（對第 4 段起的內容計算），**先算 sum、再簽章**。簽的是 ASCII 位元組，不做任何正規化。
- **放哪裡**：簽章 64 bytes 用 base64url（RFC 4648 §5，`-`、`_`，**不補 `=`**）放進第 3 段。
- **GF Sync 怎麼驗**：
  1. 整個字串 ≤ 65,536 bytes，只用 `[A-Za-z0-9*+.%_;=~-]`（不符就不寫）。
  2. 第 1 段是 `GFIN1`，第 3 段能解成 64 bytes。
  3. 用內建的公鑰驗章；**任一把**通過就算（換鑰匙期間會同時內建新舊兩把）。
  4. 再檢查 `<sum>`、`H` 表頭（`at`、`n`）、`O` 的數量 = `n` ≤ 200。
  5. `H.at` 比上一份已寫入的舊：不寫（防止舊清單被重放蓋掉新的）。**所以 `H.at` 每次產生清單都要往後推**（同一份內容重新產生也可以用同一個 `at`）。
- **金鑰管理（建議）**：
  - 私鑰只放在網站的 secret（例如 Cloudflare Workers secret），不進 repo、不進 log。
  - 公鑰（32 bytes，給 hex 或 base64 都可以）交給 claude-pm，寫進 GF Sync 的 `internal/inbox/keys.go` 的 `SiteKeys`，隨新版 GF Sync 發布。
  - 換鑰匙：先發一版同時內建新舊兩把的 GF Sync，等大家都更新了，網站才改用新私鑰；再下一版拿掉舊的。私鑰外洩時同樣流程，但要盡快。
  - 測試環境（DEV／預覽）請用**另一把**金鑰，不要和正式站共用。GF Sync 只對正式站內建正式公鑰。
- **GF Sync 內建的測試金鑰**：`internal/inbox/keys.go` 的 `TestPublicKeyHex` 是 `gfsync-mock` 用的測試金鑰，種子公開在原始碼裡，誰都能用它簽。GF Sync 只在連到本機（`http://127.0.0.1` 等）時才接受它，**網站絕對不能用這把**。

## 2. API（建議）

```
GET /api/sync/inbox
Authorization: Bearer <裝置 token>        （和 upload 相同的裝置授權）
If-None-Match: "<上次的 ETag>"            （選填）
```

- **200**：`Content-Type: application/json`，body 是

  ```json
  { "inbox": "GFIN1~<sum>~<sig>~H;..." }
  ```

  加上 `ETag` 標頭。`inbox` 是**這台裝置所屬公會**的整份清單（草案 §3、§7.2），GF Sync 原樣寫進 `Inbox.lua`。JSON 整體請小於 80 KB。
- **304**：內容和 `If-None-Match` 一樣。GF Sync 不改檔案。
- **404**：網站還沒有這個功能。GF Sync 只在 log 記一次，之後照常每 10 分鐘問一次。
- **401／403**：token 失效，和 upload 一樣處理。
- **其他錯誤**：GF Sync 不改檔案，下一輪再試。body 不是 JSON 錯誤時，GF Sync 會在 log 記 `CF-Ray`（每小時一次），方便查。
- **沒有網站單**時回 200 和只有表頭的清單（`n=0`），**不要回 404**：工具箱要能分辨「清單是空的」和「沒有清單」。
- 頻率：GF Sync 每台電腦每 10 分鐘一次（第一次在啟動後 30 秒），有 ETag 時大多是 304。

## 3. GF Sync 寫入的保證（給網站參考）

- 只寫 `Interface/AddOns/GreenFangToolkit_Inbox/GreenFangToolkit_Inbox.toc`（固定內容）和 `Inbox.lua`（一行 `GreenFangToolkit_Inbox = "<字串>"`）。路徑是程式裡的常數，**網站送來的任何內容都不會變成路徑**。
- 先寫暫存檔（同一個資料夾裡的 `<檔名>.gfsync-tmp`），讀回比對、改名，再讀回比對。失敗時舊檔案保持原樣，暫存檔刪掉。
- `_forever_`、`Interface`、`AddOns`、`GreenFangToolkit_Inbox` 任一層是連結或 junction 就不寫。絕不碰 `WTF/`。
- 遊戲開著時改名可能失敗（檔案被開著），GF Sync 下一輪再試；遊戲要等下次 `/reload` 或登入才讀得到新內容。
- log 只記張數，不記清單內容（裡面有 GUID 和公會名稱）。
