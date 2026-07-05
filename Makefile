# xQuakShell - Wails SSH Client
# Usage: make [target]
#   make build          - full Wails build (frontend + Go -> exe)
#   make dev            - run in dev mode
#   make clean          - remove build artifacts
#   make rebuild        - clean + build
#   make portable       - build + download WebView2 for Windows portable
#   make install        - install frontend deps

.PHONY: build dev clean rebuild portable install check check-imports check-fs-boundaries check-goroutines check-file-size check-session-manager-boundaries

check: check-imports check-fs-boundaries check-goroutines check-file-size check-session-manager-boundaries

check-imports:
	powershell -File scripts/check-imports.ps1

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
