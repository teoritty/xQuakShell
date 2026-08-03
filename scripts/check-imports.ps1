# Enforces layer import rules via Go AST checker (canonical rules: test/unit/architecture/rules.go).
$ErrorActionPreference = "Stop"

go test ./test/unit/architecture/... -count=1
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "layer import check: OK"
