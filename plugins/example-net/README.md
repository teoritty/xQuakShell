# Example Net Plugin

Reference plugin for `pluginsdk.NetClient` — all outbound connections go through the core network allowlist.

## Build

```bash
cd plugins/example-net
go build -o example-net.exe .
```

## Manifest

Declares `tcp:127.0.0.1:9` in `capabilities.network.outbound` (allowlist mode).

For plugins that dial user-chosen hosts at runtime, use arbitrary outbound instead:

```json
"network": {
  "allowArbitraryOutbound": true
}
```
