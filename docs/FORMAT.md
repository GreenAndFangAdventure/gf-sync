# 資料格式（Data formats）

GF Sync 只在兩個格式之間轉換：遊戲插件「綠皮冒險工具箱」寫的 SavedVariables，以及公會網站的上傳 API。本文是公開版的摘要，給想檢查 GF Sync 做了什麼的人看。`internal/svdoc`、`internal/api` 的程式碼和測試以本文為準。

## 1. 讀取：SavedVariables

- 位置：`<魔獸世界>\_forever_\WTF\Account\<帳號資料夾>\SavedVariables\GreenFangToolkit.lua`
- GF Sync **只讀不寫**，也不會碰 `Interface\AddOns\`。
- 只解析 Lua 的表格字面值（字串、數字、布林、巢狀表格），**絕不執行 Lua 程式**。檔案有大小、深度和節點數上限。
- 檔案修改後要等 5 秒都沒有再變動才會讀取（遊戲只在登出、`/reload`、斷線時寫檔）。

```lua
GreenFangToolkitDB = {
  schema = "gf.toolkit",
  schemaVersion = 2,             -- 目前只接受 2；其他值略過並提示「請更新 GF Sync」
  addonVersion = "0.4.0",
  settings = { ... },            -- 介面設定，GF Sync 不讀
  sync = {
    enabled = true,              -- 不是 true 時，GF Sync 整個檔案都不上傳。D-015 起工具箱沒有這個開關，只有舊版關閉過同步時會是 false
    outbox = { <entry>, ... },   -- 待上傳的資料
  },
}
```

### entry

```lua
{
  id = "<掃描者 GUID>:<capturedAt>:<kind>",   -- 全域唯一，用來去重
  kind = "character.self" | "roster.snapshot" | "member.report",  -- 其他 kind 一律忽略
  capturedAt = 1763370000,                      -- Unix 秒
  game = { world = "forever", interface = 16001, realm = "...", locale = "zhTW" },
  uploader = { guid = "...", name = "...", realm = "...", faction = "Horde",
               guild = "...", guildRank = "..." },
  payload = { ... },
}
```

- `character.self`（角色本人回報）：`level`、`class`、`classToken`、`race`、`raceID`、`professions`（名稱、技能 ID、等級、上限），以及 `recipes`（`["<skillLineID>"] = { recordedAt, hash, ids }`，原樣上傳）。
- 每一種 kind 都要有 `id`、`kind`、`capturedAt`（數字）、`game`、`uploader`、`payload`。`capturedAtServer`（遊戲伺服器時間）不是數字時，GF Sync 只拿掉這個欄位。
- `member.report`（其他成員在遊戲內分享給你的**他們自己**的資料；`uploader` 是收到的本機角色，一樣只接受本公會）：`subject = { name, realm, guid }`、`reportedAt`、`level`、`classToken`、`professions`（`name`、`skillLineID`、`rank`、`max`），以及對方同意時的 `location`。`subject` 只保留 `name`、`realm`、`guid` 三個字串（每個最多 64 個字元），沒有 `name` 的項目不上傳。
- `member.share`（成員分享、可能經過轉傳的資料，每個欄位帶原作者的記錄時間）：GF Sync 依固定清單重建 payload，清單以外的鍵一律丟掉：
  - `subject`：`name`（必填）、`realm`、`guid`。
  - `level`、`classToken`、`professions`、`location`：各自只留 `value`、`recordedAt`（數字，必填）、`via`（只接受 `author`、`relay`）。`professions.value[*]` 只留 `skillLineID`、`name`、`rank`、`max`；`location.value` 只留 `zone`、`subZone`。
  - `recipes["<skillLineID>"]`：`recordedAt`（必填）、`via`、`hash`、`ids`（只留數字）。
  - `withdrawn`：`recipes["<skillLineID>"]`（放棄專業的時間）。舊版工具箱可能還會寫 `all`、`location`，GF Sync 照樣接受並上傳，由網站決定要不要處理。都是時間（數字）；`withdrawn.location` 是時間，不是位置。
- `recipe.names`：`{ locale, names = { ["<配方 ID>"] = "名稱" } }`，原樣上傳。
- `roster.snapshot`（某位成員看到的整份公會名冊）：`total`，以及每位成員的 `name`、`guid`、`level`、`classToken`、`race`、`raceID`、`guildRank`、`professions`、`professionSource`。

**所在地只出現在成員自己的資料裡。** `character.self` 和 `member.report` 的 payload 可以帶 `location = { zone = "...", subZone = "..." }`（`subZone` 可以省略）。D-015 起裝了工具箱就會分享，沒有開關。GF Sync 只保留這兩個字串欄位，每個最多 60 個字元；`zone` 不合格就整個 `location` 都不上傳，`subZone` 不合格就只拿掉 `subZone`，其他鍵一律丟掉。`member.share` 的 `location` 是 `{ value = { zone, subZone }, recordedAt, via }`，`value` 用同樣的規則。

**其他地方都不包含所在地和在線狀態。** 就算插件的某個版本寫了 `zone`、`online` 或 `location`，GF Sync 上傳前也會把它們從項目本身、payload 和每位成員的資料中移除（`roster.snapshot` 永遠不帶任何成員的所在地）。記錄檔不會記錄所在地或 `subject` 的名稱；`member.report`、`member.share` 的 id 如果不是「GUID:數字:member.<kind>」（可以再接「:GUID」）的格式，記錄檔只寫 `member.<kind>~` 加 4 碼雜湊。

未知欄位會原樣轉送，由網站決定是否使用。

## 2. 上傳：公會網站 API v1

網站：`https://greenandfangadventure.com`（只接受 HTTPS）。

### 裝置配對（只做一次）

| 端點 | 說明 |
| --- | --- |
| `POST /api/sync/device/start` | 送 `{label, clientVersion}`。`label` 是 Windows 電腦名稱，會顯示在配對頁，讓你認得是哪台電腦 |
| `GET /pages/sync-pair?code=XXXX` | 網站頁面：用 Discord 登入後按「允許這台電腦同步」 |
| `POST /api/sync/device/poll` | GF Sync 等待核准；核准後拿到裝置 token（只回傳一次） |
| `GET /pages/sync-devices` | 查看自己配對過的裝置，可以撤銷 |

### 上傳

`POST /api/sync/upload`，Header `Authorization: Bearer <token>`

```json
{ "client": { "name": "GFSync", "version": "0.1.1" },
  "items": [ <outbox entry 轉成 JSON，已移除 zone／online，只留自己的 location> ] }
```

- 單次最多 50 筆、1MB；GF Sync 自己限制每小時最多 50 次。
- 同一個 `id` 重送時伺服器回 `duplicate`，不會重複寫入。
- 網站回 `rejected`、`reason = "unknown_kind"`（網站還不認得這種 kind）時，GF Sync 不記成已回覆，24 小時後再送。其他回答都記成已回覆，不再送。
- outbox 最多 200 筆（`member.share` 最多 150 筆，每筆約 4 KB），GF Sync 每次送最多 50 筆，約 4 次送完。
- token 被撤銷（401／403）時，GF Sync 會刪除本機 token，並請你重新配對。
