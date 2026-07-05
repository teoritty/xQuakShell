$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

function Test-FileLineCount {
    param(
        [string]$Pattern,
        [int]$MaxLines,
        [string]$Label
    )
    $file = Join-Path $root $Pattern
    if (-not (Test-Path $file)) {
        Write-Error "missing file: $Pattern"
        exit 1
    }
    $lineCount = (Get-Content $file | Measure-Object -Line).Lines
    if ($lineCount -gt $MaxLines) {
        Write-Error "${Label}: ${Pattern} has ${lineCount} lines (max ${MaxLines})"
        exit 1
    }
}

# Enforces the SessionManager decomposition from ADR-009.
# If this script fails, someone is putting logic back into the God Object.

# 1. Facade file must stay a thin delegate — hard cap.
Test-FileLineCount -Pattern "internal\usecase\session_manager.go" -MaxLines 120 `
    -Label "session_manager.go facade grew past its delegate-only budget (ADR-009)"

# 2. Nobody outside session_registry.go may touch the sessions map/mutex directly.
$offenders = Get-ChildItem (Join-Path $root "internal\usecase\session_*.go") -Exclude "session_registry.go","*_test.go" |
    Select-String -Pattern "m\.sessions\[|m\.mu\.(Lock|RLock)"
if ($offenders) {
    Write-Error "Direct access to session map/mutex outside session_registry.go (ADR-009):`n$offenders"
    exit 1
}

# 3. InitSessionIO must block on registry.WaitReady, not poll with a ticker.
$ioContent = Get-Content (Join-Path $root "internal\usecase\session_io_service.go") -Raw
if ($ioContent -match '(?s)func \(s \*SessionIOService\) InitSessionIO.*?(?=^func )') {
    $initBlock = $Matches[0]
    if ($initBlock -match 'time\.NewTicker|100 \* time\.Millisecond') {
        Write-Error "InitSessionIO must not busy-poll session readiness (ADR-009)"
        exit 1
    }
    if ($initBlock -notmatch 'WaitReady') {
        Write-Error "InitSessionIO must wait via SessionRegistry.WaitReady (ADR-009)"
        exit 1
    }
}

Write-Host "session manager boundaries check: OK"
