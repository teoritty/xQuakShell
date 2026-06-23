# Example Echo Plugin

Reference plugin for Phase 1 IPC, commands, views, and status bar contributions.

## Build

From the repository root:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o example-echo.exe ./plugins/example-echo
```

Copy `plugin.json` and `ui/` alongside the binary, or use `xqs-plugin pack`.

## Features

- Command `echo.ping` (activation: `onCommand:echo.ping`)
- WebView panel `echo.panel` with external `app.js` (CSP-safe)
- Responds to host `ping` RPC

## SDK

Uses `github.com/xquakshell/pluginsdk` (see `main.go`).
