# Demo RDP Plugin

Reference plugin for ADR-004: RDP is not implemented in-process in xQuakShell core.

## Approach

Real RDP sessions require a native desktop client (FreeRDP, mstsc, etc.). This reference plugin:

1. Registers the `rdp` connection protocol.
2. Receives `session.connect` with host metadata (no secrets).
3. Reports `session.updateState` = `error` with guidance to launch an external client.

Production RDP plugins may add `capabilities.network.outbound` and spawn or hand off to `mstsc.exe` / `xfreerdp`.

## Build

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o demo-rdp.exe ./plugins/demo-rdp
```

## Usage

1. Build and install the plugin with checksums.
2. Create a connection using protocol **RDP (external)**.
3. Open a session — the plugin activates and documents the external-client workflow.

## Security

- No terminal capability (no in-app RDP surface).
- Uses structured logging via `pluginsdk.LogInfo` (sensitive keys are redacted by the host).
