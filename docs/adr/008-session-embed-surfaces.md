# ADR-008: Session Embed Surfaces

## Status

Accepted

## Context

Graphical protocol plugins (VNC, RDP, SPICE) need a full session-tab viewport with a browser-based client (noVNC, ironrdp-web) and a dumb TCP byte relay in the plugin process. Existing `contributions.views` sidebar WebViews are unsuitable: wrong UX slot, opaque-origin sandbox, and no binary tunnel bridge.

## Decision

Add a first-class **Session Embed Surface** with Mode A (default): core-hosted embed broker on the Wails asset server.

| Component | Responsibility |
|-----------|----------------|
| `SessionEmbedPanel.svelte` | iframe + ResizeObserver + tab suspend/resume |
| Embed broker (`/embed/s/{token}/…`) | Same-origin static UI + WebSocket tunnel termination |
| `EmbedTunnelService` | Token mint/revoke, frame routing, backpressure |
| Plugin process | `session.registerEmbed` + dumb TCP ↔ tunnel IPC relay |
| Mode B (`localEmbedServer`) | Opt-in loopback HTTP in plugin; install consent required |

Session surface types are mutually exclusive: `terminal` **or** `embed`. Embed requires `isolation: per-session`. Viewport resize uses pixel `embed.viewport` postMessage, not terminal `session.resize`.

## Consequences

- New manifest flags: `capabilities.session.embed`, `localEmbedServer`, `embedEntry` on connection protocols.
- New plugin RPC: `session.registerEmbed`, `session.tunnelOpen/Frame/Close`, optional `session.reportLocalEmbed`.
- Host notifications: `session.embedViewport`, `session.embedActivity`, `session.tunnelData/Backpressure/Resume`.
- Core version bump to `0.3.0-dev`; plugins declare `"minCoreVersion": "0.3.0"`.
- Mode B may need Windows firewall helper (Phase 4); Mode A needs no new firewall rules.

## Alternatives considered

1. **Plugin-local HTTP server (default)** — Rejected: mixed-content, keyboard focus, firewall prompts on Windows.
2. **Reuse sidebar WebView panels** — Rejected: wrong slot, no tunnel, opaque origin.
3. **Native WebView2 child window** — Rejected: focus and memory issues.
4. **Terminal channel for pixels** — Rejected: wrong abstraction and bandwidth.

## References

- [PLAN-session-embed-surfaces.md](../../PLAN-session-embed-surfaces.md)
- [plugin-api.md](../plugin-api.md)
- ADR-003 — Process isolation
- ADR-007 — Trust boundaries
