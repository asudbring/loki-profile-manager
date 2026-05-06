#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$Version = "",
    [string]$AssetsDir = (Get-Location).Path,
    [string]$ArchivePath = "",
    [string]$ChecksumsPath = "",
    [string]$ManifestPath = "",
    [string]$InstallScript = "",
    [string]$UninstallScript = "",
    [string]$InstallDir = "",
    [switch]$TestPathUpdate,
    [switch]$RunSymlinkSwitch,
    [switch]$RequireSymlinkSwitch
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = "Stop"

function Test-IsWindowsHost {
    return [Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT
}

function Get-LokiArch {
    $arch = $env:PROCESSOR_ARCHITEW6432
    if ([string]::IsNullOrWhiteSpace($arch)) { $arch = $env:PROCESSOR_ARCHITECTURE }
    switch -Regex ($arch) {
        '^(AMD64|x86_64)$' { return 'amd64' }
        '^(ARM64|AARCH64)$' { return 'arm64' }
        default { throw "unsupported Windows architecture: $arch" }
    }
}

function Resolve-FullPath {
    param([string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path)) { return "" }
    $expanded = [Environment]::ExpandEnvironmentVariables($Path)
    if ([IO.Path]::IsPathRooted($expanded)) { return [IO.Path]::GetFullPath($expanded) }
    return [IO.Path]::GetFullPath((Join-Path (Get-Location).Path $expanded))
}

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw $Message }
}

function Invoke-Step {
    param([string]$Name, [scriptblock]$Body)
    Write-Host "== $Name =="
    & $Body
}

