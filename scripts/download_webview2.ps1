# Downloads the WebView2 Fixed Version runtime bundled with the portable Windows archive and places
# it in build/bin/WebView2, next to xQuakShell.exe.
#
# The architecture is not a detail. Wails does not scan for a runtime: it asks the loader for the
# version of the one it was given, and the loader opens exactly
#
#     <runtime>\EBWebView\<arch>\EmbeddedBrowserWebView.dll
#
# where <arch> comes from the architecture of the *application* process. The release builds
# windows/amd64, so anything but the x64 runtime leaves that file missing, and a missing file there
# is fatal rather than a fallback: the app exits before creating a window, with no message. This
# script previously fetched the x86 runtime, which is how a release shipped that did nothing at all
# when launched. Both runtimes are ~200 MB and extract identically, so nothing but the check at the
# end of this script tells them apart.
#
# Source: https://github.com/westinyang/WebView2RuntimeArchive — a community mirror of Microsoft's
# Fixed Version runtime, which Microsoft publishes only behind a per-version download page. Being a
# third-party mirror of code that ships inside our release, its contents are verified below against
# Microsoft's Authenticode signature rather than trusted.

$ErrorActionPreference = "Stop"

$version = "133.0.3065.92"
$arch = "x64"
$cabUrl = "https://github.com/westinyang/WebView2RuntimeArchive/releases/download/$version/Microsoft.WebView2.FixedVersionRuntime.$version.$arch.cab"
$buildBin = Join-Path $PSScriptRoot "..\build\bin"
$webview2Dir = Join-Path $buildBin "WebView2"
$tempCab = Join-Path $env:TEMP "WebView2.FixedVersionRuntime.$version.$arch.cab"
$tempExtract = Join-Path $env:TEMP "WebView2_extract_$version`_$arch"

# The one file that has to exist for the runtime to be usable, relative to the runtime root.
$clientDllRelative = "EBWebView\$arch\EmbeddedBrowserWebView.dll"

if (-not (Test-Path $buildBin)) {
    Write-Error "Build directory not found. Run 'make build' first."
    exit 1
}

# An existing directory is only reusable if it is the runtime we need: a leftover from the x86 era
# would otherwise be kept forever and silently break the archive again.
if (Test-Path $webview2Dir) {
    if (Test-Path (Join-Path $webview2Dir $clientDllRelative)) {
        Write-Host "WebView2 $arch runtime already present at $webview2Dir"
        exit 0
    }
    Write-Host "Replacing the runtime at ${webview2Dir}: it does not contain $clientDllRelative"
    Remove-Item -Recurse -Force $webview2Dir
}

Write-Host "Downloading WebView2 Fixed Runtime $arch ($version, ~250 MB)..."
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
    Write-Error "Download incomplete or failed. File size: $((Get-Item $tempCab -ErrorAction SilentlyContinue).Length) bytes. Expected ~250 MB."
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

# The check that distinguishes the two runtimes. It is deliberately the exact path Wails resolves,
# not an approximation of it.
$clientDll = Join-Path $runtimeSource $clientDllRelative
if (-not (Test-Path $clientDll)) {
    Write-Error ("Wrong runtime architecture: $clientDllRelative is missing. " +
                 "Wails resolves that exact path for a windows/amd64 build, and a runtime without " +
                 "it makes the application exit at startup without a window or a message.")
    Get-ChildItem (Join-Path $runtimeSource "EBWebView") -Directory -ErrorAction SilentlyContinue |
        ForEach-Object { Write-Host "  found architecture folder: $($_.Name)" }
    Remove-Item $tempCab -Force -ErrorAction SilentlyContinue
    Remove-Item $tempExtract -Recurse -Force -ErrorAction SilentlyContinue
    exit 1
}

# This runtime is a browser, fetched from a third-party mirror and then shipped inside our release
# as executable code. Microsoft signs both of these files; verifying that signature is what makes
# the mirror untrusted infrastructure rather than a trusted one.
Write-Host "Verifying Authenticode signatures..."
foreach ($file in @($msedgeExe, $clientDll)) {
    $sig = Get-AuthenticodeSignature $file
    $subject = $sig.SignerCertificate.Subject
    if ($sig.Status -ne "Valid" -or $subject -notlike "*O=Microsoft Corporation*") {
        Write-Error ("Signature check failed for $file`n" +
                     "  status:  $($sig.Status)`n" +
                     "  subject: $subject`n" +
                     "Refusing to package an unverified browser runtime into the release.")
        Remove-Item $tempCab -Force -ErrorAction SilentlyContinue
        Remove-Item $tempExtract -Recurse -Force -ErrorAction SilentlyContinue
        exit 1
    }
    Write-Host "  ok: $(Split-Path -Leaf $file) - $subject"
}

# Only now, with everything verified, does anything land next to the executable: a rejected download
# left behind would be picked up as the runtime at the next launch.
New-Item -ItemType Directory -Path $webview2Dir -Force | Out-Null
Copy-Item -Path "$runtimeSource\*" -Destination $webview2Dir -Recurse -Force

if (-not (Test-Path (Join-Path $webview2Dir $clientDllRelative))) {
    Write-Error "Copy did not produce $clientDllRelative under $webview2Dir."
    Remove-Item -Recurse -Force $webview2Dir -ErrorAction SilentlyContinue
    exit 1
}

# Cleanup
Remove-Item $tempCab -Force -ErrorAction SilentlyContinue
Remove-Item $tempExtract -Recurse -Force -ErrorAction SilentlyContinue

Write-Host "WebView2 $arch runtime installed to $webview2Dir"
Write-Host "Portable build complete. Copy build/bin/ to distribute."
