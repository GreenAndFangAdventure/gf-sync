# GF Sync 隱私權政策（Privacy policy）

最後更新：2026-11-20

GF Sync（綠皮同步）是「綠皮與獠牙冒險事務所」公會的開源工具。它會把遊戲插件「綠皮冒險工具箱」存下來的角色與公會名冊資料，上傳到公會網站 **greenandfangadventure.com**，讓網站的名冊自動更新。

## 會上傳什麼

只有插件放在 `sync.outbox` 裡的資料（格式詳見 [`docs/FORMAT.md`](docs/FORMAT.md)）：

| 類別 | 欄位 |
| --- | --- |
| 遊戲資訊 | 世界（永恆）、遊戲版本、伺服器、語系 |
| 上傳者（你登入的角色） | 角色 GUID、角色名稱（包含公會名冊上的完整名稱「名字 姓氏」）、伺服器、陣營、公會、公會階級 |
| 角色本人資料（`character.self`） | 等級、職業、種族、專業技能與等級、配方清單（配方 ID，開過專業視窗才有）、資料擷取時間 |
| 自己角色的所在地 | 寫入存檔那一刻（通常是登出時）所在的區域和子區域名稱，只有你自己的角色，不含座標 |
| 其他成員分享給你的資料（`member.report`） | 公會裡也裝了綠皮冒險工具箱的成員，在遊戲內分享給你的**他們自己**的資料：角色名稱（包含完整名稱）、伺服器、GUID、等級、職業、專業與技能點、分享時間，以及所在的區域和子區域名稱 |
| 公會成員轉傳的資料（`member.share`） | 公會成員在遊戲內分享、再由其他成員轉傳給你的資料：成員的角色名稱（包含完整名稱）、伺服器、GUID、等級、職業、專業與技能點、配方清單、所在的區域和子區域名稱，每一項都附原作者記錄的時間；以及「放棄專業」的時間（舊版工具箱也可能帶「關閉同步」或「關掉分享所在地」的時間） |
| 製作服務與訂單（`crafting`，綠皮冒險工具箱 0.11.0 起；你自己的，以及其他成員分享的） | 是否接單、工錢報價（銅幣）；訂單編號、客人和工匠的角色 GUID、配方與物品 ID、數量、客人出的材料（物品 ID 和數量）、工錢與估價、訂單狀態和時間。**不含**名字、備註或其他文字 |
| 冒險單與職工單（`adventure`，綠皮冒險工具箱 0.12.0 起；你自己的，以及其他成員分享的） | 團長開的冒險單：地城代碼、任務 ID、時間、人數、標籤、狀態，團長對每位職工（角色 GUID）的決定；職工單：想去的地城、任務、職責、時間；對冒險單的申請、撤回、回應。**冒險單的短備註（最多 40 字）會原樣上傳，這是 GF Sync 唯一會上傳的自由文字**，只給公會成員看；請不要在備註裡寫個人資料。不含名字或其他文字 |
| 配方名稱（`recipe.names`） | 配方 ID 和遊戲裡顯示的配方名稱對照，讓網站能顯示配方名稱；不含任何人的資料 |
| 公會名冊（`roster.snapshot`） | 名冊總人數；每位成員的角色名稱、GUID、等級、職業、種族、公會階級、專業 |
| 配對時 | 電腦名稱（顯示在配對頁和「同步裝置」頁，讓你認得是哪台電腦）、GF Sync 版本 |

**也會上傳其他公會成員在遊戲內分享給你的等級、專業、配方與所在地，包括經過其他成員轉傳的。**

**裝了綠皮冒險工具箱，就代表同意分享。** 你的等級、職業、專業、配方與所在地會自動分享給公會裡有裝工具箱的人，由他們轉傳給其他成員，並可能由他們的 GF Sync 上傳到公會網站。工具箱裡沒有關閉分享或關閉所在地的開關；不想分享的話，請不要安裝工具箱。

**不收集：** 公會名冊裡其他成員的所在地、任何人的在線狀態、座標、聊天內容、帳號密碼、Battle.net 帳號資訊、電腦上的其他檔案。就算插件的某個版本寫入這些資料，GF Sync 也會在上傳前移除；所在地只保留區域和子區域兩個名稱，成員分享和轉傳的資料也只保留上面列出的欄位（GF Sync 依固定的欄位清單重建，清單以外的內容一律不送）。

記錄檔不會記錄所在地。

GF Sync 只讀取 `WTF\Account\*\SavedVariables\GreenFangToolkit.lua`，不會修改任何遊戲檔案。

GF Sync 只上傳「綠皮與獠牙冒險事務所」成員的資料：存檔裡如果有其他公會的角色，或沒有公會的項目（例如舊版插件留下的），GF Sync 會直接略過，不會送出。記錄檔只記略過的筆數，不記公會或角色名稱。

## 資料存在哪裡

- **公會網站**：上傳的資料存放在公會網站（greenandfangadventure.com，架設於 Cloudflare）的資料庫，只用來顯示與維護公會名冊。資料會和你配對時登入的 Discord 帳號關聯，記錄「這個 Discord 使用者回報了這個角色」，但不會因此給予任何權限。網站和 Cloudflare 一般都會記錄連線的 IP 位址與時間。
- **你的電腦**（`%APPDATA%\GFSync`）：
  - `token.dat`：裝置 token，用 Windows DPAPI 加密，只有你的 Windows 帳號能解開。網站只保存 token 的 SHA-256 雜湊。
  - `state.json`：已上傳項目的 id（依網站和配對的裝置分開記錄；換網站或重新配對後會重新送出，網站會回「重複」不會重複寫入）、遊戲資料夾位置、是否暫停。
  - `gfsync.log`：記錄檔（遊戲資料夾路徑、上傳結果），不會記錄 token 或配對碼。魔獸世界的帳號資料夾名稱會遮罩成「前 2 個字元 + `******` + `~` + 4 碼雜湊」（例如 `SW******~3f9a`；4 個字元以下的名稱不留任何字元），角色 GUID 只留最後 4 碼（例如 `Player-…EF01`），也不會記錄其他成員分享的名稱與所在地，所以可以放心把記錄檔貼給別人求助。超過 1MB 會換新檔。
  - 這些檔案只留在你的電腦上，不會被上傳。

