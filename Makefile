# xQuakShell - Wails SSH Client
# Run `make help` for the target list. Works on Windows and Linux.

.DEFAULT_GOAL := help

.PHONY: help build dev clean rebuild portable install check gates test test-go test-frontend typecheck-frontend coverage mutate lint sec gosec govulncheck budgets-update deps-linux require-wails

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
	@echo gates - every pre-merge gate CI enforces, in CI order
	@echo test - check + Go and frontend test suites
	@echo coverage - per-package coverage floors for the plugin stack
	@echo mutate - mutation testing, slow, runs nightly in CI
	@echo budgets-update - re-record both halves of the size baseline
	@echo lint - staticcheck
	@echo sec - govulncheck + gosec
	@echo gosec - static security analysis, also part of gates
	@echo govulncheck - reachable-vulnerability scan, needs network
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

# gates is what CI enforces, in the order CI runs it, so a green local run means
# a green pipeline. Keep it in step with .github/workflows/test.yml and the `go`
# job of security.yml; `test` is the shorter loop for while you work.
#
# gosec is here and govulncheck is not, and the split is the point. gosec is a
# static analyser over the tree you already have, so once its module is in the
# build cache it costs a minute and no network. govulncheck has to fetch the
# advisory database on every run, which is a network round trip in front of
# every local gate and a failure with no internet - security.yml runs it.
#
# This split exists because the security workflow was red on main for a day
# without anyone able to see it locally: `make sec` was the only way to find
# out, and nothing ran it. A gate CI enforces that a developer cannot run is a
# gate that reports failures late and to the wrong person.
#
# One divergence this does NOT close: gosec analyses the files its build tags
# select, so a run on Windows sees `*_windows.go` and CI on ubuntu-latest sees
# the linux half. The two are complementary rather than identical, and a finding
# in a linux-only file will still surface first in CI.
gates: check typecheck-frontend test-go test-frontend coverage lint gosec

test-go:
	go test ./... -race -count=1

test-frontend:
	cd frontend && npm test

typecheck-frontend:
	cd frontend && npm run check

coverage:
	go run ./scripts/coverage

# The baseline has two owners: this side records the Go numbers, frontend/ records
# the .ts and .svelte ones, and each carries the other's entries through. Running
# only one half is safe (the Go updater refuses to write a baseline that would drop
# an entry it does not own) but it is still half a job, so re-record with one command.
budgets-update:
	go run ./scripts/budgets -update
	cd frontend && npm run budgets:update

# Not part of `gates`: a mutation run reruns a test suite once per mutant and
# takes tens of minutes. .github/workflows/mutation.yml runs it nightly; this
# target is for reproducing a nightly failure locally.
mutate:
	go run ./scripts/mutate
	cd frontend && npm run mutate

# Both targets type-check the whole module, which needs frontend/dist to exist
# for the go:embed in main.go. Run `make install && cd frontend && npm run build`
# (or `make build`) first on a clean tree.
lint:
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

# sec is the full security pass; `gates` runs the gosec half of it. Split into
# two targets so that half can be depended on without dragging the advisory
# database fetch along with it.
sec: govulncheck gosec

govulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

# The flags must stay byte-identical to the gosec step in .github/workflows/security.yml.
# That workflow gates on zero findings from the same invocation, so any drift here
# turns this target into a green light for a red pipeline - which is exactly the
# failure it was added to prevent. The three excluded rules are justified at
# length in security.yml; do not restate the reasoning in two places.
gosec:
	go run github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION) -exclude-dir=test/fixtures -exclude-dir=frontend -exclude-dir=build -exclude=G104,G304,G103 ./...

# Printed rather than executed: a build target must not run a privileged
# installer. Mirrors the apt packages installed by .github/workflows/release.yml.
deps-linux:
	@echo Linux build dependencies:
	@echo   sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev
	@echo For WEBKIT=4.1 builds, use libwebkit2gtk-4.1-dev instead.
