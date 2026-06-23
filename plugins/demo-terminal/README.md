# Demo Terminal Plugin

Demonstrates a custom connection protocol and plugin session bridge.

## Protocol

Contributes `demo-terminal` connection protocol. When a session connects, the core calls `session.connect` and streams terminal I/O via `session.writeInput` / `session.writeOutput`.

## Build

From the repository root:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o demo-terminal.exe ./plugins/demo-terminal
```

## Isolation

Uses default `per-plugin` process isolation. For `per-session`, set `"isolation": "per-session"` in `plugin.json`.