## 如何停止上傳

停止上傳的方法是暫停或解除安裝 GF Sync。綠皮冒險工具箱在遊戲內沒有開關。

- **GF Sync**：在工作列的公會盾牌圖示上按右鍵，勾選「暫停同步」。暫停狀態重新開機後仍然有效。
- **撤銷這台電腦**：到 <https://greenandfangadventure.com/pages/sync-devices> 撤銷裝置，token 會立刻失效。
- **解除安裝**：工作列選單的「解除安裝 GF Sync…」、「開始」選單的「解除安裝 綠皮同步（GF Sync）」，或 Windows「設定 → 應用程式」。解除安裝時會刪除 `%APPDATA%\GFSync`（含 token）。

暫停或解除安裝 GF Sync，只會停止**這台電腦**上傳。你在遊戲內分享出去的資料，仍然可能由其他成員的 GF Sync 上傳；要停止遊戲內分享，請停用或移除綠皮冒險工具箱。舊版工具箱如果關閉過同步（`sync.enabled = false`），GF Sync 仍然會遵守，不上傳那個帳號的檔案。

已經上傳到網站的資料不會因為暫停或解除安裝而刪除。如果要刪除，請在公會 Discord 聯絡幹部。

## 檢查更新

GF Sync 每天會檢查一次有沒有新版本（工作列選單的「檢查更新」也會）：向公會網站拿版本清單，再向 GitHub（`github.com`）拿同一版的雜湊清單來核對；網站沒有清單時直接向 GitHub 拿。這些請求**不帶任何遊戲資料、帳號或配對 token**，只有 GF Sync 的版本號（在 User-Agent 裡）。GitHub 和公會網站一樣會看到你的 IP 位址。GF Sync 目前只會通知有新版，不會自己下載或安裝。

## 原始碼

GF Sync 以 MIT 授權開源，所有行為都可以從原始碼檢查：<https://github.com/GreenAndFangAdventure/gf-sync>

---

## English summary

GF Sync uploads character and guild-roster data that the in-game AddOn "GreenFang Toolkit" saved to `sync.outbox` to the guild website greenandfangadventure.com (hosted on Cloudflare). Uploaded fields: game world/realm/locale; the uploading character's GUID, name (including the full "given name + family name" shown in the guild roster), realm, faction, guild and rank; level, class, race and professions; for roster snapshots, each member's name, GUID, level, class, race, rank and professions; and at pairing time the computer name and GF Sync version. Your own character's location (zone and sub-zone names at the moment the AddOn saved, usually at logout; no coordinates) is uploaded with character data. It also uploads crafting services and orders (AddOn 0.11.0 and later, your own and those other members shared): whether you take orders, fee quotes in copper, and per order the order ID, the customer's and crafter's character GUIDs, recipe and item IDs, quantities, materials (item IDs and counts), fees, status and times, with no names, notes or other text. It also uploads adventure tickets and worker tickets (AddOn 0.12.0 and later, your own and those other members shared): dungeon codes, quest IDs, times, group size, tags, status, the leader's decision per worker (character GUID), the worker's wanted dungeons, quests, roles and times, and applications and replies. A ticket's short note (at most 40 characters, control characters removed) is uploaded as written and is the only free text GF Sync ever uploads; it is shown only to guild members. It also uploads your own recipe list, recipe ID-to-name tables, and the name (including the full name), level, class, professions, recipes and zone that other guild members shared in game, including data relayed through other members, each with the time the original author recorded it and the time a profession was dropped. Installing the AddOn means agreeing to share: your level, class, professions, recipes and zone are shared with and relayed by guild members who use the AddOn and may be uploaded by their GF Sync. The AddOn has no switch to turn sharing off; if you do not want to share, do not install it. GF Sync rebuilds relayed data from a fixed list of fields and sends nothing else. Roster members' locations and anyone's online status are never uploaded, and GF Sync strips them even if present; the log never records locations or shared members' names. GF Sync never modifies game files. Locally it keeps a DPAPI-encrypted device token, upload state and a log in `%APPDATA%\GFSync`; none of these are uploaded. The log masks WoW account folder names (first 2 characters + `******~` + a 4-digit hash) and shortens character GUIDs to their last 4 digits. To stop uploading from this computer: choose "暫停同步" (Pause) in the tray menu, revoke the device at <https://greenandfangadventure.com/pages/sync-devices>, or uninstall (which deletes `%APPDATA%\GFSync`). This does not stop the AddOn from sharing in game, so other members' GF Sync may still upload your data; disable or remove the AddOn to stop that. If an older AddOn version had sync turned off (`sync.enabled = false`), GF Sync still respects it. To delete data already uploaded, contact the guild officers on Discord.

Update checks: once a day (and from the tray menu), GF Sync fetches a version manifest from the guild website and a checksum list from GitHub (`github.com`) to verify it. These requests carry no game data, account or token, only the GF Sync version in the User-Agent; GitHub sees your IP address. GF Sync currently only notifies you; it does not download or install anything by itself.
