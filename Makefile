# xQuakShell - Wails SSH Client
# Usage: make [target]
#   make build          - full Wails build (frontend + Go -> exe)
#   make dev            - run in dev mode
#   make clean          - remove build artifacts
#   make rebuild        - clean + build
#   make portable       - build + download WebView2 for Windows portable
#   make install        - install frontend deps

.PHONY: build dev clean rebuild portable install

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
