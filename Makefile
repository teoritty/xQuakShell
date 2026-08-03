# xQuakShell - Wails SSH Client
# Usage: make [target]
#   make build          - full Wails build (frontend + Go -> exe)
#   make dev            - run in dev mode
#   make clean          - remove build artifacts
#   make rebuild        - clean + build
#   make portable       - build + download WebView2 for Windows portable
#   make install        - install frontend deps
#   make lint           - staticcheck (correctness/simplification)
#   make sec            - govulncheck + gosec (security)

.PHONY: build dev clean rebuild portable install check check-imports check-composition-root check-fs-boundaries check-goroutines check-file-size check-session-manager-boundaries lint sec

# Versions are pinned (never @latest) so local runs and CI agree.
# Keep in sync with .github/workflows/security.yml.
GOVULNCHECK_VERSION := v1.6.0
GOSEC_VERSION       := v2.28.0
STATICCHECK_VERSION := v0.7.0

# Both targets type-check the whole module, which needs frontend/dist to exist
# for the go:embed in main.go. Run `make install && cd frontend && npm run build`
# (or `make build`) first on a clean tree.
lint:
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

sec:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...
	go run github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION) -exclude-dir=test/fixtures -exclude-dir=frontend -exclude-dir=build -exclude=G104,G304,G103 ./...

check: check-imports check-composition-root check-fs-boundaries check-goroutines check-file-size check-session-manager-boundaries

check-imports:
	powershell -File scripts/check-imports.ps1

check-composition-root:
	powershell -File scripts/check-composition-root.ps1

check-fs-boundaries:
	powershell -File scripts/check-fs-boundaries.ps1

check-goroutines:
	powershell -File scripts/check-goroutines.ps1

check-file-size:
	powershell -File scripts/check-file-size.ps1

check-session-manager-boundaries:
	powershell -File scripts/check-session-manager-boundaries.ps1

# Default target: full Wails build
build:
	wails build

# Development mode with hot reload
dev:
	wails dev

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@powershell -NoProfile -Command "if (Test-Path build) { Remove-Item -Recurse -Force build }; if (Test-Path frontend/dist) { Remove-Item -Recurse -Force frontend/dist }"
	@echo "Done."

# Clean + build
rebuild: clean build

# Portable build (Windows): build + download WebView2
portable: build
	@echo "Downloading WebView2 Fixed Runtime for portable distribution..."
	@powershell -NoProfile -ExecutionPolicy Bypass -File scripts/download_webview2.ps1

# Install frontend dependencies
install:
	cd frontend && npm install
