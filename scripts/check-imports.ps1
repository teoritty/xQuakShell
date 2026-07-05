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
    '"github.com/'
) -Label "internal/usecase must not import internal/infra or third-party packages"

$usecaseGolangHits = Get-ChildItem -Path $usecase -Filter "*.go" -Recurse |
    Where-Object { $_.Name -notlike "*_test.go" } |
    Select-String -Pattern '"golang.org/'
if ($usecaseGolangHits) {
    Write-Error "internal/usecase production code must not import golang.org packages:`n$($usecaseGolangHits | Out-String)"
    exit 1
}

$usecasePkgHits = Get-ChildItem -Path $usecase -Filter "*.go" -Recurse |
    Select-String -Pattern '"ssh-client/internal/pkg' |
    Where-Object { $_.Line -notmatch '"ssh-client/internal/pkg/safego"' }
if ($usecasePkgHits) {
    Write-Error "internal/usecase may import only internal/pkg/safego from internal/pkg:`n$($usecasePkgHits | Out-String)"
    exit 1
}

$domain = Join-Path $root "internal\domain"
$domainHits = Get-ChildItem -Path $domain -Filter "*.go" -Recurse |
    Select-String -Pattern '"ssh-client/' |
    Where-Object { $_.Line -notmatch '"ssh-client/internal/domain' }
if ($domainHits) {
    Write-Error "internal/domain must not import outside domain (except stdlib and golang.org/x/crypto/ssh):`n$($domainHits | Out-String)"
    exit 1
}

$presentation = Join-Path $root "internal\presentation"
Test-LayerImports -Dir $presentation -ForbiddenPatterns @(
    '"ssh-client/internal/infra'
) -Label "internal/presentation must not import internal/infra"

$presentationRepoHits = @()
$presentationRepoHits += Get-ChildItem -Path $presentation -Filter "*.go" -Recurse |
    Where-Object { $_.Name -notlike "*_test.go" } |
    Select-String -Pattern '\.(connRepo|passwordRepo|identRepo)\.'
$presentationRepoHits += Get-ChildItem -Path $presentation -Filter "*.go" -Recurse |
    Where-Object { $_.Name -notlike "*_test.go" } |
    Select-String -Pattern '\t(connRepo|passwordRepo|identRepo)\s{2,}domain\.(Connection|Password|Identity)Repository'
if ($presentationRepoHits) {
    Write-Error "internal/presentation must not reference vault repositories directly; use VaultService:`n$($presentationRepoHits | Out-String)"
    exit 1
}

$infra = Join-Path $root "internal\infra"
Test-LayerImports -Dir $infra -ForbiddenPatterns @(
    '"ssh-client/internal/usecase'
) -Label "internal/infra must not import internal/usecase"

Write-Host "layer import check: OK"
