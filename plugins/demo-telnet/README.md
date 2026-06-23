# Demo Telnet Plugin

Reference plugin for ADR-004: legacy telnet handling moved out of core into an out-of-process plugin.

## What it does

- Contributes the `telnet` connection protocol.
- Uses `per-session` isolation and terminal capabilities.
- Echoes keyboard input in the terminal panel (demo only — not a real telnet client).

## Build

From the repository root:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o demo-telnet.exe ./plugins/demo-telnet
```

Copy the binary next to `plugin.json` under `data/plugins/<id>/` or ship it in `plugins/demo-telnet/` beside the executable for bundled installs.

## Usage

1. Build the plugin binary.
2. Install or place the plugin directory with `SHA256SUMS`.
3. Create a connection with protocol **Telnet (demo)**.
4. Open a session — the core routes it through `PluginSessionBridge`.

## Security

- No vault secret access.
- Terminal plugins require `isolation: per-session` (enforced by manifest validation).
