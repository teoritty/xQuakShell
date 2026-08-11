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
| ~~Mode B (`localEmbedServer`)~~ | ~~Opt-in loopback HTTP in plugin; install consent required~~ — **removed, see below** |

Session surface types are mutually exclusive: `terminal` **or** `embed`. Embed requires `isolation: per-session`. Viewport resize uses pixel `embed.viewport` postMessage, not terminal `session.resize`.

## Consequences

- New manifest flags: `capabilities.session.embed`, `localEmbedServer`, `embedEntry` on connection protocols.
- New plugin RPC: `session.registerEmbed`, `session.tunnelOpen/Frame/Close`, optional `session.reportLocalEmbed`.
- Host notifications: `session.embedViewport`, `session.embedActivity`, `session.tunnelData/Backpressure/Resume`.
- Core version bump to `0.3.0-dev`; plugins declare `"minCoreVersion": "0.3.0"`.
- Mode B may need Windows firewall helper (Phase 4); Mode A needs no new firewall rules.

## Amendment — Mode B removed

Mode A is unchanged and remains the decision. Mode B — the opt-in `localEmbedServer` loopback HTTP
server inside the plugin process — is removed, along with `session.reportLocalEmbed`.

The `session` capability keeps version 1.0.0. Capability majors are matched exactly, so bumping it
would refuse every plugin that grants `session` until its manifest named the new number — including
plugins that never used Mode B — while catching nothing: a plugin that depended on the feature names
it in `requires.features`, and the per-feature check refuses that at any version. The removal is
recorded by name in `removedFeatures` and `removedSchemaFields` instead.

It was the only path on which a plugin process opened a listening socket of its own, which makes it
incompatible with OS-level plugin isolation: a sandbox that must deny the plugin the network cannot
simultaneously let it serve HTTP, and on Windows an AppContainer blocks loopback by default anyway.
Since every embed plugin already uses Mode A — the host reads the plugin's `ui/` assets and
terminates the browser WebSocket itself — nothing was using the path this removes.

The "Phase 4 Windows firewall helper" above is withdrawn with it; there is no listener to permit.

Note that Mode B never worked: `HandlePluginReportLocalEmbed` returned `ErrLocalEmbedNotSupported`
unconditionally from the moment it was written. The manifest field parsed and the capability gate
allowed the call, so the shape existed end to end, and only the handler refused.

## Alternatives considered

1. **Plugin-local HTTP server (default)** — Rejected: mixed-content, keyboard focus, firewall prompts on Windows.
2. **Reuse sidebar WebView panels** — Rejected: wrong slot, no tunnel, opaque origin.
3. **Native WebView2 child window** — Rejected: focus and memory issues.
4. **Terminal channel for pixels** — Rejected: wrong abstraction and bandwidth.

## References

- [plugin-api.md](../plugin-api.md)
- ADR-003 — Process isolation (no ADR file; see [the index](README.md#numbers-below-007))
- ADR-007 — Trust boundaries
