# Download WebView2 Fixed Runtime (x86) for portable distribution
# Places runtime in build/bin/WebView2/ next to xQuakShell.exe
# Source: https://github.com/westinyang/WebView2RuntimeArchive (community archive of Microsoft Fixed Runtime)

$ErrorActionPreference = "Stop"
$version = "133.0.3065.92"
$cabUrl = "https://github.com/westinyang/WebView2RuntimeArchive/releases/download/$version/Microsoft.WebView2.FixedVersionRuntime.$version.x86.cab"
$buildBin = Join-Path $PSScriptRoot "..\build\bin"
$webview2Dir = Join-Path $buildBin "WebView2"
$tempCab = Join-Path $env:TEMP "WebView2.FixedVersionRuntime.$version.x86.cab"
$tempExtract = Join-Path $env:TEMP "WebView2_extract_$version"

if (-not (Test-Path $buildBin)) {
    Write-Error "Build directory not found. Run 'make build' first."
    exit 1
}

if (Test-Path $webview2Dir) {
    Write-Host "WebView2 runtime already present at $webview2Dir"
    exit 0
}

Write-Host "Downloading WebView2 Fixed Runtime x86 ($version, ~214 MB)..."
Write-Host "Using curl for faster download (PowerShell Invoke-WebRequest is very slow for large files)."

$downloaded = $false

# Prefer curl.exe - fast, shows progress, built into Windows 10+
if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
    try {
        & curl.exe -L -o $tempCab $cabUrl
        if ($LASTEXITCODE -eq 0 -and (Test-Path $tempCab) -and (Get-Item $tempCab).Length -gt 100000000) {
            $downloaded = $true
        }
    } catch { }
}

# Fallback: Net.WebClient (faster than Invoke-WebRequest for large files)
if (-not $downloaded) {
    try {
        $ProgressPreference = 'SilentlyContinue'
        (New-Object Net.WebClient).DownloadFile($cabUrl, $tempCab)
        $ProgressPreference = 'Continue'
        $downloaded = (Test-Path $tempCab) -and (Get-Item $tempCab).Length -gt 100000000
    } catch { }
}

# Last resort: Invoke-WebRequest with progress disabled (otherwise 10x slower)
if (-not $downloaded) {
    try {
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $cabUrl -OutFile $tempCab -UseBasicParsing
        $ProgressPreference = 'Continue'
    } catch {
        Write-Error "Download failed: $_"
        exit 1
    }
}

if (-not (Test-Path $tempCab) -or (Get-Item $tempCab).Length -lt 100000000) {
    Write-Error "Download incomplete or failed. File size: $((Get-Item $tempCab -ErrorAction SilentlyContinue).Length) bytes. Expected ~214 MB."
    exit 1
}

Write-Host "Extracting CAB..."
if (Test-Path $tempExtract) {
    Remove-Item -Recurse -Force $tempExtract
}
New-Item -ItemType Directory -Path $tempExtract -Force | Out-Null

# Use Windows expand to extract CAB (destination must exist, no trailing backslash)
$expandResult = & expand $tempCab -F:* $tempExtract 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Error "Extract failed: $expandResult"
    Remove-Item $tempCab -Force -ErrorAction SilentlyContinue
    exit 1
}

# CAB may extract to subfolder Microsoft.WebView2.FixedVersionRuntime.{version}.{arch}
$runtimeSource = $tempExtract
$msedgeExe = Join-Path $runtimeSource "msedgewebview2.exe"
if (-not (Test-Path $msedgeExe)) {
    $subdirs = Get-ChildItem $tempExtract -Directory
    foreach ($d in $subdirs) {
        $candidate = Join-Path $d.FullName "msedgewebview2.exe"
        if (Test-Path $candidate) {
            $runtimeSource = $d.FullName
            $msedgeExe = $candidate
            break
        }
    }
}
if (-not (Test-Path $msedgeExe)) {
    Write-Error "Runtime structure unexpected. msedgewebview2.exe not found."
    Get-ChildItem $tempExtract -Recurse | Select-Object -First 20 | ForEach-Object { Write-Host $_.FullName }
    Remove-Item $tempCab -Force -ErrorAction SilentlyContinue
    Remove-Item $tempExtract -Recurse -Force -ErrorAction SilentlyContinue
    exit 1
}

New-Item -ItemType Directory -Path $webview2Dir -Force | Out-Null
Copy-Item -Path "$runtimeSource\*" -Destination $webview2Dir -Recurse -Force

# Cleanup
Remove-Item $tempCab -Force -ErrorAction SilentlyContinue
Remove-Item $tempExtract -Recurse -Force -ErrorAction SilentlyContinue

Write-Host "WebView2 runtime installed to $webview2Dir"
Write-Host "Portable build complete. Copy build/bin/ to distribute."
