# xQuakShell - Wails SSH Client
# Run `make help` for the target list. Works on Windows and Linux.

.DEFAULT_GOAL := help

.PHONY: help build dev clean rebuild portable install check test test-go test-frontend coverage lint sec deps-linux require-wails

# ---------------------------------------------------------------------------
# Host detection
#
# OS=Windows_NT is set by Windows itself, but it is also set under MSYS2 and
# Git Bash, where POSIX tools are available and cmd.exe builtins are not. So we
# detect two independent things: the target OS, and which shell dialect make
# will use for recipes.
#
# The shell probe relies on quote handling alone - sh strips the quotes, cmd.exe
# keeps them. It runs no external command and redirects nothing, so it cannot
# print an error on the platform it is not testing for: /dev/null and NUL are
# each invalid on the other side, and a missing `uname` is noisy too.
# ---------------------------------------------------------------------------
ifeq ($(shell echo "probe"),probe)
    HOST_SHELL := posix
else
    HOST_SHELL := cmd
endif

ifeq ($(OS),Windows_NT)
    HOST_OS := windows
else
    # Only reachable on a real POSIX host, where uname is always present.
    HOST_OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
endif

# detect returns the path to a binary, or empty if it is not on PATH.
# `where` prints every match, so keep only the first one.
ifeq ($(HOST_SHELL),cmd)
    detect = $(firstword $(shell where $(1) 2>NUL))
else
    detect = $(firstword $(shell command -v $(1) 2>/dev/null))
endif

WAILS_BIN := $(call detect,wails)

# Pinned to the Wails CLI version the release workflow installs.
WAILS_VERSION := v2.12.0

# Versions are pinned (never @latest) so local runs and CI agree.
# Keep in sync with .github/workflows/security.yml.
GOVULNCHECK_VERSION := v1.6.0
GOSEC_VERSION       := v2.28.0
STATICCHECK_VERSION := v0.7.0

# ---------------------------------------------------------------------------
# Build flags
#
# Linux ships two incompatible WebKit generations. Mirrors the release.yml
# matrix, so `make build WEBKIT=4.1` reproduces the 4.1 CI build.
# ---------------------------------------------------------------------------
WEBKIT ?= 4.0

ifeq ($(HOST_OS),linux)
ifeq ($(WEBKIT),4.1)
    BUILD_TAGS := -tags webkit2_41
endif
endif

BUILD_FLAGS := $(BUILD_TAGS) $(EXTRA_BUILD_FLAGS)

help:
	@echo xQuakShell make targets - host $(HOST_OS)/$(HOST_SHELL)
	@echo build - full Wails build, frontend + Go to a binary
	@echo dev - run in dev mode with hot reload
	@echo clean - remove build/ and frontend/dist, asks for confirmation
	@echo rebuild - clean + build
	@echo portable - build + bundle the WebView2 runtime, Windows only
	@echo install - install frontend dependencies
	@echo check - architecture gates, layers and boundaries and budgets
	@echo test - check + Go and frontend test suites
	@echo coverage - per-package coverage floors for the plugin stack
	@echo lint - staticcheck
	@echo sec - govulncheck + gosec
	@echo deps-linux - print the system packages a Linux build needs
	@echo WEBKIT=4.1 - build against libwebkit2gtk-4.1 on Linux
	@echo NOTE - clean and rebuild delete all of build/, including the portable
	@echo data directory build/bin/data with its vault, audit db and plugins.
	@echo Both prompt before deleting. FORCE=1 skips the prompt.

# ---------------------------------------------------------------------------
# Tool guards - fail with an actionable message instead of "command not found"
# ---------------------------------------------------------------------------
require-wails:
ifeq ($(WAILS_BIN),)
	@echo ERROR: the wails CLI was not found on PATH.
	@echo Install it with: go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)
	@exit 1
endif

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
build: require-wails
	wails build $(BUILD_FLAGS)

dev: require-wails
	wails dev $(BUILD_FLAGS)

rebuild: clean build

# `cd x && y` is valid in both cmd.exe and sh, so this needs no branching.
install:
	cd frontend && npm install

# Deliberately not a plain rm: build/bin/data holds the portable-mode vault,
# audit database and installed plugins, so this prompts for an explicit "yes".
# FORCE=1 skips the prompt for non-interactive use.
clean:
	@go run ./scripts/clean $(if $(FORCE),--force,)

# WebView2 is a Windows-only runtime; on Linux WebKit comes from the system.
portable:
ifeq ($(HOST_OS),windows)
	@$(MAKE) build
	@echo Downloading the WebView2 fixed runtime for portable distribution...
	@powershell -NoProfile -ExecutionPolicy Bypass -File scripts/download_webview2.ps1
else
	@echo ERROR: portable bundles the Windows-only WebView2 runtime.
	@echo On $(HOST_OS), use: make build
	@exit 1
endif

# ---------------------------------------------------------------------------
# Gates
#
# The architecture rules live in Go so they run anywhere Go runs; they used to
# be PowerShell scripts that only worked on Windows.
# ---------------------------------------------------------------------------
check:
	go test ./test/unit/architecture/... -count=1

test: check test-go test-frontend

test-go:
	go test ./... -race -count=1

test-frontend:
	cd frontend && npm test

coverage:
	go run ./scripts/coverage

# Both targets type-check the whole module, which needs frontend/dist to exist
# for the go:embed in main.go. Run `make install && cd frontend && npm run build`
# (or `make build`) first on a clean tree.
lint:
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

sec:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...
	go run github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION) -exclude-dir=test/fixtures -exclude-dir=frontend -exclude-dir=build -exclude=G104,G304,G103 ./...

# Printed rather than executed: a build target must not run a privileged
# installer. Mirrors the apt packages installed by .github/workflows/release.yml.
deps-linux:
	@echo Linux build dependencies:
	@echo   sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev
	@echo For WEBKIT=4.1 builds, use libwebkit2gtk-4.1-dev instead.
