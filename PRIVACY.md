# GF Sync 隱私權政策（Privacy policy）

最後更新：2026-11-20

GF Sync（綠皮同步）是「綠皮與獠牙冒險事務所」公會的開源工具。它會把遊戲插件「綠皮冒險工具箱」存下來的角色與公會名冊資料，上傳到公會網站 **greenandfangadventure.com**，讓網站的名冊自動更新。

## 會上傳什麼

只有插件放在 `sync.outbox` 裡、你在遊戲中沒有關閉同步的資料（格式詳見 [`docs/FORMAT.md`](docs/FORMAT.md)）：

| 類別 | 欄位 |
| --- | --- |
| 遊戲資訊 | 世界（永恆）、遊戲版本、伺服器、語系 |
| 上傳者（你登入的角色） | 角色 GUID、角色名稱、伺服器、陣營、公會、公會階級 |
| 角色本人資料（`character.self`） | 等級、職業、種族、專業技能與等級、配方清單（配方 ID，開過專業視窗才有）、資料擷取時間 |
| 自己角色的所在地（遊戲內可以關閉，預設開啟） | 寫入存檔那一刻（通常是登出時）所在的區域和子區域名稱，只有你自己的角色，不含座標 |
| 其他成員分享給你的資料（`member.report`） | 公會裡也裝了綠皮冒險工具箱的成員，在遊戲內分享給你的**他們自己**的資料：角色名稱、伺服器、GUID、等級、職業、專業與技能點、分享時間；對方有開「分享所在地」時，還有所在的區域和子區域名稱 |
| 公會成員轉傳的資料（`member.share`） | 公會成員在遊戲內分享、再由其他成員轉傳給你的資料：成員的角色名稱、伺服器、GUID、等級、職業、專業與技能點、配方清單、所在的區域和子區域名稱（原作者有開「分享所在地」時），每一項都附原作者記錄的時間；以及「撤回」的時間（原作者關閉同步、關掉分享所在地或放棄專業時） |
| 配方名稱（`recipe.names`） | 配方 ID 和遊戲裡顯示的配方名稱對照，讓網站能顯示配方名稱；不含任何人的資料 |
| 公會名冊（`roster.snapshot`） | 名冊總人數；每位成員的角色名稱、GUID、等級、職業、種族、公會階級、專業 |
| 配對時 | 電腦名稱（顯示在配對頁和「同步裝置」頁，讓你認得是哪台電腦）、GF Sync 版本 |

**也會上傳其他公會成員在遊戲內分享給你的等級、專業、配方與所在地，包括經過其他成員轉傳的（他們可以在遊戲內關閉分享）。** 同樣地，你的等級、專業、配方與所在地會分享給公會裡有裝工具箱的人，由他們轉傳給其他成員，並可能由他們的 GF Sync 上傳到公會網站。關掉同步就完全不分享，工具箱也會通知大家撤回你的資料；關掉「分享所在地」就不分享所在地，也會請其他成員刪掉你之前的所在地。

**不收集：** 公會名冊裡其他成員的所在地、任何人的在線狀態、座標、聊天內容、帳號密碼、Battle.net 帳號資訊、電腦上的其他檔案。就算插件的某個版本寫入這些資料，GF Sync 也會在上傳前移除；所在地只保留區域和子區域兩個名稱，成員分享和轉傳的資料也只保留上面列出的欄位（GF Sync 依固定的欄位清單重建，清單以外的內容一律不送）。

**關閉所在地：** 在綠皮冒險工具箱的「設定」裡關掉分享所在地，插件就不會再寫入，GF Sync 也就不會上傳。記錄檔不會記錄所在地。

GF Sync 只讀取 `WTF\Account\*\SavedVariables\GreenFangToolkit.lua`，不會修改任何遊戲檔案。

GF Sync 只上傳「綠皮與獠牙冒險事務所」成員的資料：存檔裡如果有其他公會的角色，或沒有公會的項目（例如舊版插件留下的），GF Sync 會直接略過，不會送出。記錄檔只記略過的筆數，不記公會或角色名稱。

## 資料存在哪裡

