# Changelog

## v0.2.0 (embed sessions — requires core 0.3.0-dev+)

- `Client.Embed()` — `Register`, `OpenTunnel`, `SendFrame`, `CloseTunnel`
- `RegisterEmbedHostHandlers` — `session.embedViewport`, `session.embedActivity`, `session.tunnelData`, backpressure/resume
- `RunBidirectionalRelay` — TCP handle ↔ tunnel helper (plugin-side)

## v0.1.0

- Initial published SDK aligned with xQuakShell Plugin API 1.0.0
- `Host` / `Client` JSON-RPC loop over stdin/stdout
- Typed helpers for vault, filesystem, and events
- JSON-RPC `-32700` parse error responses on malformed frames
