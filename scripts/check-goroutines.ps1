# Fails if production Go files launch goroutines without safego.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

$excludeDirs = @(
    (Join-Path $root "test"),
    (Join-Path $root "internal\pkg\safego")
)

function Should-SkipFile {
    param([string]$Path)
    if ($Path -match '_test\.go$') {
        return $true
    }
    foreach ($dir in $excludeDirs) {
        if ($Path.StartsWith($dir, [System.StringComparison]::OrdinalIgnoreCase)) {
            return $true
        }
    }
    return $false
}

$scanRoots = @(
    (Join-Path $root "internal"),
    (Join-Path $root "app.go"),
    (Join-Path $root "main.go"),
    (Join-Path $root "main_plugins.go"),
    (Join-Path $root "main_connectors.go")
)

$hits = @()
foreach ($scanRoot in $scanRoots) {
    if (-not (Test-Path $scanRoot)) {
        continue
    }
    if (Test-Path $scanRoot -PathType Leaf) {
        $files = @(Get-Item $scanRoot)
    } else {
        $files = Get-ChildItem -Path $scanRoot -Filter "*.go" -Recurse -File
    }
    foreach ($file in $files) {
        if (Should-SkipFile $file.FullName) {
            continue
        }
        $found = Select-String -Path $file.FullName -Pattern '^\s+go\s+'
        if ($found) {
            $hits += $found
        }
    }
}

if ($hits) {
    Write-Error @"
production code must use safego.GoNamed instead of raw 'go' launches:
$($hits | Out-String)
"@
    exit 1
}

Write-Host "goroutine check: OK"