- **公會網站**：上傳的資料存放在公會網站（greenandfangadventure.com，架設於 Cloudflare）的資料庫，只用來顯示與維護公會名冊。資料會和你配對時登入的 Discord 帳號關聯，記錄「這個 Discord 使用者回報了這個角色」，但不會因此給予任何權限。網站和 Cloudflare 一般都會記錄連線的 IP 位址與時間。
- **你的電腦**（`%APPDATA%\GFSync`）：
  - `token.dat`：裝置 token，用 Windows DPAPI 加密，只有你的 Windows 帳號能解開。網站只保存 token 的 SHA-256 雜湊。
  - `state.json`：已上傳項目的 id（依網站和配對的裝置分開記錄；換網站或重新配對後會重新送出，網站會回「重複」不會重複寫入）、遊戲資料夾位置、是否暫停。
  - `gfsync.log`：記錄檔（遊戲資料夾路徑、上傳結果），不會記錄 token 或配對碼。魔獸世界的帳號資料夾名稱會遮罩成「前 2 個字元 + `******` + `~` + 4 碼雜湊」（例如 `SW******~3f9a`；4 個字元以下的名稱不留任何字元），角色 GUID 只留最後 4 碼（例如 `Player-…EF01`），也不會記錄其他成員分享的名稱與所在地，所以可以放心把記錄檔貼給別人求助。超過 1MB 會換新檔。
  - 這些檔案只留在你的電腦上，不會被上傳。

## 如何暫停、關閉或撤銷

- **遊戲內**：在綠皮冒險工具箱的「設定」關閉同步（進階：輸入 `/gf sync off`），GF Sync 就不會上傳這個帳號的任何資料。
- **GF Sync**：在工作列的公會盾牌圖示上按右鍵，勾選「暫停同步」。暫停狀態重新開機後仍然有效。
- **撤銷這台電腦**：到 <https://greenandfangadventure.com/pages/sync-devices> 撤銷裝置，token 會立刻失效。
- **解除安裝**：工作列選單的「解除安裝 GF Sync…」、「開始」選單的「解除安裝 綠皮同步（GF Sync）」，或 Windows「設定 → 應用程式」。解除安裝時會刪除 `%APPDATA%\GFSync`（含 token）。

已經上傳到網站的資料不會因為暫停或解除安裝而刪除。如果要刪除，請在公會 Discord 聯絡幹部。

## 原始碼

GF Sync 以 MIT 授權開源，所有行為都可以從原始碼檢查：<https://github.com/GreenAndFangAdventure/gf-sync>

---

## English summary

GF Sync uploads character and guild-roster data that the in-game AddOn "GreenFang Toolkit" saved to `sync.outbox` to the guild website greenandfangadventure.com (hosted on Cloudflare). Uploaded fields: game world/realm/locale; the uploading character's GUID, name, realm, faction, guild and rank; level, class, race and professions; for roster snapshots, each member's name, GUID, level, class, race, rank and professions; and at pairing time the computer name and GF Sync version. Your own character's location (zone and sub-zone names at the moment the AddOn saved, usually at logout; no coordinates) is uploaded with character data. It is on by default and can be turned off in the AddOn's settings. It also uploads your own recipe list, recipe ID-to-name tables, and the level, class, professions, recipes and (if they allow it) zone that other guild members shared in game, including data relayed through other members, each with the time the original author recorded it and any withdrawal times; they can turn sharing off in game, and likewise your data may be shared with and relayed by guild members who use the AddOn and uploaded by them. GF Sync rebuilds relayed data from a fixed list of fields and sends nothing else. Roster members' locations and anyone's online status are never uploaded, and GF Sync strips them even if present; the log never records locations or shared members' names. GF Sync never modifies game files. Locally it keeps a DPAPI-encrypted device token, upload state and a log in `%APPDATA%\GFSync`; none of these are uploaded. The log masks WoW account folder names (first 2 characters + `******~` + a 4-digit hash) and shortens character GUIDs to their last 4 digits. To stop syncing: turn sync off in the AddOn's settings (or type `/gf sync off`) in game, choose "暫停同步" (Pause) in the tray menu, revoke the device at <https://greenandfangadventure.com/pages/sync-devices>, or uninstall (which deletes `%APPDATA%\GFSync`). To delete data already uploaded, contact the guild officers on Discord.
