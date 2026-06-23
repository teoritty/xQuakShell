# xQuakShell - Wails SSH Client
# Usage: make [target]
#   make build          - full Wails build (frontend + Go -> exe)
#   make dev            - run in dev mode
#   make clean          - remove build artifacts
#   make rebuild        - clean + build
#   make portable       - build + download WebView2 for Windows portable
#   make install        - install frontend deps

.PHONY: build dev clean rebuild portable install plugins plugin-cli plugin-pack test-plugin bench-plugin check-imports

# Build reference plugins (Phase 1/3)
plugins:
	go build -ldflags="-s -w" -trimpath -o plugins/example-echo/example-echo.exe ./plugins/example-echo
	go build -ldflags="-s -w" -trimpath -o plugins/demo-terminal/demo-terminal.exe ./plugins/demo-terminal
	go build -ldflags="-s -w" -trimpath -o plugins/example-events/example-events.exe ./plugins/example-events
	go run ./cmd/xqs-plugin checksums -dir plugins/example-echo
	go run ./cmd/xqs-plugin checksums -dir plugins/demo-terminal
	go run ./cmd/xqs-plugin checksums -dir plugins/example-events

test-plugin:
	go test ./test/unit/plugin/... -race -count=1 -cover

bench-plugin:
	go test ./test/bench/... -bench=. -benchmem -benchtime=100ms

check-imports:
	powershell -File scripts/check-imports.ps1

# Plugin developer CLI (Phase 6)
plugin-cli:
	go build -ldflags="-s -w" -trimpath -o build/xqs-plugin.exe ./cmd/xqs-plugin

# Pack example-echo as reference bundle
plugin-pack: plugin-cli plugins
	cd plugins/example-echo && ../../build/xqs-plugin.exe pack -o example-echo.xqs-plugin

# Default target: full Wails build
build: plugins
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
