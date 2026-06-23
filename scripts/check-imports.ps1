# Enforces layer import rules from CONTRIBUTING.md / docs/architecture.md.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

function Test-LayerImports {
    param(
        [string]$Dir,
        [string[]]$ForbiddenPatterns,
        [string]$Label
    )
    $hits = @()
    foreach ($pattern in $ForbiddenPatterns) {
        $found = Get-ChildItem -Path $Dir -Filter "*.go" -Recurse |
            Select-String -Pattern $pattern
        if ($found) {
            $hits += $found
        }
    }
    if ($hits) {
        Write-Error "${Label}:`n$($hits | Out-String)"
        exit 1
    }
}

$usecase = Join-Path $root "internal\usecase"
Test-LayerImports -Dir $usecase -ForbiddenPatterns @(
    '"ssh-client/internal/infra',
    '"ssh-client/internal/pkg',
    '"github.com/'
) -Label "internal/usecase must import only internal/domain and stdlib"

$domain = Join-Path $root "internal\domain"
$domainHits = Get-ChildItem -Path $domain -Filter "*.go" -Recurse |
    Select-String -Pattern '"ssh-client/' |
    Where-Object { $_.Line -notmatch '"ssh-client/internal/domain' }
if ($domainHits) {
    Write-Error "internal/domain must not import outside domain (except stdlib and golang.org/x/crypto/ssh):`n$($domainHits | Out-String)"
    exit 1
}

Write-Host "layer import check: OK"
