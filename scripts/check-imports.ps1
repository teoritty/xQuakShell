# Fails if internal/usecase Go sources import internal/infra (layer rule).
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$usecase = Join-Path $root "internal\usecase"
$hits = Get-ChildItem -Path $usecase -Filter "*.go" -Recurse | Select-String -Pattern '"ssh-client/internal/infra'
if ($hits) {
    Write-Error "internal/usecase must not import internal/infra:`n$($hits | Out-String)"
    exit 1
}
Write-Host "layer import check: OK (usecase has no infra imports)"
