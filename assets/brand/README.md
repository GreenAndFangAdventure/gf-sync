# 公會徽章（Guild emblem）

`guild-emblem-source.png`（485×397，透明背景）是「綠皮與獠牙冒險事務所」的徽章：木框圓盤、深綠色盾旗，以及黃色的獠牙鬼臉。

## 授權：保留所有權利，不在 MIT 授權範圍內

- 本 repo 的程式碼以 MIT 授權發布，但**這張徽章不是**。
- 徽章，以及由它產生的所有圖示（GFSync.exe 與安裝檔的圖示、工作列圖示、`cmd/mkres -preview` 輸出的預覽圖），都是「綠皮與獠牙冒險事務所」的標誌，**保留所有權利**。
- 允許的使用：建置與發布本專案（GF Sync）時使用，以及在介紹 GF Sync 時展示。
- 其他用途請先取得公會同意。如果你 fork 本專案做成別的工具，請換成自己的圖示：把 `guild-emblem-source.png` 換成你的圖，並調整 `internal/icon` 中臉部顏色的判斷。

同一個資料夾裡的 `brand.go` 只是把圖檔嵌進程式的幾行程式碼，屬於 MIT 授權。

## 圖示是怎麼產生的（可以重現）

所有圖示都是從這張 PNG 用程式算出來的，repo 裡沒有放產生出來的圖示檔，CI 每次建置都會重新產生：

| 用途 | 尺寸 | 內容 | 產生的程式 |
| --- | --- | --- | --- |
| 工作列圖示（正常、暫停、錯誤、配對中） | 16、20、24、32、40、48 | 簡化版：程式畫的深綠盾，加上從徽章取出的黃色獠牙臉；狀態點（灰、紅、琥珀）在右下角 | `internal/icon`（程式啟動時產生） |
| GFSync.exe 的圖示、捷徑、解除安裝的顯示圖示 | 16、24、32 | 簡化版（同上，沒有狀態點） | `cmd/mkres` → `.syso` |
| 同上 | 48、64、256 | 完整徽章（裁掉透明邊、置中成正方形後縮放） | `cmd/mkres` → `.syso` |
| 安裝檔圖示（`SetupIconFile`） | 同 exe | 同 exe | `cmd/mkres -ico dist\gfsync.ico` |

預覽所有尺寸（淺色和深色工作列各一份，小圖另外放大 4 倍）：

```powershell
go run ./cmd/mkres -preview dist\icon-preview.png
```

---

## English

`guild-emblem-source.png` is the emblem of the guild 綠皮與獠牙冒險事務所. **It is not covered by the MIT license of this repository.** The emblem and every icon generated from it (exe, installer and tray icons) are the guild's mark, all rights reserved. You may use them only to build and distribute GF Sync itself; if you fork the project for something else, replace the emblem with your own artwork. `brand.go` (the few lines that embed the file) is MIT licensed. All icons are generated from this PNG at build or run time by `internal/icon` and `cmd/mkres`; run `go run ./cmd/mkres -preview dist/icon-preview.png` to preview them.
