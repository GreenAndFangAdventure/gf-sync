# GF Sync 隱私權政策（Privacy policy）

最後更新：2026-11-17

GF Sync（綠皮同步）是「綠皮與獠牙冒險事務所」公會的開源工具。它會把遊戲插件「綠皮冒險工具箱」存下來的角色與公會名冊資料，上傳到公會網站 **greenandfangadventure.com**，讓網站的名冊自動更新。

## 會上傳什麼

只有插件放在 `sync.outbox` 裡、你在遊戲中沒有關閉同步的資料（格式詳見 [`docs/FORMAT.md`](docs/FORMAT.md)）：

| 類別 | 欄位 |
| --- | --- |
| 遊戲資訊 | 世界（永恆）、遊戲版本、伺服器、語系 |
| 上傳者（你登入的角色） | 角色 GUID、角色名稱、伺服器、陣營、公會、公會階級 |
| 角色本人資料（`character.self`） | 等級、職業、種族、專業技能與等級、資料擷取時間 |
| 公會名冊（`roster.snapshot`） | 名冊總人數；每位成員的角色名稱、GUID、等級、職業、種族、公會階級、專業 |
| 配對時 | 電腦名稱（顯示在配對頁和「同步裝置」頁，讓你認得是哪台電腦）、GF Sync 版本 |

**不收集：** 所在地（區域）、在線狀態、聊天內容、帳號密碼、Battle.net 帳號資訊、電腦上的其他檔案。就算插件的某個版本寫入所在地或在線狀態，GF Sync 也會在上傳前移除。

GF Sync 只讀取 `WTF\Account\*\SavedVariables\GreenFangToolkit.lua`，不會修改任何遊戲檔案。

## 資料存在哪裡

- **公會網站**：上傳的資料存放在公會網站（greenandfangadventure.com，架設於 Cloudflare）的資料庫，只用來顯示與維護公會名冊。資料會和你配對時登入的 Discord 帳號關聯，記錄「這個 Discord 使用者回報了這個角色」，但不會因此給予任何權限。網站和 Cloudflare 一般都會記錄連線的 IP 位址與時間。
- **你的電腦**（`%APPDATA%\GFSync`）：
  - `token.dat`：裝置 token，用 Windows DPAPI 加密，只有你的 Windows 帳號能解開。網站只保存 token 的 SHA-256 雜湊。
  - `state.json`：已上傳項目的 id、遊戲資料夾位置、是否暫停。
  - `gfsync.log`：記錄檔（遊戲資料夾路徑、帳號資料夾名稱、上傳結果），不會記錄 token。超過 1MB 會換新檔。
  - 這些檔案只留在你的電腦上，不會被上傳。

## 如何暫停、關閉或撤銷

- **遊戲內**：輸入 `/gf sync off`，插件就會標記不同步，GF Sync 不會上傳這個帳號的任何資料。
- **GF Sync**：在工作列綠色圖示上按右鍵，勾選「暫停同步」。暫停狀態重新開機後仍然有效。
- **撤銷這台電腦**：到 <https://greenandfangadventure.com/pages/sync-devices> 撤銷裝置，token 會立刻失效。
- **解除安裝**：工作列選單的「解除安裝 GF Sync…」、「開始」選單的「解除安裝 綠皮同步（GF Sync）」，或 Windows「設定 → 應用程式」。解除安裝時會刪除 `%APPDATA%\GFSync`（含 token）。

已經上傳到網站的資料不會因為暫停或解除安裝而刪除。如果要刪除，請在公會 Discord 聯絡幹部。

## 原始碼

GF Sync 以 MIT 授權開源，所有行為都可以從原始碼檢查：<https://github.com/GreenAndFangAdventure/gf-sync>

---

## English summary

GF Sync uploads character and guild-roster data that the in-game AddOn "GreenFang Toolkit" saved to `sync.outbox` to the guild website greenandfangadventure.com (hosted on Cloudflare). Uploaded fields: game world/realm/locale; the uploading character's GUID, name, realm, faction, guild and rank; level, class, race and professions; for roster snapshots, each member's name, GUID, level, class, race, rank and professions; and at pairing time the computer name and GF Sync version. **Location (zone) and online status are never uploaded**, and GF Sync strips them even if present. GF Sync never modifies game files. Locally it keeps a DPAPI-encrypted device token, upload state and a log in `%APPDATA%\GFSync`; none of these are uploaded. To stop syncing: type `/gf sync off` in game, choose "暫停同步" (Pause) in the tray menu, revoke the device at <https://greenandfangadventure.com/pages/sync-devices>, or uninstall (which deletes `%APPDATA%\GFSync`). To delete data already uploaded, contact the guild officers on Discord.
