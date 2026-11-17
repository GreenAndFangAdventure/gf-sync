# Build GF Sync: run tests, build GFSync.exe, compile the Inno Setup installer,
# and write SHA-256 checksums. Output goes to dist\ (not committed).
#
#   powershell -ExecutionPolicy Bypass -File build.ps1 -Version 0.1.1
#   powershell -ExecutionPolicy Bypass -File build.ps1 -SkipInstaller      # exe only
#   powershell -ExecutionPolicy Bypass -File build.ps1 -SkipBuild          # installer from the existing dist\GFSync.exe
#
# CI (.github/workflows/build.yml) runs -SkipInstaller, signs the exe, then
# -SkipBuild, so the installer packages the signed exe.
param(
    [string]$Version = "0.1.1",
    [switch]$SkipTests,
    [switch]$SkipInstaller,
    [switch]$SkipBuild
)
$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
$dist = Join-Path $root "dist"
$exe = Join-Path $dist "GFSync.exe"

function Find-Tool($name, $candidates) {
    $cmd = Get-Command $name -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    foreach ($c in $candidates) { if (Test-Path $c) { return $c } }
    return $null
}

$go = Find-Tool "go" @("$env:ProgramFiles\Go\bin\go.exe", "$env:LOCALAPPDATA\Programs\Go\bin\go.exe")
if (-not $go) { throw "Go not found. Install with: winget install --id GoLang.Go -e" }

Push-Location $root
try {
    New-Item -ItemType Directory -Force $dist | Out-Null

    # Version info + asInvoker manifest + icon, linked in by go build
    # (cmd/gfsync/rsrc_windows_amd64.syso is generated, not committed).
    # Also writes the installer icon.
    $syso = Join-Path $root "cmd\gfsync\rsrc_windows_amd64.syso"
    & $go run ./cmd/mkres -version $Version -syso $syso -ico (Join-Path $dist "gfsync.ico")
    if ($LASTEXITCODE -ne 0) { throw "mkres failed" }

    if (-not $SkipBuild) {
        if (-not $SkipTests) {
            & $go test ./...
            if ($LASTEXITCODE -ne 0) { throw "go test failed" }
        }
        $env:CGO_ENABLED = "0"
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        # Symbols are kept on purpose (no -s -w): stripped, unsigned Go binaries
        # are flagged by antivirus heuristics more often.
        & $go build -trimpath -ldflags "-H windowsgui -X main.version=$Version" -o $exe ./cmd/gfsync
        if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    } elseif (-not (Test-Path $exe)) {
        throw "-SkipBuild needs an existing $exe"
    }

    $outputs = @($exe)
    if (-not $SkipInstaller) {
        # The installer shows the privacy policy (InfoBeforeFile). Inno Setup
        # reads .txt as UTF-8 only with a BOM, so copy PRIVACY.md with one.
        $privacy = Get-Content -Raw -Encoding UTF8 (Join-Path $root "PRIVACY.md")
        [IO.File]::WriteAllText((Join-Path $dist "PRIVACY.txt"), $privacy, (New-Object Text.UTF8Encoding $true))

        $iscc = Find-Tool "ISCC" @(
            "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe",
            "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
            "$env:ProgramFiles\Inno Setup 6\ISCC.exe")
        if (-not $iscc) { throw "Inno Setup not found. Install with: winget install --id JRSoftware.InnoSetup -e" }
        # Numeric x.y.z.w for VersionInfoVersion ("0.1.0-beta1" -> "0.1.0.0").
        $parts = @((($Version -split '[-+]')[0] -split '\.') + @('0', '0', '0', '0'))[0..3]
        $versionNum = $parts -join '.'
        & $iscc /Q "/DAppVersion=$Version" "/DAppVersionNum=$versionNum" (Join-Path $root "installer\gfsync.iss")
        if ($LASTEXITCODE -ne 0) { throw "ISCC failed" }
        $outputs += (Join-Path $dist "GFSync-Setup-$Version.exe")
    }

    $lines = foreach ($f in $outputs) {
        $h = (Get-FileHash -Algorithm SHA256 $f).Hash.ToLower()
        "$h  $(Split-Path $f -Leaf)"
    }
    $sums = Join-Path $dist "SHA256SUMS.txt"
    [IO.File]::WriteAllLines($sums, $lines)
    Write-Host "Built:"
    foreach ($f in $outputs) { Write-Host ("  {0}  ({1:N1} MB)" -f $f, ((Get-Item $f).Length / 1MB)) }
    Write-Host "SHA-256 ($sums):"
    $lines | ForEach-Object { Write-Host "  $_" }
} finally {
    Pop-Location
}
