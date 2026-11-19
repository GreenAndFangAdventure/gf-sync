; 綠皮同步（GF Sync）安裝檔 — Inno Setup 6
; 由 sync/build.ps1 編譯：ISCC /DAppVersion=0.1.0 installer\gfsync.iss
; 安裝到使用者資料夾，不需要系統管理員權限，也不會碰魔獸世界的任何資料夾。

#ifndef AppVersion
  #define AppVersion "0.0.0-dev"
#endif
#ifndef AppVersionNum
  #define AppVersionNum "0.0.0.0"
#endif
#define AppPublisher "綠皮與獠牙冒險事務所"
#define AppName "綠皮同步（GF Sync）"
#define AppExe "GFSync.exe"
; 版本資訊的產品名稱只用 "GF Sync"；介面上保留中文名稱。
#define ProductName "GF Sync"

[Setup]
AppId={{6E0C4E7B-2B57-4C1E-9F0A-6A1D7C3B9F21}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL=https://greenandfangadventure.com
AppSupportURL=https://greenandfangadventure.com
DefaultDirName={localappdata}\Programs\GFSync
DisableDirPage=yes
DisableProgramGroupPage=yes
DisableReadyPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..\dist
OutputBaseFilename=GFSync-Setup-{#AppVersion}
SetupIconFile=..\dist\gfsync.ico
UninstallDisplayIcon={app}\{#AppExe}
UninstallDisplayName={#AppName}
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
CloseApplications=yes
RestartApplications=no
ShowLanguageDialog=no
; 安裝過程中顯示隱私權政策（build.ps1 由 PRIVACY.md 產生 UTF-8 BOM 的 PRIVACY.txt）。
InfoBeforeFile=..\dist\PRIVACY.txt
; 安裝檔本身的版本資訊（檔案內容 → 詳細資料）
VersionInfoVersion={#AppVersionNum}
VersionInfoTextVersion={#AppVersion}
VersionInfoProductVersion={#AppVersionNum}
VersionInfoProductTextVersion={#AppVersion}
VersionInfoCompany={#AppPublisher}
VersionInfoProductName={#ProductName}
VersionInfoDescription={#ProductName} Setup
VersionInfoCopyright=© 2026 {#AppPublisher}
VersionInfoOriginalFileName=GFSync-Setup-{#AppVersion}.exe

[Languages]
Name: "default"; MessagesFile: "compiler:Default.isl"

[Messages]
SetupAppTitle=安裝
SetupWindowTitle=安裝 - %1
UninstallAppTitle=解除安裝
UninstallAppFullTitle=解除安裝 %1
ConfirmTitle=確認
ErrorTitle=錯誤
ExitSetupTitle=結束安裝
ExitSetupMessage=安裝還沒有完成，現在結束就不會安裝。%n%n要結束安裝嗎？
ButtonBack=< 上一步(&B)
ButtonNext=下一步(&N) >
ButtonInstall=安裝(&I)
ButtonOK=確定
ButtonCancel=取消
ButtonYes=是(&Y)
ButtonNo=否(&N)
ButtonFinish=完成(&F)
WizardInfoBefore=隱私權政策
InfoBeforeLabel=請先閱讀 GF Sync 會上傳哪些資料，以及如何關閉或撤銷。
InfoBeforeClickLabel=閱讀完畢後按「下一步」繼續安裝。之後也可以在工作列選單的「隱私權政策」再看一次。
WizardSelectTasks=選擇附加工作
SelectTasksDesc=要一起設定哪些項目？
SelectTasksLabel2=請選擇安裝 [name] 時要一起設定的項目，然後按「安裝」。
WizardPreparing=準備安裝
PreparingDesc=正在準備在你的電腦上安裝 [name]。
ApplicationsFound=下列程式正在使用要更新的檔案，建議讓安裝程式自動關閉它們。
ApplicationsFound2=下列程式正在使用要更新的檔案，建議讓安裝程式自動關閉它們，安裝完成後會再嘗試啟動。
CloseApplications=自動關閉這些程式(&A)
DontCloseApplications=不要關閉(&D)
WizardInstalling=安裝中
InstallingLabel=正在安裝 [name]，請稍候。
StatusExtractFiles=正在解壓縮檔案…
StatusCreateRegistryEntries=正在寫入設定…
StatusCreateIcons=正在建立捷徑…
StatusRunProgram=正在完成安裝…
StatusRollback=正在復原變更…
FinishedHeadingLabel=[name] 安裝完成
FinishedLabelNoIcons=[name] 已經安裝好了。GF Sync 會常駐在右下角工作列（圖示可能收在「^」裡）。%n%n第一次啟動時會自動開啟公會網站，請用 Discord 登入並按「允許這台電腦同步」。
FinishedLabel=[name] 已經安裝好了。GF Sync 會常駐在右下角工作列（圖示可能收在「^」裡）。%n%n第一次啟動時會自動開啟公會網站，請用 Discord 登入並按「允許這台電腦同步」。%n%n之後也可以從「開始」選單（或桌面捷徑，如果有勾選）開啟。
ClickFinish=按「完成」結束安裝。
ConfirmUninstall=確定要移除 %1 嗎？這台電腦的配對資料也會一起刪除。
UninstallStatusLabel=正在移除 %1，請稍候。
WizardUninstalling=解除安裝
StatusUninstalling=正在移除 %1…
UninstalledAll=%1 已經從你的電腦移除。%n%n如果不再使用，也可以到公會網站的「同步裝置」頁面撤銷這台電腦。

[Tasks]
Name: "autostart"; Description: "開機時自動啟動 GF Sync（建議）"
; 桌面捷徑：預設不勾選。重新安裝時 Inno Setup 會記得上次的選擇。
Name: "desktopicon"; Description: "在桌面建立捷徑"; Flags: unchecked

[Files]
Source: "..\dist\{#AppExe}"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\PRIVACY.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\LICENSE"; DestDir: "{app}"; DestName: "LICENSE.txt"; Flags: ignoreversion
Source: "..\THIRD_PARTY_LICENSES.md"; DestDir: "{app}"; DestName: "THIRD_PARTY_LICENSES.txt"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#AppName}"; Filename: "{app}\{#AppExe}"
Name: "{autoprograms}\解除安裝 {#AppName}"; Filename: "{uninstallexe}"
; 開機自動啟動：放在目前使用者的「啟動」資料夾，不寫登錄檔 Run 機碼（減少防毒誤判）。
; 使用者可以在「工作管理員 → 啟動應用程式」或直接刪除捷徑來關閉。
Name: "{userstartup}\{#AppName}"; Filename: "{app}\{#AppExe}"; Tasks: autostart
; 桌面捷徑（目前使用者的桌面）。解除安裝時 Inno Setup 會一起移除。
Name: "{userdesktop}\{#AppName}"; Filename: "{app}\{#AppExe}"; Tasks: desktopicon

[InstallDelete]
; 重新安裝時取消勾選「開機自動啟動」，就移除之前放的捷徑。
Type: files; Name: "{userstartup}\{#AppName}.lnk"; Tasks: not autostart
; 重新安裝時取消勾選「在桌面建立捷徑」，就移除之前放的桌面捷徑。
Type: files; Name: "{userdesktop}\{#AppName}.lnk"; Tasks: not desktopicon

[Run]
Filename: "{app}\{#AppExe}"; Description: "現在啟動 GF Sync"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM {#AppExe}"; Flags: runhidden; RunOnceId: "StopGFSync"

[UninstallDelete]
; GF Sync 自己的資料（配對 token、已上傳紀錄、記錄檔）。不碰魔獸世界的資料夾。
Type: filesandordirs; Name: "{userappdata}\GFSync"
