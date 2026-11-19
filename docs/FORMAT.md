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
    enabled = true,              -- 玩家在工具箱「設定」關閉同步（或 /gf sync off）時是 false，GF Sync 就整個檔案都不上傳
    outbox = { <entry>, ... },   -- 待上傳的資料
  },
}
```

### entry

```lua
{
  id = "<掃描者 GUID>:<capturedAt>:<kind>",   -- 全域唯一，用來去重
  kind = "character.self" | "roster.snapshot",  -- 其他 kind 一律忽略
  capturedAt = 1763370000,                      -- Unix 秒
  game = { world = "forever", interface = 16001, realm = "...", locale = "zhTW" },
  uploader = { guid = "...", name = "...", realm = "...", faction = "Horde",
               guild = "...", guildRank = "..." },
  payload = { ... },
}
```

- `character.self`（角色本人回報）：`level`、`class`、`classToken`、`race`、`raceID`、`professions`（名稱、技能 ID、等級、上限）。
- `roster.snapshot`（某位成員看到的整份公會名冊）：`total`，以及每位成員的 `name`、`guid`、`level`、`classToken`、`race`、`raceID`、`guildRank`、`professions`、`professionSource`。

**所在地只有自己的角色。** `character.self` 的 payload 可以帶 `location = { zone = "...", subZone = "..." }`（`subZone` 可以省略；插件預設開啟，玩家可以在「設定」關掉）。GF Sync 只保留這兩個字串欄位，每個最多 60 個字元；`zone` 不合格就整個 `location` 都不上傳，`subZone` 不合格就只拿掉 `subZone`，其他鍵一律丟掉。

**其他地方都不包含所在地和在線狀態。** 就算插件的某個版本寫了 `zone`、`online` 或 `location`，GF Sync 上傳前也會把它們從項目本身、payload 和每位成員的資料中移除（`roster.snapshot` 永遠不帶任何成員的所在地）。記錄檔不會記錄所在地。

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
- token 被撤銷（401／403）時，GF Sync 會刪除本機 token，並請你重新配對。
