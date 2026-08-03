# Enforces composition root policy (ADR-010): DI only in main.go + main_*.go; app.go is Wails facade.
$ErrorActionPreference = "Stop"

go test ./test/unit/architecture/... -run TestCompositionRoot -count=1
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "composition root check: OK"
