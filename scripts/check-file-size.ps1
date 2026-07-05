$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

function Test-FileLineCount {
    param(
        [string]$Pattern,
        [int]$MaxLines,
        [string]$Label
    )
    $hits = @()
    $files = Get-ChildItem -Path (Join-Path $root $Pattern) -Filter "*.go" -Recurse -ErrorAction SilentlyContinue
    foreach ($file in $files) {
        $lineCount = (Get-Content $file.FullName | Measure-Object -Line).Lines
        if ($lineCount -gt $MaxLines) {
            $relative = $file.FullName.Substring($root.Length + 1)
            $hits += "${relative}: ${lineCount} lines (max ${MaxLines})"
        }
    }
    if ($hits) {
        Write-Error "${Label}:`n$($hits -join [Environment]::NewLine)"
        exit 1
    }
}

Test-FileLineCount -Pattern "internal\usecase\github_*.go" -MaxLines 300 -Label "github usecase files exceed 300 lines"
Test-FileLineCount -Pattern "internal\infra\plugin\process_*.go" -MaxLines 300 -Label "process host files exceed 300 lines"

$facadeChecks = @(
    @{ Path = "internal\usecase\github_plugin_service.go"; Max = 100 },
    @{ Path = "internal\infra\plugin\process_host.go"; Max = 100 }
)
foreach ($check in $facadeChecks) {
    $file = Join-Path $root $check.Path
    if (-not (Test-Path $file)) {
        Write-Error "missing facade file: $($check.Path)"
        exit 1
    }
    $lineCount = (Get-Content $file | Measure-Object -Line).Lines
    if ($lineCount -gt $check.Max) {
        Write-Error "$($check.Path): ${lineCount} lines (facade max $($check.Max))"
        exit 1
    }
}

Write-Host "file size check: OK"
