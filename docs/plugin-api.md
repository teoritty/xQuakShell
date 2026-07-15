# xQuakShell Plugin API Reference

This document describes the plugin IPC contract, bundle format, security limits, and session lifecycle implemented by the xQuakShell core.

## Prerequisites

- Go 1.25+
- `CGO_ENABLED=0` for portable static plugin binaries on Windows

## Project layout

```
my-plugin/
  plugin.json       # manifest (required)
  my-plugin.exe     # go-binary entry (built artifact)
  ui/               # optional WebView assets
  data/             # runtime data (created by core)
```

Plugins communicate with the core via **JSON-RPC 2.0 over NDJSON** on stdin/stdout. Implement the handlers listed in [Plugin IPC reference](#plugin-ipc-reference).

## Manifest

See [plugin-manifest.md](./plugin-manifest.md) for the full schema.

Required fields: `id`, `name`, `version`, `engine.type` (`go-binary`), `engine.entry`.

## Bundles (`.xqsp`)

A bundle is a ZIP archive containing:

- `plugin.json`
- plugin binary and assets
- `SHA256SUMS` (SHA-256 hashes of all files except the checksums file itself)

Install via **Settings → Plugins → Install folder…** or **Install bundle…** (`.xqsp` file).

Installed plugins are copied to `data/plugins/<id>/` under the xQuakShell executable directory (ADR-006 portable layout).

## Security limits

- **IPC frames:** NDJSON lines are capped at **256 KiB**; oversize frames are rejected.
- **Filesystem reads:** `fs.read` returns at most **256 KiB** per call; use `offset` to read larger files (max **16 MiB** total per file).
- **Filesystem writes:** `fs.write` accepts at most **256 KiB** per chunk; omit `offset` to replace a file, or set `offset` for chunked writes.
- **Network:** outbound patterns must be explicit `tcp:hostname:port` — wildcards in the host are rejected. Set `allowArbitraryOutbound: true` in the manifest for session plugins that dial user-chosen hosts (telnet, custom protocols); install requires an explicit consent checkbox. Optional `allowPrivateNetworks: true` extends arbitrary mode to LAN/loopback. Dial failures use generic error codes (no host leakage).
- **Vault IDOR:** plugins may access a connection only while they own an active session for that connection.
- **Secrets:** `vault.getSecret` requires install-time user consent plus manifest declaration. Passphrases require an encrypted identity and an unlocked session cache.
- **Manual start:** `StartPluginManual` requires `onManual` in `activationEvents`.
- **Plugin filesystem:** plugin `fs.*` RPC calls remain manifest-sandboxed. The host Local Files browser uses `HostFileSystem` (ADR-007) and is not available to plugins.

See [security-model.md](./security-model.md) for the full threat model.

## Signing (optional)

1. Generate an Ed25519 publisher key pair (base64-encoded private key file).
2. Write `SHA256SUMS` covering all plugin files (normalized LF line endings).
3. Sign the manifest: Ed25519 over canonical JSON `{manifest, checksumsSha256}` where `checksumsSha256` is the hex SHA-256 of normalized `SHA256SUMS` bytes. Store the signature in `plugin.json` → `signature`.
4. Pack files into a `.xqsp` ZIP archive including `SHA256SUMS`.
5. Add the publisher public key (base64 Ed25519) to **Settings → Plugins → Trusted publishers** in xQuakShell.

## IPC overview

Plugins run out-of-process and communicate via **JSON-RPC 2.0 over NDJSON** on stdin/stdout.

| Kind | Has `id` field | Response expected |
|------|----------------|-------------------|
| **Request** | yes | `result` or `error` |
| **Notification** | no | none |

**Limits and timeouts (as implemented today):**

| Limit | Value |
|-------|-------|
| Max NDJSON frame size | 256 KiB |
| Host → plugin RPC timeout (`initialize` excepted) | 5 s |
| Host → plugin `initialize` timeout | 10 s |
| Host → plugin `shutdown` timeout | 2 s |
| Grace period after stdin close | 3 s |
| Plugin → core recommended client timeout | 5 s |
| Plugin → host `channel.open` timeout | 10 s |
| `channel.close` grace period | 0 s (immediate, no flush window) |
| Max binary channel frame (`kind=0x02`) | 1 MiB |

**Shutdown sequence (host → plugin):**

1. Notification `deactivate` (no response)
2. RPC `shutdown` (plugin returns `{"ok":true}`; 2 s timeout)
3. Stdin closed; process exit or force-kill after grace period

Session plugins must declare every contributed protocol in `capabilities.session.connectProtocols` (see [plugin-manifest.md](./plugin-manifest.md#capabilities)).

Every plugin → core RPC passes through a manifest-driven capability gate. Denied calls return `-32001` and are audit-logged. See [security-model.md](./security-model.md#capability-gate).

## Session plugin lifecycle

This section describes the runtime path for plugins with `capabilities.session.terminal: true` and `isolation: per-session` (required for terminal plugins unless `allowMultiSession` is explicitly set — see [plugin-manifest.md](./plugin-manifest.md#capabilities)).

For **embed session surfaces** (VNC/RDP-style canvas clients), see [Session embed lifecycle](#session-embed-lifecycle) below.

### Sequence

1. User opens a session tab → host sets UI state to `connecting`.
2. Host creates a terminal input bridge (`session.writeInput` notifications) and an output channel (`session.writeTerminal` RPC).
3. Host starts the per-session plugin process and sends `initialize`, then `activate` with reason `onProtocol:<protocolId>`.
4. Host binds the session for IDOR checks, then sends **`session.connect` as a synchronous RPC** (5 s timeout).
5. Plugin returns `{"accepted":true}` quickly. Long-running work (TCP dial, protocol handshake) must run in a goroutine.
6. Plugin calls `session.updateState` with `"ready"` when the terminal stream is usable.
7. Host starts streaming output to the UI (`TerminalOutput` Wails event). The terminal panel is shown only when session state is `ready`.
8. Keyboard input: UI → `session.writeInput` notification → plugin.
9. Window resize: UI → `session.resize` notification → plugin.
10. Tab close: host sends `session.disconnect` notification, **synchronously closes every channel bus channel bound to the session** (see [Channel bus](#channel-bus) below — this runs before the SSH client itself closes, since `exec`-purpose channels ride that client), then stops the per-session process.

On crash, the supervisor restarts the process (up to **3** attempts, exponential backoff from 200 ms), sends `activate` with reason `crash-recovery`, re-binds the session, and **re-sends `session.connect`** with freshly resolved `fields`. The UI shows `connecting` with message “Recovering from plugin crash”. If recovery fails, state becomes `error` (“Plugin process crashed (recovery failed)”).

### `session.connect` contract

Host sends (field names match JSON tags in the core):

```json
{
  "sessionId": "...",
  "connectionId": "...",
  "protocol": "my-protocol",
  "host": "example.com",
  "port": 23,
  "username": "optional-connection-username",
  "fields": {
    "myField": "value"
  }
}
```

- `host` / `port` come from the saved connection. When `port` is 0, the host uses `defaultPort` from the plugin’s `connectionProtocols` entry.
- `fields` contains only keys declared in the manifest for that protocol; undeclared keys are rejected before the RPC is sent.
- Secret field values are resolved in memory by the host; plugins must not call `vault.getSecret` for manifest-declared connection fields.

Plugin must respond with `{"accepted":true}`. If the RPC returns an error, the session transitions to `error`.

### `session.updateState` values

Plugin → host (`capabilities.session.terminal: true` required):

| `state` | Effect |
|---------|--------|
| `"connecting"` | UI stays on connecting screen |
| `"ready"` | UI shows terminal; output stream attached |
| `"error"` | UI shows error; optional `error` string → `errorMessage` |

Calling `session.writeTerminal` before `"ready"` buffers output in the host (channel depth 128), but the user does not see the terminal until state is `ready`.

### Terminal I/O (as implemented)

**Host → plugin (notifications):**

| Method | Params |
|--------|--------|
| `session.writeInput` | `{"dataBase64":"<bytes>"}` — **no `sessionId`**. Input is batched up to **512 bytes** or **8 ms**. |
| `session.resize` | `{"cols":80,"rows":24}` — **no `sessionId`**. Pending input is flushed before resize. |
| `session.disconnect` | `{"sessionId":"<id>"}` |

With `isolation: per-session`, each OS process serves one session, so `sessionId` is omitted from `writeInput` / `resize`. Store `sessionId` from `session.connect` and pass it to `session.writeTerminal` / `session.updateState`.

**Plugin → host (RPC):**

| Method | Params |
|--------|--------|
| `session.writeTerminal` | `{"sessionId":"...","outputBase64":"<bytes>"}` |
| `session.updateState` | `{"sessionId":"...","state":"ready","error":"optional"}` |

If the UI consumer does not read terminal output within **2 seconds**, `session.writeTerminal` returns rate-limited (`-32003`).

### Minimal session plugin pattern

On `session.connect`, return `{"accepted":true}` immediately and perform I/O in a goroutine:

1. Dial the target via `net.dial` (plugin → core RPC).
2. Stream bytes with `net.read` / `net.write` and `session.writeTerminal`.
3. Handle `session.writeInput` notifications (host sends only `dataBase64`; capture `sessionId` from `session.connect`).
4. Call `session.updateState` with `"ready"` when the terminal stream is usable, or `"error"` on failure.

Example `session.connect` handler response:

```json
{"accepted": true}
```

Example `session.updateState` request (plugin → core):

```json
{"sessionId": "...", "state": "ready"}
```

Requires `capabilities.network` (allowlist or `allowArbitraryOutbound`) and `capabilities.session.terminal: true`. See [Network API](#network-api).

## Session embed lifecycle

Embed sessions replace the terminal with a **full-tab iframe** and a **binary WebSocket tunnel** terminated in the host (Mode A — core embed broker). The Go plugin stays a dumb TCP relay; protocol decode (noVNC, ironrdp-web, etc.) runs in `ui/` inside the iframe.

Requires `capabilities.session.embed: true`, `isolation: per-session`, and `contributions.connectionProtocols[].embedEntry` (default `ui/embed.html`). **`terminal` and `embed` are mutually exclusive.**

### Sequence

1. User opens a session tab → host state `connecting`.
2. Host starts the per-session plugin, `initialize`, `activate` (`onProtocol:<id>`), binds session, sends **`session.connect`** (same payload as terminal plugins).
3. Plugin returns `{"accepted":true}` and dials the target in a goroutine (`net.dial`).
4. Plugin calls **`session.registerEmbed`** → host mints a session-scoped token and URLs.
5. Plugin calls **`session.tunnelOpen`** for each tunnel id (typically `"main"`).
6. Plugin starts dumb relay: TCP ↔ `session.tunnelFrame` / `session.tunnelData`.
7. Plugin calls **`session.updateState`** with `"ready"`.
8. Host emits **`SessionEmbedReady`** to the UI with `uiUrl`, `tunnelUrl`, `sandbox`.
9. Frontend mounts `SessionEmbedPanel`: iframe loads `uiUrl`; embed page opens WebSocket to `tunnelUrl`.
10. Viewport: host sends **`embed.viewport`** postMessage (pixels + `devicePixelRatio`); optional **`session.embedViewport`** notification to plugin.
11. Tab blur: **`embed.suspend`**, **`ReportEmbedActivity(false)`**, broker backpressure. Tab focus: **`embed.resume`** + full viewport report.
12. Tab close: `session.disconnect`, token revoked, tunnels closed.

Crash recovery re-sends `session.connect`; plugin must call `session.registerEmbed` again (new token). UI remounts iframe on `SessionEmbedReady`.

### `session.registerEmbed`

Plugin → host (`capabilities.session.embed` required):

```json
{
  "sessionId": "...",
  "uiEntry": "ui/vnc.html",
  "tunnelIds": ["main"]
}
```

Host response:

```json
{
  "embedToken": "<64-hex>",
  "uiUrl": "/embed/s/<token>/ui/index.html",
  "tunnelUrl": "/embed/s/<token>/tunnel/main",
  "expiresAt": "2026-07-03T20:00:00Z"
}
```

- `uiEntry` must match the manifest `embedEntry` for the connection protocol.
- Re-registering for the same session invalidates the previous token.

### Tunnel IPC

| Direction | Mechanism |
|-----------|-----------|
| Plugin → browser | RPC `session.tunnelFrame` (`dataBase64`, optional `eof`) |
| Browser → plugin | Notification `session.tunnelData` (`sessionId`, `tunnelId`, `dataBase64`) |
| Backpressure | Notification `session.tunnelBackpressure` / `session.tunnelResume` |

Limits: max frame **64 KiB**; default aggregate **32 MiB/s** per session; max **4** tunnels per session. Oversized or rate-limited frames return `-32003`.

Other plugin → host RPC: `session.tunnelOpen`, `session.tunnelClose`. **`session.writeTerminal` is not used.**

### Host → plugin notifications (embed)

| Method | When |
|--------|------|
| `session.embedViewport` | Container resized or tab re-activated (`widthPx`, `heightPx`, `devicePixelRatio`, `active`) |
| `session.embedActivity` | Tab focus changed (`active`) |
| `session.tunnelData` | Bytes from browser WebSocket |
| `session.tunnelBackpressure` | WS consumer slow or tab inactive |
| `session.tunnelResume` | Backpressure cleared |
| `session.disconnect` | Tab closed |

**`session.resize` (`cols`/`rows`) is terminal-only.** Embed clients must listen for host → iframe postMessage:

```json
{ "source": "xquakshell-host", "type": "embed.viewport", "widthPx": 1280, "heightPx": 720, "devicePixelRatio": 1.25 }
{ "source": "xquakshell-host", "type": "embed.suspend" }
{ "source": "xquakshell-host", "type": "embed.resume" }
```

Embed pages must **not** disconnect the WebSocket on suspend — pause rendering only.

### Minimal embed plugin pattern

On `session.connect`, return `{"accepted":true}`, dial the target with `net.dial`, then:

1. RPC `session.registerEmbed` with `sessionId`, `uiEntry`, `tunnelIds`.
2. RPC `session.tunnelOpen` for each tunnel id.
3. Relay TCP ↔ `session.tunnelFrame` (plugin → browser) and handle `session.tunnelData` notifications (browser → plugin).
4. RPC `session.updateState` with `"ready"`.

Register handlers for host notifications: `session.tunnelData`, `session.tunnelBackpressure`, `session.tunnelResume`, `session.embedViewport`, `session.embedActivity`.

Requires `capabilities.session.embed`, `capabilities.network`, and static assets under `ui/`.

### Mode B — local embed server (opt-in)

`capabilities.session.localEmbedServer: true` allows `session.reportLocalEmbed` (loopback HTTP in the plugin). **Not recommended for VNC/RDP**; install requires separate consent. Mode A (core broker) is the default and reference path.

See [ADR-008](./adr/008-session-embed-surfaces.md).

## Channel bus

The channel bus is a **binary duplex peer** to the JSON-RPC control plane, for raw byte streams the JSON-RPC transport isn't a good fit for — remote exec output, relayed TCP/UDP traffic, embed-surface framebuffers. It rides the same stdin/stdout pipe pair as JSON-RPC, multiplexed by a frame header. See [ADR-011](./adr/011-binary-channel-bus.md) for the full design rationale.

### Frame layer

Every message on the wire — JSON-RPC included — is now length-prefixed with a fixed **9-byte header**:

```
[4 bytes: length][1 byte: kind][4 bytes: channelId][payload]
```

- `length` is the payload length only (excludes the 9 header bytes).
- `length` and `channelId` are **big-endian**.
- `channelId` is a `uint32`; the host is the sole allocator, from a monotonic counter that is never reused for the lifetime of a plugin process connection.
- `channelId = 0` is reserved for the JSON-RPC control plane — existing JSON-RPC traffic is otherwise unchanged, just framed instead of newline-delimited.

`kind` values:

| Kind | Meaning | Payload |
|------|---------|---------|
| `0x01` | JSON-RPC | the JSON-RPC message, length-framed instead of newline-framed (256 KiB cap, unchanged) |
| `0x02` | Binary channel data | raw bytes, no JSON, no base64 (1 MiB cap) |
| `0x03` | Credit/window update | fixed 8-byte payload: `[4B channelId][4B credit]`, no subtype byte |

`0x04`–`0x0F` are reserved and forbidden in v1 — any arrival, any invalid `kind`, an oversized `length`, or a `channelId` referencing a channel that isn't open is a protocol violation: the host closes stdio and terminates the plugin process immediately, then runs the existing crash-recovery path (supervisor restart, up to 3 attempts). There is no binary error frame — application-level errors (e.g. an exec command exiting non-zero) travel on `channel.close {reason, message}` over JSON-RPC.

### `channel.open` / `channel.close`

Opening and closing a channel is negotiated over JSON-RPC; only data flows as binary frames.

| Method | Direction | Params | Response |
|--------|-----------|--------|----------|
| `channel.open` | plugin → host | `{"purpose":"...","parentSessionId":"...","hint":"..."}` | `{"channelId":<uint32>}` |
| `channel.close` | either side | `{"channelId":<uint32>,"reason":"...","message":"..."}` | notification (no response) |

- `channel.open` is a synchronous RPC with a **10 s timeout** (like `initialize`, not the standard 5 s RPC timeout — spawning an exec process or dialing a relay is comparably slow setup work).
- `channel.close` has a **0 s grace period** — immediate, no flush window. After sending or receiving `channel.close`, both sides must treat the channel as closed and drop any further frames for that `channelId` as no-ops, not errors. Close is idempotent regardless of which side initiates it or how many times it arrives.
- `purpose` is validated against the plugin's manifest-declared `channel.purposes` (see [plugin-manifest.md](./plugin-manifest.md#capabilities)) by the same capability `Gate` that denies undeclared RPC methods — an undeclared purpose is denied with `-32001` and audit-logged.
- `parentSessionId` is checked against the same session-ownership rule as `vault.getSecret` and the tunnel proxies; a channel can only be opened for a session the plugin currently owns an active binding for.

### Purpose

`purpose` is a closed, host-validated enum. The plugin never gets a raw socket or raw credentials — it asks for a channel whose other end the host itself constructs and owns:

| `purpose` | What the host does |
|-----------|---------------------|
| `exec` | Runs a command over the already-authenticated parent session's SSH connection (an exec channel on the existing `ssh.Client`), streaming stdin/stdout/stderr onto the binary channel. The command must match one of the plugin's manifest-declared `execCommands` argv templates — never a free-form string. |
| `embed-stream` | Wires the channel to the session's video/embed surface (VNC/RDP framebuffer path) for a `capabilities.session.embed` plugin. |
| `tcp-relay` | Dials a target through the existing `TunnelDialProxy` allowlist/dial policy — for cases that are genuinely a fresh TCP dial, not exec. |
| `udp-relay` | Dials a target UDP endpoint directly from the host (`net.DialUDP`), validated against a `udp:`-prefixed allowlist entry (same dial-policy core as `tcp-relay`). SSH has no native UDP forwarding, so this is a direct host→target dial, not tunnelled through the parent SSH chain (matches topologies like mosh: SSH launches `mosh-server`, then UDP flows host↔server directly). Still bound to `parentSessionId` for ownership/lifecycle even though the dial is direct. One UDP datagram maps to exactly one `kind=0x02` frame. |

### Flow control (credit)

Credit-based flow control per channel, at **frame granularity**, carried entirely as binary `kind=0x03` frames — never JSON-RPC (routing credit through channel 0 would head-of-line-block the control plane).

- On `channel.open`, the receiver grants an initial credit: **4 frames** for `exec` / `tcp-relay` / `udp-relay`, **8 frames** for `embed-stream`.
- The sender may have at most `credit` frames in flight, unacknowledged, regardless of how large or small each frame's payload is (each frame still must stay under the 1 MiB `maxFrameSize` ceiling — credit and frame size are independent limits).
- The receiver sends a `kind=0x03` credit-update frame as it drains frames, replenishing the sender's count.
- The host enforces credit limits server-side regardless of what the plugin claims locally; a plugin sending past its granted credit is a protocol violation (fail-fast).

**Exhaustion policy is per-purpose:**

- `exec` / `tcp-relay` / `udp-relay` — the backend **pauses reading from its upstream source** (the SSH exec stdout pipe, the relayed TCP connection, or the UDP socket) once credit hits 0. This is real backpressure, not an unbounded in-process buffer; for `udp-relay`, pausing the socket read lets the OS receive buffer bound and drop excess datagrams, the correct behavior for a lossy transport.
- `embed-stream` — instead of blocking the producer, the host-side buffer **drops the oldest unsent frame** (latest-frame-wins), consistent with the existing embed backpressure model.

**Throughput cap (`maxThroughputKbps`) is enforced, not just declared** — a token-bucket limiter on the write path bounds sustained bytes/sec, independent of and in addition to the frame-count credit window. `0` (manifest field absent) uses the host default, the same numeric default as `maxTunnelBandwidthKbps` (32 MiB/s).

### Session lifecycle coupling

A channel's `parentSessionId` binding is one-directional: the parent session owns the channel's lifetime, never the reverse. When the parent session closes — user closes the tab, SSH connection drops, `session.disconnect`, plugin crash-recovery — the host **synchronously closes every channel bound to that session before tearing down the session object**, and specifically **before the SSH client itself closes** (`exec`-purpose channels ride that client). This is an explicit step in the [session close sequence](#sequence) above, alongside the existing `session.disconnect` notification.

Independently of session close, if the **plugin process** itself exits or crashes, every channel owned by that process is closed and its remote end torn down unconditionally — regardless of whether the parent session is still alive — so a `docker exec` can never outlive its channel across a plugin restart.

Closing one channel has no effect on the parent session or its sibling channels.

## Plugin IPC reference

Complete method list as implemented in the core today.

### Host → plugin

#### Lifecycle RPC (plugin must register handlers)

| Method | Params | Response | Notes |
|--------|--------|----------|-------|
| `initialize` | see below | any JSON | Sent once per process start |
| `ping` | omitted / `null` | any JSON | Used by `PingPlugin` when process is already running |
| `activate` | `{"reason":"<trigger>"}` | any JSON | Not sent by dev `StartPlugin` — only `initialize` |
| `shutdown` | omitted / `null` | `{"ok":true}` recommended | 2 s timeout |
| `session.connect` | see [Session plugin lifecycle](#session-connect-contract) | `{"accepted":true}` | Sync RPC; failure → session error |

**`initialize` params:**

```json
{
  "pluginId": "com.example.plugin",
  "apiVersion": "1.0.0",
  "api": {
    "pluginApi": "1.0.0",
    "capabilities": {
      "vault": { "version": "1.0.0", "features": ["getConnection", "getSecret"] }
    }
  },
  "capabilities": { "...": "copy of manifest capabilities" },
  "dataDir": "<plugin or session data directory>",
  "coreVersion": "1.0.0"
}
```

`apiVersion` is the frozen protocol envelope version (`pluginApi`); `api` is the host's full
versioning descriptor — the envelope version plus every capability's version and feature flags
(ADR-012). The host is the authority: it re-checks the plugin's `requires` against this descriptor
and refuses initialization on any incompatibility. `coreVersion` is informational only.

**Observed `activate` reason values:** `onProtocol:<id>`, `onCommand:<id>`, `onStartup`, `onManual`, `crash-recovery`.

`StartPlugin` (Settings → Start) calls `EnsureRunning` only — it sends **`initialize` without `activate`**. Production triggers use `Activate` / `ActivateForSession`, which send both.

#### Host → plugin notifications

| Method | Params | When |
|--------|--------|------|
| `session.writeInput` | `{"dataBase64":"..."}` | Terminal keyboard input |
| `session.resize` | `{"cols":uint,"rows":uint}` | Terminal window resize |
| `session.disconnect` | `{"sessionId":"..."}` | Session tab closed |
| `session.embedViewport` | `sessionId`, `widthPx`, `heightPx`, `devicePixelRatio`, `active` | Embed container resized / tab activated |
| `session.embedActivity` | `sessionId`, `active` | Session tab focus changed |
| `session.tunnelData` | `sessionId`, `tunnelId`, `dataBase64` | Browser → plugin tunnel bytes |
| `session.tunnelBackpressure` | `sessionId` | Pause TCP read (consumer slow / tab inactive) |
| `session.tunnelResume` | `sessionId` | Resume after backpressure |
| `deactivate` | omitted | Before shutdown |
| `view.postMessage` | `{"panelId":"...","message":<json>}` | UI → plugin WebView panel |
| `event` | `{"channel":"...","payload":<json>}` | Core event bus delivery to subscribers |

#### Other host → plugin RPC

| Method | Params | When |
|--------|--------|------|
| `command.execute` | `{"commandId":"...","args":<json>}` | Contributed command invoked |

### Plugin → host

All methods below require a matching manifest capability unless marked “always allowed”. Responses are typically `{"ok":true}` unless noted.

| Method | Capability | Params | Response |
|--------|------------|--------|----------|
| `log.write` | always | `level`, `message`, optional `fields` map | `{"ok":true}` — max **50/s** |
| `ping` | always | omitted | `{"pong":"ok"}` |
| `fs.read` | `filesystem.read` | `path`, `offset?`, `maxBytes?` | `contentBase64`, `offset`, `totalSize`, `eof` |
| `fs.list` | `filesystem.read` | `path` | `entries[{name,isDir}]` |
| `fs.write` | `filesystem.write` | `path`, `contentBase64`, `offset?` | `{"ok":true}` |
| `net.dial` | `network.outbound` and/or `allowArbitraryOutbound` | `network` (only `"tcp"`), `host`, `port` | `handleId` |
| `net.read` | same as dial | `handleId`, `maxBytes?` | `contentBase64`, `eof` |
| `net.write` | same as dial | `handleId`, `contentBase64` | `{"ok":true}` |
| `net.close` | same as dial | `handleId` | `{"ok":true}` |
| `vault.getConnection` | `vault.readConnectionFields` + active session ownership | `connectionId` | subset: `id`, `name`, `host`, `port`, `protocol`, `folderId` |
| `vault.getSecret` | `vault.getSecret` + install consent | `connectionId`, `field` | `field`, `valueBase64` |
| `session.updateState` | `session.terminal` **or** `session.embed` | `sessionId`, `state`, `error?` | `{"ok":true}` |
| `session.writeTerminal` | `session.terminal` | `sessionId`, `outputBase64` | `{"ok":true}` |
| `session.registerEmbed` | `session.embed` | `sessionId`, `uiEntry`, `tunnelIds?` | `embedToken`, `uiUrl`, `tunnelUrl`, `expiresAt` |
| `session.tunnelOpen` | `session.embed` | `sessionId`, `tunnelId` | `{"ok":true}` |
| `session.tunnelFrame` | `session.embed` | `sessionId`, `tunnelId`, `dataBase64`, `eof?` | `{"ok":true}` |
| `session.tunnelClose` | `session.embed` | `sessionId`, `tunnelId` | `{"ok":true}` |
| `session.reportLocalEmbed` | `session.embed` + `localEmbedServer` | port, pathPrefix, token | `{"ok":true}` |
| `events.subscribe` | `events.subscribe` allowlist | `channel` | `{"ok":true}` |
| `events.publish` | `events.publish` namespace | `channel`, `payload` | `{"ok":true}` — max **100/s** |
| `view.postMessage` | contributed `views` | `panelId`, `message` | `{"ok":true}` |
| `channel.open` | `channel` | `purpose`, `parentSessionId`, `hint` | `channelId` — see [Channel bus](#channel-bus) |
| `channel.close` | `channel` | `channelId`, `reason?`, `message?` | notification, no response |

FS paths must use the `${pluginData}` prefix (see [plugin-manifest.md](./plugin-manifest.md#capabilities)). Symlinks are rejected.

Unknown plugin → core methods return `-32601` (`method not found`).

## Network API

Outbound TCP from plugins always goes through core `net.*` RPC — plugins cannot open sockets directly.

### Manifest

Either declare explicit allowlist patterns **or** arbitrary outbound mode (or both — dial succeeds if either permits the target):

```json
"capabilities": {
  "network": {
    "outbound": ["tcp:127.0.0.1:9"],
    "allowArbitraryOutbound": true,
    "allowPrivateNetworks": true
  }
}
```

- **Allowlist:** each `outbound` entry is `tcp:hostname:port` (no wildcards). Private/LAN/loopback IPs are blocked unless the pattern explicitly allowlists that host.
- **`allowArbitraryOutbound`:** dial any resolvable public host on TCP ports 1–65535 after install-time user consent.
- **`allowPrivateNetworks`:** requires `allowArbitraryOutbound`; also permits loopback, RFC1918, and link-local targets.

See [security-model.md](./security-model.md#network-outbound-ssrf) for SSRF rules.

### RPC usage

| Step | RPC | Direction |
|------|-----|-----------|
| Dial | `net.dial` | plugin → core |
| Read | `net.read` | plugin → core |
| Write | `net.write` | plugin → core |
| Close | `net.close` | plugin → core |

Example `net.dial` request:

```json
{"network": "tcp", "host": "example.com", "port": 23}
```

Example response:

```json
{"handleId": "h1"}
```

| Limit | Value |
|-------|-------|
| Max concurrent handles per plugin process | 8 |
| TCP dial timeout | 10 s |
| TCP write timeout (established connection) | 10 s |
| `net.read` on established connection | Blocks until data, RPC cancel, or close; idle/cancel returns empty bytes (not an error) |
| Max bytes per `net.read` / `net.write` call | 256 KiB |
| Supported network | `"tcp"` only |

Dial policy denial → `-32001`. Dial/transport failure after policy check → `-32603` with generic `"request failed"` (no host/port in the message). Unknown handle on read/write/close → `-32002`.

Session plugins that connect to user-chosen hosts (telnet, custom protocols) typically need `allowArbitraryOutbound: true` and optionally `allowPrivateNetworks: true` for LAN devices.

## Connection fields

Plugin session protocols can declare **connection fields** in `contributions.connectionProtocols[].fields`. The host renders these in the connection editor; plugins never draw that UI and never read the vault directly for those values.

### Data flow

1. **Manifest** — field groups and definitions are parsed once at plugin load and cached (`ProtocolDef`).
2. **UI** — `GetPluginConnectionProtocols` returns field metadata; the user edits values in Connection Details.
3. **Vault** — non-secret values are stored on `Connection.pluginFields`; secret values are stored in `VaultData.pluginSecrets` and referenced by `secret:<connectionId>.<fieldId>`.
4. **Connect** — on `session.connect`, the host resolves secrets in memory and sends a `fields` map to the plugin. Resolved values exist only for the active session RPC; they are not logged.

### Field types

| Type | Stored as | Notes |
|------|-----------|--------|
| `text` | string | Optional `validation.pattern`, length bounds |
| `password` | secret ref | Must set `secret: true`; no default allowed |
| `number` | string (decimal) | Validated as float; optional min/max |
| `select` | string | Must declare `options`; default must match an option |
| `checkbox` | `"true"` / `"false"` | |
| `textarea` | string | Max 1 MiB unless `validation.maxSizeBytes` set |

See [plugin-manifest.md](./plugin-manifest.md#connection-protocol-fields) for the full schema.

### session.connect payload

The host sends (full lifecycle: [Session plugin lifecycle](#session-plugin-lifecycle)):

```json
{
  "sessionId": "...",
  "connectionId": "...",
  "protocol": "telnet",
  "host": "example.com",
  "port": 23,
  "fields": {
    "username": "admin",
    "password": "plaintext-only-in-RPC",
    "terminalType": "vt100",
    "enableLogging": "false"
  }
}
```

SSH connections do not include `fields`. The plugin receives only field IDs declared in its manifest; undeclared keys are rejected before the RPC is sent.

Read field values from the `fields` map in the `session.connect` payload.

## Cross-compilation

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o my-plugin.exe .
```

The core validates the binary matches the host OS at install time.

## UI integration

### Wails methods

- `StartPlugin(pluginId)` — manual start (audit-logged, respects disabled flag, idempotent if already running)
- `SetPluginEnabled(pluginId, enabled)` — toggle plugin; disabling stops the process
- `PingPlugin(pluginId)` — ping RPC (does not auto-start)

### Frontend events

- `PluginContributionsChanged` — refresh merged commands/views/status bar
- `PluginStateChanged` — `{ pluginId, state, sessionId? }` where state is `starting|running|stopped|suspended|crashed`
- `SessionEmbedReady` — `{ sessionId, embed: { uiUrl, tunnelUrl, sandbox } }` when embed registration completes
- `PluginViewMessage` — plugin → host view relay

## RPC error codes

| Code | Meaning | Typical cause |
|------|---------|---------------|
| -32700 | Parse error | Malformed JSON-RPC frame |
| -32603 | Internal error | Dial/transport failure after policy check; proxy unavailable; generic host failure |
| -32602 | Invalid params | Bad JSON shape or missing required fields |
| -32601 | Method not found | Unknown plugin → core method |
| -32001 | Capability denied | Manifest does not declare the capability; session not bound; vault IDOR |
| -32002 | Resource not found | Unknown `net.*` handle ID |
| -32003 | Rate limited | `log.write` (>50/s), `events.publish` (>100/s), terminal backpressure |
| -32004 | Not implemented | Handler returned not-implemented |
| -32005 | Auth provider busy | Too many concurrent `auth.*` attempts for one plugin |
| -32006 | Auth attempt not found | `ErrSessionNotBound` on `auth.*` RPC (invalid or foreign `attemptId`) |
| -32007 | Auth challenge timeout | Keyboard-interactive / OTP round exceeded host timeout |
| -32009 | Incompatible plugin API | Envelope or capability version mismatch at initialize (ADR-012) |
| -32010 | Missing feature | Required capability feature flag not offered by the host (ADR-012) |

### Auth provider (host → plugin)

Requires `capabilities.auth.provider` and install consent for auth provider access. Activation: `onAuthRequest:<authMethodId>`.

| Method | Params | Result |
|--------|--------|--------|
| `auth.prepare` | `{ attemptId, authMethodId, connectionId, fields? }` | `{ publicKeyBlobBase64 }` |
| `auth.answerChallenge` | `{ attemptId, authMethodId, name, instruction, questions[] }` | `{ answers[] }` |
| `auth.sign` | `{ attemptId, authMethodId, dataBase64, algorithms[] }` | `{ signatureBase64, signatureFormat }` |

Private keys never cross the process boundary — only sign requests and public key blobs.

### Tunnel provider (dynamic forward)

Requires `capabilities.tunnel.provider`. Plugin routes local SOCKS5 (or similar) clients; after `tunnel.bind`, user traffic is spliced natively without IPC.

Local and dynamic (`-L`/`-D`) forward rules must bind to loopback (`127.0.0.1` or `::1`); non-loopback bind addresses are rejected at save time.

| Method | Direction | Purpose |
|--------|-----------|---------|
| `tunnel.localAccept` | host → plugin | New local client on dynamic rule (`ruleId`, `providerId`, `localConnId`) |
| `tunnel.localFrame` | host → plugin | Pre-bind bytes from local client |
| `tunnel.localClose` | both | Close local side |
| `tunnel.dial` | plugin → host | Open SSH `direct-tcpip` channel |
| `tunnel.localWrite` | plugin → host | Pre-bind bytes to local client |
| `tunnel.bind` | plugin → host | Hand off to native splice (irreversible) |
| `tunnel.close` | plugin → host | Close unbound SSH channel |
