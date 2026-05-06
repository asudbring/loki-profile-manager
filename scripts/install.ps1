#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$Version = "",
    [string]$ArchivePath = "",
    [string]$ChecksumsPath = "",
    [string]$InstallDir = "",
    [switch]$AddToPath,
    [switch]$NoPath,
    [switch]$Force,
    [string]$StorePath = "",
    [switch]$RequireSymlink,
    [switch]$EnableDeveloperMode,
    [switch]$ElevateForDeveloperMode
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = "Stop"

function Test-IsWindowsHost {
    return [Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT
}

function Resolve-FullPath {
    param([string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path)) { return "" }
    $expanded = [Environment]::ExpandEnvironmentVariables($Path)
    if ([IO.Path]::IsPathRooted($expanded)) { return [IO.Path]::GetFullPath($expanded) }
    return [IO.Path]::GetFullPath((Join-Path (Get-Location).Path $expanded))
}

function Get-DefaultInstallDir {
    $root = $env:LOCALAPPDATA
    if ([string]::IsNullOrWhiteSpace($root)) {
        if ([string]::IsNullOrWhiteSpace($env:USERPROFILE)) { throw "LOCALAPPDATA and USERPROFILE are not set" }
        $root = Join-Path $env:USERPROFILE 'AppData\Local'
    }
    return Join-Path $root 'Programs\Loki'
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

function Find-ReleaseArchive {
    param([string]$RequestedArchive, [string]$ExpectedVersion)
    if (-not [string]::IsNullOrWhiteSpace($RequestedArchive)) { return (Resolve-FullPath $RequestedArchive) }

    $arch = Get-LokiArch
    $patterns = @()
    if (-not [string]::IsNullOrWhiteSpace($ExpectedVersion)) {
        $patterns += "loki_${ExpectedVersion}_windows_${arch}.zip"
    }
    $patterns += "loki_*_windows_${arch}.zip"

    $roots = @()
    if (-not [string]::IsNullOrWhiteSpace($PSScriptRoot)) { $roots += $PSScriptRoot }
    $roots += (Get-Location).Path
    $roots = @($roots | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -Unique)

    foreach ($root in $roots) {
        foreach ($pattern in $patterns) {
            $candidate = Get-ChildItem -LiteralPath $root -Filter $pattern -File -ErrorAction SilentlyContinue |
                Sort-Object LastWriteTime -Descending |
                Select-Object -First 1
            if ($candidate) { return $candidate.FullName }
        }
    }
    throw "release archive not found; pass -ArchivePath"
}

function Find-ChecksumsFile {
    param([string]$RequestedPath, [string]$Archive)
    if (-not [string]::IsNullOrWhiteSpace($RequestedPath)) { return (Resolve-FullPath $RequestedPath) }
    $archiveDir = Split-Path -Parent $Archive
    foreach ($candidate in @((Join-Path $archiveDir 'checksums.txt'), (Join-Path $PSScriptRoot 'checksums.txt'), (Join-Path (Get-Location).Path 'checksums.txt'))) {
        if (-not [string]::IsNullOrWhiteSpace($candidate) -and (Test-Path -LiteralPath $candidate)) { return (Resolve-FullPath $candidate) }
    }
    throw "checksums.txt not found; pass -ChecksumsPath"
}

function Get-ExpectedSha256 {
    param([string]$Checksums, [string]$Asset)
    $assetName = [IO.Path]::GetFileName($Asset)
    foreach ($line in Get-Content -LiteralPath $Checksums) {
        if ($line -match '^\s*([0-9A-Fa-f]{64})\s+\*?(.+?)\s*$') {
            $hash = $matches[1].ToLowerInvariant()
            $name = [IO.Path]::GetFileName($matches[2].Trim())
            if ($name -eq $assetName) { return $hash }
        }
    }
    throw "checksum entry not found for $assetName in $Checksums"
}

function Assert-Checksum {
    param([string]$Asset, [string]$Checksums)
    $expected = Get-ExpectedSha256 -Checksums $Checksums -Asset $Asset
    $actual = (Get-FileHash -LiteralPath $Asset -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "checksum mismatch for $([IO.Path]::GetFileName($Asset)): got $actual want $expected" }
}

function Invoke-LokiChecked {
    param([string]$Exe, [string[]]$Arguments)
    $output = & $Exe @Arguments 2>&1
    $code = $LASTEXITCODE
    if ($code -ne 0) { throw "loki $($Arguments -join ' ') failed with exit $code`n$output" }
    return @($output)
}

function Get-UserPathEntries {
    $path = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ([string]::IsNullOrWhiteSpace($path)) { return @() }
    return @($path -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
}

function Add-InstallDirToUserPath {
    param([string]$Directory)
    $normalized = $Directory.TrimEnd('\')
    $entries = @(Get-UserPathEntries)
    $exists = $false
    foreach ($entry in $entries) {
        if ($entry.TrimEnd('\') -ieq $normalized) { $exists = $true; break }
    }
    if (-not $exists) {
        $entries += $Directory
        [Environment]::SetEnvironmentVariable('Path', ($entries -join ';'), 'User')
    }
    $processEntries = @($env:Path -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    $processExists = $false
    foreach ($entry in $processEntries) {
        if ($entry.TrimEnd('\') -ieq $normalized) { $processExists = $true; break }
    }
    if (-not $processExists) { $env:Path = (($processEntries + $Directory) -join ';') }
    return (-not $exists)
}

function Test-IsAdmin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Enable-DeveloperModeInCurrentProcess {
    $path = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock'
    New-Item -Path $path -Force | Out-Null
    New-ItemProperty -Path $path -Name AllowDevelopmentWithoutDevLicense -PropertyType DWord -Value 1 -Force | Out-Null
}

function Enable-DeveloperModeElevated {
    $script = @"
`$ErrorActionPreference = 'Stop'
New-Item -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock' -Force | Out-Null
New-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock' -Name AllowDevelopmentWithoutDevLicense -PropertyType DWord -Value 1 -Force | Out-Null
"@
    $path = Join-Path ([IO.Path]::GetTempPath()) ("loki-enable-dev-mode-" + [guid]::NewGuid().ToString('N') + ".ps1")
    Set-Content -LiteralPath $path -Value $script -Encoding UTF8
    try {
        $hostExe = (Get-Process -Id $PID).Path
        Start-Process -FilePath $hostExe -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $path) -Verb RunAs -Wait
    } finally {
        Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
    }
}

function Test-SymlinkCapability {
    $root = Join-Path ([IO.Path]::GetTempPath()) ("loki-symlink-probe-" + [guid]::NewGuid().ToString('N'))
    $target = Join-Path $root 'target.txt'
    $link = Join-Path $root 'link.txt'
    try {
        New-Item -ItemType Directory -Force -Path $root | Out-Null
        'loki symlink probe' | Set-Content -LiteralPath $target -Encoding UTF8
        New-Item -ItemType SymbolicLink -Path $link -Target $target -ErrorAction Stop | Out-Null
        return $true
    } catch {
        return $false
    } finally {
        Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Copy-IfExists {
    param([string]$Source, [string]$Destination)
    if ([string]::IsNullOrWhiteSpace($Source) -or -not (Test-Path -LiteralPath $Source)) { return }
    $sourceFull = [IO.Path]::GetFullPath($Source)
    $destFull = [IO.Path]::GetFullPath($Destination)
    if ($sourceFull -ieq $destFull) { return }
    Copy-Item -LiteralPath $Source -Destination $Destination -Force
}

if (-not (Test-IsWindowsHost)) { throw "install.ps1 must run on Windows" }
if ($AddToPath -and $NoPath) { throw "-AddToPath and -NoPath are mutually exclusive" }

$ArchivePath = Find-ReleaseArchive -RequestedArchive $ArchivePath -ExpectedVersion $Version
$ChecksumsPath = Find-ChecksumsFile -RequestedPath $ChecksumsPath -Archive $ArchivePath
if ([string]::IsNullOrWhiteSpace($InstallDir)) { $InstallDir = Get-DefaultInstallDir }
$InstallDir = Resolve-FullPath $InstallDir
$StorePath = Resolve-FullPath $StorePath
$shouldAddPath = $AddToPath.IsPresent -or (-not $NoPath.IsPresent)
$tempRoot = Join-Path ([IO.Path]::GetTempPath()) ("loki-install-" + [guid]::NewGuid().ToString('N'))

try {
    Write-Host "== verify archive =="
    if (-not (Test-Path -LiteralPath $ArchivePath)) { throw "archive not found: $ArchivePath" }
    if (-not (Test-Path -LiteralPath $ChecksumsPath)) { throw "checksums not found: $ChecksumsPath" }
    Assert-Checksum -Asset $ArchivePath -Checksums $ChecksumsPath

    Write-Host "== extract archive =="
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    Expand-Archive -LiteralPath $ArchivePath -DestinationPath $tempRoot -Force
    $extractedExe = Get-ChildItem -LiteralPath $tempRoot -Filter 'loki.exe' -File -Recurse | Select-Object -First 1
    if (-not $extractedExe) { throw "loki.exe missing from archive" }

    $versionOutput = Invoke-LokiChecked -Exe $extractedExe.FullName -Arguments @('--version')
    $installedVersion = ($versionOutput | Select-Object -First 1).ToString().Trim()
    if (-not [string]::IsNullOrWhiteSpace($Version) -and $installedVersion -ne $Version) {
        throw "version mismatch: got $installedVersion want $Version"
    }

    Write-Host "== install files =="
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $installedExe = Join-Path $InstallDir 'loki.exe'
    if ((Test-Path -LiteralPath $installedExe) -and -not $Force) {
        throw "loki.exe already exists at $installedExe; pass -Force to overwrite"
    }
    Copy-Item -LiteralPath $extractedExe.FullName -Destination $installedExe -Force

    foreach ($name in @('README.md', 'CHANGELOG.md', 'install.ps1', 'uninstall.ps1')) {
        $source = Get-ChildItem -LiteralPath $tempRoot -Filter $name -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($source) {
            Copy-IfExists -Source $source.FullName -Destination (Join-Path $InstallDir $name)
        } elseif (-not [string]::IsNullOrWhiteSpace($PSScriptRoot)) {
            Copy-IfExists -Source (Join-Path $PSScriptRoot $name) -Destination (Join-Path $InstallDir $name)
        }
    }

    $metadata = [ordered]@{
        version = $installedVersion
        installed_at = (Get-Date).ToUniversalTime().ToString('o')
        install_dir = $InstallDir
        archive = $ArchivePath
        path_added = $shouldAddPath
        store_path = $StorePath
        developer_mode_requested = [bool]$EnableDeveloperMode
    }
    $metadata | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $InstallDir 'install-metadata.json') -Encoding UTF8

    if ($shouldAddPath) {
        Write-Host "== update user PATH =="
        $added = Add-InstallDirToUserPath -Directory $InstallDir
        if ($added) { Write-Host "Added $InstallDir to user PATH. Open a new terminal to use loki by name." }
        else { Write-Host "Install directory already present in user PATH." }
    }

    Write-Host "== smoke =="
    $null = Invoke-LokiChecked -Exe $installedExe -Arguments @('--version')
    $null = Invoke-LokiChecked -Exe $installedExe -Arguments @('doctor', '--json')
    $null = Invoke-LokiChecked -Exe $installedExe -Arguments @('tui', '--help')

    Write-Host "== symlink probe =="
    $symlinkOK = Test-SymlinkCapability
    if (-not $symlinkOK -and $EnableDeveloperMode) {
        if (Test-IsAdmin) {
            Enable-DeveloperModeInCurrentProcess
        } elseif ($ElevateForDeveloperMode) {
            Enable-DeveloperModeElevated
        } else {
            Write-Warning "Developer Mode requested but process is not elevated. Re-run elevated or add -ElevateForDeveloperMode."
        }
        $symlinkOK = Test-SymlinkCapability
    }
    if (-not $symlinkOK) {
        $message = "Windows symlink probe failed. Enable Developer Mode or run elevated before using symlink activations."
        if ($RequireSymlink) { throw $message }
        Write-Warning $message
    }

    if (-not [string]::IsNullOrWhiteSpace($StorePath)) {
        Write-Host "== configure store =="
        $null = Invoke-LokiChecked -Exe $installedExe -Arguments @('store', 'init', $StorePath)
    }

    Write-Host "Loki installed"
    Write-Host "Version: $installedVersion"
    Write-Host "Install dir: $InstallDir"
    if ($shouldAddPath) { Write-Host "Next: open a new terminal, then run: loki doctor" }
    else { Write-Host "Next: & '$installedExe' doctor" }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}
