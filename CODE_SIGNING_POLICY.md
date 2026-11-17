# Code signing policy

Free code signing provided by [SignPath.io](https://about.signpath.io/), certificate by [SignPath Foundation](https://signpath.org/).

> 狀態：申請中。在 SignPath Foundation 核准之前，GitHub Releases 上的檔案都**沒有簽章**。

## 簽章的對象

只會簽署本 repo 的 GitHub Actions（`.github/workflows/build.yml`）從公開原始碼建置出來的檔案：

- `GFSync.exe`
- `GFSync-Setup-<版本>.exe`

不會簽署在個人電腦上建置的檔案，也不會簽署第三方程式。每次發布都要由下列 Approver 人工核准後才會簽章。

## 團隊角色

| 角色 | 成員 | 職責 |
| --- | --- | --- |
| Authors（作者） | [綠皮與獠牙冒險企業](https://github.com/GreenAndFangAdventure) GitHub 組織擁有者（[名單](https://github.com/orgs/GreenAndFangAdventure/people)） | 撰寫與提交程式碼 |
| Reviewers（審查者） | [綠皮與獠牙冒險企業](https://github.com/GreenAndFangAdventure) GitHub 組織擁有者（[名單](https://github.com/orgs/GreenAndFangAdventure/people)） | 審查每一個合併進 `main` 的變更 |
| Approvers（核准者） | [綠皮與獠牙冒險企業](https://github.com/GreenAndFangAdventure) GitHub 組織擁有者（[名單](https://github.com/orgs/GreenAndFangAdventure/people)） | 核准每一次簽章請求 |

GitHub 組織的所有成員都必須開啟兩步驟驗證（MFA）。

## 隱私權

GF Sync 會把資料傳送到公會網站，詳見 [隱私權政策（PRIVACY.md）](PRIVACY.md)。安裝過程中會顯示隱私權政策；使用者可以隨時暫停同步或解除安裝。

---

## English

Free code signing provided by [SignPath.io](https://about.signpath.io/), certificate by [SignPath Foundation](https://signpath.org/).

Only binaries built by this repository's GitHub Actions workflow from the public source are signed, and every signing request is manually approved. Team roles — Authors, Reviewers and Approvers — are currently held by the owner of the GreenAndFangAdventure GitHub organization. All organization members use multi-factor authentication. GF Sync sends data to the guild website; see the [privacy policy](PRIVACY.md), which is also shown during installation.
