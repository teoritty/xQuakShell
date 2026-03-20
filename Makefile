# xQuakShell - Wails SSH Client
# Usage: make [target]
#   make build          - full Wails build (frontend + Go -> exe)
#   make dev            - run in dev mode
#   make clean          - remove build artifacts
#   make rebuild        - clean + build
#   make portable       - build + download WebView2 for Windows portable
#   make portable-linux - build + download guacd for Linux portable
#   make install        - install frontend deps

.PHONY: build dev clean rebuild portable portable-linux install guacservice-windows-poc

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

# Portable build (Linux): build + download guacd
portable-linux: build
	@echo "Setting up guacd for portable distribution..."
	@bash scripts/download_guacd_linux.sh

# Install frontend dependencies
install:
	cd frontend && npm install

# [POC] Build guacamole-server-windows fork for embedded RDP research.
# Not included in the main build; run manually to evaluate feasibility.
guacservice-windows-poc:
	@echo "=== guacamole-server-windows PoC ==="
	@echo "This target clones and builds the ofiriluz/guacamole-server-windows fork."
	@echo "Prerequisites: Visual Studio 2019+, CMake, vcpkg."
	@echo ""
	@echo "Steps:"
	@echo "  1. git clone https://github.com/ofiriluz/guacamole-server-windows.git build/poc/guacamole-server-windows"
	@echo "  2. cd build/poc/guacamole-server-windows"
	@echo "  3. cmake -B build -DCMAKE_TOOLCHAIN_FILE=<vcpkg-root>/scripts/buildsystems/vcpkg.cmake"
	@echo "  4. cmake --build build --config Release"
	@echo "  5. Copy guacservice.exe to build/bin/guacd/ and test protocol compatibility."
	@echo ""
	@echo "See docs/POC_GUACD_WINDOWS.md for details."
