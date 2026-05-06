#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$InstallDir = "",
    [switch]$RemoveState,
    [string]$RemoveStore = "",
    [switch]$Force
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

function Get-UserPathEntries {
    $path = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ([string]::IsNullOrWhiteSpace($path)) { return @() }
    return @($path -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
}

function Remove-InstallDirFromUserPath {
    param([string]$Directory)
    $normalized = $Directory.TrimEnd('\')
    $entries = @(Get-UserPathEntries)
    $kept = @()
    $removed = $false
    foreach ($entry in $entries) {
        if ($entry.TrimEnd('\') -ieq $normalized) { $removed = $true; continue }
        $kept += $entry
    }
    if ($removed) { [Environment]::SetEnvironmentVariable('Path', ($kept -join ';'), 'User') }

    $processEntries = @($env:Path -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    $processKept = @()
    foreach ($entry in $processEntries) {
        if ($entry.TrimEnd('\') -ieq $normalized) { continue }
        $processKept += $entry
    }
    $env:Path = ($processKept -join ';')
    return $removed
}

function Confirm-DestructiveAction {
    param([string]$Phrase, [string]$Target)
    if ($Force) { return }
    $answer = Read-Host "Type $Phrase to delete $Target"
    if ($answer -ne $Phrase) { throw "confirmation failed for $Target" }
}

if (-not (Test-IsWindowsHost)) { throw "uninstall.ps1 must run on Windows" }
if ([string]::IsNullOrWhiteSpace($InstallDir)) { $InstallDir = Get-DefaultInstallDir }
$InstallDir = Resolve-FullPath $InstallDir
$RemoveStore = Resolve-FullPath $RemoveStore

Write-Host "== uninstall Loki =="
Write-Host "Install dir: $InstallDir"

$pathRemoved = Remove-InstallDirFromUserPath -Directory $InstallDir
if ($pathRemoved) { Write-Host "Removed install directory from user PATH. Open a new terminal to refresh PATH." }

$ownedFiles = @(
    'loki.exe',
    'README.md',
    'CHANGELOG.md',
    'install.ps1',
    'install-metadata.json',
    'loki-install.json'
)
foreach ($name in $ownedFiles) {
    $path = Join-Path $InstallDir $name
    if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Force }
}

$self = Join-Path $InstallDir 'uninstall.ps1'
if (Test-Path -LiteralPath $self) { Remove-Item -LiteralPath $self -Force -ErrorAction SilentlyContinue }

if (Test-Path -LiteralPath $InstallDir) {
    $remaining = Get-ChildItem -LiteralPath $InstallDir -Force -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $remaining) { Remove-Item -LiteralPath $InstallDir -Force }
}

if ($RemoveState) {
    $stateDir = Join-Path $env:LOCALAPPDATA 'loki-profile-manager'
    Confirm-DestructiveAction -Phrase 'DELETE LOKI STATE' -Target $stateDir
    if (Test-Path -LiteralPath $stateDir) { Remove-Item -LiteralPath $stateDir -Recurse -Force }
    Write-Host "Removed local state: $stateDir"
} else {
    Write-Host "Preserved local state under LOCALAPPDATA."
}

if (-not [string]::IsNullOrWhiteSpace($RemoveStore)) {
    Confirm-DestructiveAction -Phrase 'DELETE LOKI STORE' -Target $RemoveStore
    if (Test-Path -LiteralPath $RemoveStore) { Remove-Item -LiteralPath $RemoveStore -Recurse -Force }
    Write-Host "Removed store: $RemoveStore"
} else {
    Write-Host "Preserved synced stores and managed targets."
}

Write-Host "Loki uninstalled"
