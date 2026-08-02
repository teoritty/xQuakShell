# Enforces filesystem trust boundary rules (ADR-007).
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

function FailIfMatch {
    param(
        [string]$Label,
        [object[]]$Hits
    )
    if ($Hits) {
        Write-Error "${Label}:`n$($Hits | Out-String)"
        exit 1
    }
}

function Find-NonCommentMatches {
    param(
        [string]$Path,
        [string[]]$Patterns
    )
    $hits = @()
    $lines = Get-Content -Path $Path
    for ($i = 0; $i -lt $lines.Count; $i++) {
        $trimmed = $lines[$i].TrimStart()
        if ($trimmed.StartsWith("//")) {
            continue
        }
        foreach ($pattern in $Patterns) {
            if ($lines[$i] -match $pattern) {
                $hits += [PSCustomObject]@{
                    Path     = $Path
                    Line     = $i + 1
                    LineText = $lines[$i]
                }
                break
            }
        }
    }
    return $hits
}

$pluginDir = Join-Path $root "internal\infra\plugin"
$pluginHits = @()
Get-ChildItem -Path $pluginDir -Filter "*.go" -Recurse | ForEach-Object {
    $pluginHits += Find-NonCommentMatches -Path $_.FullName -Patterns @(
        '"xquakshell/internal/infra/host"'
        'domain\.HostFileSystem'
    )
}
FailIfMatch -Label "infra/plugin must not import or use HostFileSystem" -Hits $pluginHits

$transferFile = Join-Path $root "internal\usecase\transfer_service.go"
$portableInTransfer = Find-NonCommentMatches -Path $transferFile -Patterns @(
    'PortableDataStore'
    'portableData'
)
FailIfMatch -Label "transfer_service must use HostFileSystem only" -Hits $portableInTransfer

$handlersFile = Join-Path $root "internal\presentation\wails\handlers_local_fs.go"
$handlerLines = Get-Content -Path $handlersFile

function Test-HandlerBodyUses {
    param(
        [string]$FuncPattern,
        [string]$ForbiddenPattern
    )
    $inFunc = $false
    $braceDepth = 0
    $hits = @()
    foreach ($line in $handlerLines) {
        if (-not $inFunc -and $line -match $FuncPattern) {
            $inFunc = $true
            $braceDepth = 0
        }
        if ($inFunc) {
            $braceDepth += ($line.ToCharArray() | Where-Object { $_ -eq '{' }).Count
            $braceDepth -= ($line.ToCharArray() | Where-Object { $_ -eq '}' }).Count
            $trimmed = $line.TrimStart()
            if (-not $trimmed.StartsWith("//") -and $line -match $ForbiddenPattern) {
                $hits += $line
            }
            if ($inFunc -and $braceDepth -le 0 -and $line -match '\}') {
                $inFunc = $false
            }
        }
    }
    return $hits
}

$hostInPortableHandlers = @()
$hostInPortableHandlers += Test-HandlerBodyUses -FuncPattern 'func \(a \*AppAPI\) GetPortableDataRoot' -ForbiddenPattern 'hostFS'
$hostInPortableHandlers += Test-HandlerBodyUses -FuncPattern 'func \(a \*AppAPI\) GetTempDir' -ForbiddenPattern 'hostFS'
FailIfMatch -Label "GetPortableDataRoot/GetTempDir must not use hostFS" -Hits $hostInPortableHandlers

$portableInHostHandlers = @()
$portableInHostHandlers += Test-HandlerBodyUses -FuncPattern 'func \(a \*AppAPI\) ListLocalPath' -ForbiddenPattern 'portableData'
$portableInHostHandlers += Test-HandlerBodyUses -FuncPattern 'func \(a \*AppAPI\) GetUserHomeDir' -ForbiddenPattern 'portableData'
FailIfMatch -Label "host FS handlers must not use portableData" -Hits $portableInHostHandlers

$forbiddenFsSymbols = @()
Get-ChildItem -Path $root -Filter "*.go" -Recurse |
    Where-Object { $_.FullName -notmatch '\\test\\' } |
    ForEach-Object {
        $forbiddenFsSymbols += Find-NonCommentMatches -Path $_.FullName -Patterns @(
            'LocalFileSystem'
            '\bNewLocalFS\b'
            'ErrLocalPathDenied'
        )
    }
FailIfMatch -Label "removed LocalFileSystem symbols must not remain in production code" -Hits $forbiddenFsSymbols

$usecaseDir = Join-Path $root "internal\usecase"
$usecaseFsHits = @()
Get-ChildItem -Path $usecaseDir -Filter "*.go" -Recurse | ForEach-Object {
    $usecaseFsHits += Find-NonCommentMatches -Path $_.FullName -Patterns @(
        '\bos\.(Stat|Lstat|Remove|RemoveAll|ReadFile|WriteFile|Open|Create|Mkdir|Rename|ReadDir|Chmod|Chown)\b'
        '"os"'
    )
}
FailIfMatch -Label "internal/usecase must not call os.* filesystem APIs" -Hits $usecaseFsHits

Write-Host "filesystem boundary check: OK"