function Get-UserPathEntries {
    $path = [Environment]::GetEnvironmentVariable("Path", "User")
    if ([string]::IsNullOrWhiteSpace($path)) { return @() }
    return @($path -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
}

function Test-DeveloperMode {
    $value = (Get-ItemProperty `
        -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock' `
        -Name AllowDevelopmentWithoutDevLicense `
        -ErrorAction SilentlyContinue).AllowDevelopmentWithoutDevLicense
    return $value -eq 1
}

function Test-IsAdmin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Invoke-Loki {
    param(
        [string]$Exe,
        [string[]]$Arguments,
        [switch]$AllowFailure
    )
    $output = & $Exe @Arguments 2>&1
    $code = $LASTEXITCODE
    if ($code -ne 0 -and -not $AllowFailure) {
        throw "loki $($Arguments -join ' ') failed with exit $code`n$output"
    }
    return [pscustomobject]@{ ExitCode = $code; Output = @($output) }
}

if (-not (Test-IsWindowsHost)) {
    throw "windows installer smoke must run on Windows"
}

$AssetsDir = Resolve-FullPath $AssetsDir
if ([string]::IsNullOrWhiteSpace($ArchivePath)) {
    $arch = Get-LokiArch
    $ArchivePath = Get-ChildItem -LiteralPath $AssetsDir -Filter "loki_*_windows_${arch}.zip" -File |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First 1 -ExpandProperty FullName
}
$ArchivePath = Resolve-FullPath $ArchivePath
if ([string]::IsNullOrWhiteSpace($ChecksumsPath)) { $ChecksumsPath = Join-Path $AssetsDir 'checksums.txt' }
$ChecksumsPath = Resolve-FullPath $ChecksumsPath
if ([string]::IsNullOrWhiteSpace($ManifestPath)) { $ManifestPath = Join-Path $AssetsDir 'release-manifest.json' }
$ManifestPath = Resolve-FullPath $ManifestPath
if ([string]::IsNullOrWhiteSpace($InstallScript)) { $InstallScript = Join-Path $AssetsDir 'install.ps1' }
$InstallScript = Resolve-FullPath $InstallScript
if ([string]::IsNullOrWhiteSpace($UninstallScript)) { $UninstallScript = Join-Path $AssetsDir 'uninstall.ps1' }
$UninstallScript = Resolve-FullPath $UninstallScript

$SmokeRoot = Join-Path $env:TEMP ("loki-installer-smoke-" + [guid]::NewGuid().ToString('N'))
$SmokeLocalAppData = Join-Path $SmokeRoot 'LocalAppData'
$DisposableStore = Join-Path $SmokeRoot 'store'
$SymlinkTarget = Join-Path $env:USERPROFILE 'loki-installer-smoke\symlink-probe.txt'
if ([string]::IsNullOrWhiteSpace($InstallDir)) { $InstallDir = Join-Path $SmokeRoot 'Install' }
$InstallDir = Resolve-FullPath $InstallDir

$summary = [ordered]@{
    archive = $ArchivePath
    install_script = $InstallScript
    uninstall_script = $UninstallScript
    manifest = $ManifestPath
    install_dir = $InstallDir
    smoke_root = $SmokeRoot
    developer_mode = Test-DeveloperMode
    admin = Test-IsAdmin
    installed_version = $null
    path_updated = $false
    store_smoke = $false
    symlink_switch = "not-run"
    uninstall_removed_binary = $false
    state_preserved = $false
    store_preserved = $false
}

try {
    Invoke-Step "preflight" {
        Assert-True (Test-Path -LiteralPath $ArchivePath) "archive not found: $ArchivePath"
        Assert-True (Test-Path -LiteralPath $ChecksumsPath) "checksums not found: $ChecksumsPath"
        Assert-True (Test-Path -LiteralPath $ManifestPath) "release manifest not found: $ManifestPath"
        Assert-True (Test-Path -LiteralPath $InstallScript) "install script not found: $InstallScript"
        Assert-True (Test-Path -LiteralPath $UninstallScript) "uninstall script not found: $UninstallScript"
        Remove-Item -LiteralPath $SmokeRoot -Recurse -Force -ErrorAction SilentlyContinue
        New-Item -ItemType Directory -Force -Path $SmokeLocalAppData | Out-Null
        New-Item -ItemType Directory -Force -Path (Split-Path $SymlinkTarget) | Out-Null
    }

    Invoke-Step "install" {
        $installParams = @{
            ArchivePath = $ArchivePath
            ChecksumsPath = $ChecksumsPath
            InstallDir = $InstallDir
            Force = $true
        }
        if ($Version) { $installParams.Version = $Version }
        if ($TestPathUpdate) { $installParams.AddToPath = $true } else { $installParams.NoPath = $true }
        & $InstallScript @installParams
        Assert-True (Test-Path -LiteralPath (Join-Path $InstallDir 'loki.exe')) "installed loki.exe missing"
        Assert-True (Test-Path -LiteralPath (Join-Path $InstallDir 'install-metadata.json')) "install metadata missing"
        $summary.path_updated = (Get-UserPathEntries | ForEach-Object { $_.TrimEnd('\') }) -contains $InstallDir.TrimEnd('\')
        if ($TestPathUpdate) { Assert-True $summary.path_updated "install dir missing from user PATH" }
    }

    $oldLocalAppData = $env:LOCALAPPDATA
    $env:LOCALAPPDATA = $SmokeLocalAppData
    $loki = Join-Path $InstallDir 'loki.exe'
    try {
        Invoke-Step "binary smoke" {
            $versionResult = Invoke-Loki -Exe $loki -Arguments @('--version')
            $gotVersion = ($versionResult.Output | Select-Object -First 1).ToString().Trim()
            $summary.installed_version = $gotVersion
            if ($Version) { Assert-True ($gotVersion -eq $Version) "version mismatch: got $gotVersion want $Version" }
            $doctor = Invoke-Loki -Exe $loki -Arguments @('doctor', '--json')
            $null = ($doctor.Output -join "`n") | ConvertFrom-Json
            $null = Invoke-Loki -Exe $loki -Arguments @('tui', '--help')
        }

        Invoke-Step "store smoke" {
            $null = Invoke-Loki -Exe $loki -Arguments @('store', 'init', $DisposableStore)
            $null = Invoke-Loki -Exe $loki -Arguments @('machine', 'register', '--allow-profile', 'work', '--active-profile', 'work')
            $null = Invoke-Loki -Exe $loki -Arguments @('verify', 'work')
            $null = Invoke-Loki -Exe $loki -Arguments @('switch', 'work', '--dry-run')
            $summary.store_smoke = $true
        }

        if ($RunSymlinkSwitch) {
            Invoke-Step "symlink switch smoke" {
                Remove-Item -LiteralPath $SymlinkTarget -Force -ErrorAction SilentlyContinue
                $storeFile = Join-Path $DisposableStore 'profiles\work\core\files\symlink-probe.txt'
                $manifest = Join-Path $DisposableStore 'profiles\work\core\manifest.yaml'
                'hello from Loki installer smoke' | Set-Content -LiteralPath $storeFile -Encoding UTF8
                @"
version: 1
name: work-core
files:
  - id: symlink-probe
    source: files/symlink-probe.txt
    target: ~/loki-installer-smoke/symlink-probe.txt
    mode: symlink
skills: []
ignore: []
merge_rules: {}
targets: {}
"@ | Set-Content -LiteralPath $manifest -Encoding UTF8
                $null = Invoke-Loki -Exe $loki -Arguments @('verify', 'work')
                $null = Invoke-Loki -Exe $loki -Arguments @('switch', 'work', '--dry-run')
                $switch = Invoke-Loki -Exe $loki -Arguments @('switch', 'work', '--yes') -AllowFailure
                if ($switch.ExitCode -eq 0) {
                    $item = Get-Item -LiteralPath $SymlinkTarget -Force
                    Assert-True ([bool]($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) "symlink target is not a reparse point"
                    $summary.symlink_switch = "passed"
                } elseif ($RequireSymlinkSwitch) {
                    throw "symlink switch failed: $($switch.Output -join "`n")"
                } else {
                    $summary.symlink_switch = "failed-allowed"
                    Write-Warning ($switch.Output -join "`n")
                }
            }
        }
    } finally {
        $env:LOCALAPPDATA = $oldLocalAppData
    }

    Invoke-Step "uninstall" {
        & $UninstallScript -InstallDir $InstallDir
        if ($LASTEXITCODE -ne 0) { throw "uninstall script failed with exit $LASTEXITCODE" }
        $summary.uninstall_removed_binary = -not (Test-Path -LiteralPath (Join-Path $InstallDir 'loki.exe'))
        Assert-True $summary.uninstall_removed_binary "loki.exe remains after uninstall"
        if ($TestPathUpdate) {
            $pathStillPresent = (Get-UserPathEntries | ForEach-Object { $_.TrimEnd('\') }) -contains $InstallDir.TrimEnd('\')
            Assert-True (-not $pathStillPresent) "install dir remains in user PATH after uninstall"
        }
        $summary.state_preserved = Test-Path -LiteralPath (Join-Path $SmokeLocalAppData 'loki-profile-manager')
        $summary.store_preserved = Test-Path -LiteralPath $DisposableStore
        Assert-True $summary.store_preserved "disposable store should be preserved by default uninstall"
    }

    [pscustomobject]$summary | ConvertTo-Json -Depth 5
} finally {
    Remove-Item -LiteralPath $SymlinkTarget -Force -ErrorAction SilentlyContinue
}
