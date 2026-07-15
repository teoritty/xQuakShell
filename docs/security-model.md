# Plugin Security Model

This document summarizes how xQuakShell constrains out-of-process plugins.

## Session protocols

- Every contributed `connectionProtocols[].id` must appear in `capabilities.session.connectProtocols`.
- `session.connect` is rejected at runtime when the connection protocol is not declared for the target plugin.
- Full session lifecycle and IPC payloads: [plugin-api.md — Session plugin lifecycle](./plugin-api.md#session-plugin-lifecycle).

- **`session.connect.fields`** contains only keys declared in the plugin manifest for that protocol. Undeclared field ids are rejected before the RPC is sent.

- Secret field values are resolved by the host at connect time; plugins must not call `vault.getSecret` for manifest-declared connection fields.

## Connection field secrets
Declarative connection fields replace most ad-hoc secret access for session plugins:

| Concern | Behavior |
|---------|----------|
| Declaration | `secret: true` on field def; `password` type requires `secret: true` |
| Storage | Plaintext secret bytes in `VaultData.pluginSecrets`; connection holds opaque ref `secret:<connId>.<fieldId>` |
| Encryption at rest | Whole vault file encrypted (age + scrypt); same boundary as SSH passwords |
| UI | Secret values never round-trip to the frontend after save |
| Connect | Host resolves secrets once per `session.connect` / reconnect; values passed in RPC `fields` map |
| Audit | `session.connect` logged with field **count** only (ring buffer + existing vault audit patterns); no secret values |
| Validation | Host validates field values on save (required, pattern, options, size, checkbox encoding) |
| Empty secret | Submitting empty string for a secret field removes the stored secret |
| Uninstall | Plugin uninstall blocked while any connection uses one of its protocol ids |

Plugins should treat `fields` as session-scoped credentials: do not persist them under `${pluginData}` unless the user explicitly opts in via a non-secret field.

## Capability gate

Every plugin→core RPC passes through a manifest-driven gate. Methods such as `fs.read`, `vault.getConnection`, `vault.getSecret`, `events.publish`, and `net.dial` require matching entries in `plugin.json` `capabilities`.

Denied calls return `ErrCapabilityDenied` and are audit-logged without secret material. Policy denials (unknown method, disallowed capability, blocked resolved IP) use `-32001`. Transport and dial failures after policy checks use `-32603` without leaking host/port details in plugin-visible messages.

The gate also applies a version check: a method mapped to an above-baseline capability feature is allowed only if the plugin negotiated a version that reaches it (ADR-012). The capability grant remains the authorization boundary; the version check is additive.

## API version handshake (ADR-012)

Plugin API compatibility is negotiated at `initialize`, not trusted from the static manifest. The host advertises its full API descriptor (envelope version + per-capability versions and feature flags) and re-checks the plugin's `requires` against the **live** registry, failing closed on any skew. The host is the sole authority: a plugin's echoed descriptor is never trusted for enforcement. A plugin built against the pre-1.0 API (`minCoreVersion < 1.0.0`) is rejected. Incompatibilities surface as `-32009` (version) / `-32010` (missing feature) and are logged.

## Ownership (IDOR)

Authorization for vault and session data is enforced in the **usecase** layer:

| Resource | Rule |
|----------|------|
| `vault.getConnection` / `vault.getSecret` | Allowed only when `SessionManager` has an active session where `pluginId` and `connectionId` match |
| `view.postMessage` (inbound) | Allowed only for `panelId` values contributed by the same plugin |
| `session.updateState` / `session.writeTerminal` | **Usecase:** `PluginSessionRPCHandler` + `PluginSessionAuthorizer` enforce scope and bound sessions; **Usecase:** `SessionManager` verifies `pluginId` owns the session |

### Terminal isolation policy

- Plugins with `capabilities.session.terminal: true` **must** use `isolation: per-session`. `allowMultiSession` is rejected for terminal plugins.
- Non-terminal plugins may set `allowMultiSession` only with `isolation: per-plugin` and explicit install-time user consent (`multiSessionAccessGranted` in vault settings).
- Install preview shows a **multi-session warning** when `allowMultiSession` is set; install is audit-logged.
- Every session bind/unbind is audit-logged (`session.bind` / `session.unbind`).
- **Per-session:** RPC target `sessionId` must match the process instance session.
- **Per-plugin:** RPC target `sessionId` must be in the host **bound session registry** (populated on `session.connect`, cleared on disconnect).
- Cross-plugin session IDOR is denied in `SessionManager`; cross-session IDOR within the same plugin is denied by scope rules above.

## UI asset sandbox

- Plugin WebView assets are served only from `<pluginRoot>/ui/` with extension allowlisting.
- Binary artifacts (`*.exe`, `plugin.json`, etc.) are not served over HTTP.

## Process resource limits

- **Linux / macOS / BSD:** `RLIMIT_AS`, `RLIMIT_NOFILE`, best-effort `RLIMIT_NPROC` via `Prlimit` / `setrlimit` (128 MiB memory cap, same as Windows Job Object).
- **Windows:** per-process Job Object with `PROCESS_MEMORY` / `JOB_MEMORY` caps (128 MiB) and kill-on-close.
- Exactly one goroutine calls `cmd.Wait()` per plugin child (`processReaper`).

## Secrets (ADR-002)

- Manifest declares allowed fields in `capabilities.vault.getSecret` (no wildcards).

- User must grant access at install time when the plugin requires secrets.

- Grants are stored in vault settings (`secretAccessGranted`).

- `passphrase` is returned only when the identity is encrypted **and** the host `PassphraseCache` holds the user-supplied value for that session.

- Secret values are never written to audit or plugin logs.
- Plugins should use structured `log.write` with a `fields` map. Sensitive field keys (`password`, `secret`, `token`, `key`, …) are stripped at the IPC boundary.
- Free-text `message` values still pass through heuristic redaction as a fallback.

## Process isolation (ADR-003)

- Default: **one process per plugin ID** (`per-plugin`).
- Optional: **one process per session** (`per-session` in manifest).
- Per-session processes receive a **session-scoped `dataDir`**; the FS capability proxy uses the same directory as `initialize.dataDir` (no cross-session file access).
- Windows: child processes are assigned to a Job Object with kill-on-close (startup fails if job object unavailable).
- Linux: `PR_SET_PDEATHSIG`, dedicated process group (`Setpgid`), and tracked PIDs killed on host shutdown.
- Crash recovery: supervisor restarts with exponential backoff (max 3 attempts), sends `activate`, then re-sends `session.connect` while sessions remain active.
- `engine.args` is rejected in v1 to prevent manifest injection.

## Graceful shutdown
1. Core sends `deactivate` as a **notification** (plugins: `RegisterNotification` / `OnDeactivate`).
2. Core sends `shutdown` as an **RPC request** with a short timeout (plugins: `Register` / `OnShutdown`, return `{"ok":true}`).
3. Core closes plugin stdin; if the process has not exited within the grace period, it is force-killed.
4. 
## Activation policy
Plugins start only via declared `activationEvents`:

| Event | Meaning |
|-------|---------|
| `onStartup` | Start when the host starts |
| `onProtocol:<id>` | Start when a connection uses that protocol |
| `onCommand:<id>` | Start when a contributed command runs |
| `onManual` | Start via **Settings → Start plugin** (`StartPluginManual`) |
| `onView:<panelId>` or `onView:*` | Start when a contributed WebView panel is opened |

`PingPlugin` does not auto-start. Disabled plugins are stored in vault settings (`plugins.disabled`) and cannot start until re-enabled.

## WebView sandbox

Plugin UI loads in a sandboxed iframe (`allow-scripts` only). CSP on asset responses allows `script-src 'self'`.

Host↔iframe `postMessage` uses an explicit target origin:

- The host appends `?hostOrigin=<host origin>` to the iframe URL.
- The host sends messages to `*` because the sandboxed iframe has an opaque origin; delivery is scoped to `iframe.contentWindow`.
- Plugin scripts reply to the `hostOrigin` query parameter and ignore messages from other origins.

## Event bus

- Publish: namespace `plugin.<ownId>.*` enforced at manifest validation **and** runtime; `core.*` publish is rejected.
- Subscribe: allowlist — `core.session.*` or explicit `core.session.opened|closed|stateChanged`; broad `core.*` rejected at manifest validation.
- Session events delivered only to plugins with active sessions.
- Rate limit: 100 events/second per plugin.
- Inbound plugin RPC resets the idle-suspend activity timer.

## Channel bus (ADR-011)

Full wire format, `channel.open`/`channel.close`, credit model, and purpose semantics: [plugin-api.md — Channel bus](./plugin-api.md#channel-bus).

- **`exec` requires explicit install-time user consent**, exactly like `auth.provider` and `allowArbitraryOutbound` — running arbitrary commands over the user's already-authenticated session is high-impact and must never be silently grantable. The install preview shows a dedicated consent line ("Run commands over your authenticated session (exec channel)") whenever the manifest declares `exec` in `capabilities.channel.purposes`. `embed-stream` and `tcp-relay`/`udp-relay` require no new consent beyond the existing ADR-008 embed consent and the existing `allowArbitraryOutbound`/`allowPrivateNetworks` network model, respectively.
- **`exec` commands are argv-array allowlists, never shell strings** — see [plugin-manifest.md — `exec` command allowlist](./plugin-manifest.md#capabilities) for the template/regex schema. The host always invokes via an argv array, eliminating command injection as a class of bug rather than relying on escaping.
- **IDOR ownership carries over unchanged:** a channel's `parentSessionId` is checked against the same session-binding/ownership rule that already protects `vault.getSecret` and the tunnel proxies (see [Ownership (IDOR)](#ownership-idor) above) — a plugin may open or hold a channel for a session only while it owns an active binding for it.
- **`tcp-relay`/`udp-relay` audit the canonicalized, post-validation target, never the raw plugin-supplied `hint`** — the same principle already applied to dial-policy audit entries under [Network outbound (SSRF)](#network-outbound-ssrf) below: logging the raw value would let two different encodings of the same address (DNS name vs. IP literal vs. non-canonical IP form) look like different events, or let a bypass attempt look benign in the log.
- **`hint` validation reuses the existing dial policy verbatim** — `tcp-relay` and `udp-relay` are checked through the same allowlist/IP-restriction functions `net.dial` already uses (`tcp:host:port` and `udp:host:port` respectively), not a parallel validator.

## Network outbound (SSRF)
Full `net.*` RPC reference and limits: [plugin-api.md — Network API](./plugin-api.md#network-api).

- **Allowlist mode (default):** manifest `outbound` patterns are validated (`tcp:host:port` only; no wildcards). Host resolves the target before dial; loopback, RFC1918, link-local, and metadata IPs are blocked unless the manifest explicitly allowlists that IP literal. Dial uses the resolved IP address to prevent DNS rebinding between policy check and connect.
- **Arbitrary outbound mode:** when `allowArbitraryOutbound: true`, plugins may dial any resolvable public host on TCP ports 1–65535 after install-time user consent (persisted in vault settings). Private/LAN/loopback addresses remain blocked unless `allowPrivateNetworks: true`. Combined allowlist + arbitrary: a dial succeeds if either mode permits the target.
- Install consent for arbitrary network access is audit-logged alongside other elevated permissions.

## Terminal backpressure

Plugin terminal output is written to a bounded channel. If the UI consumer does not read within **2 seconds**, the host returns `ErrTerminalBackpressure` instead of silently dropping bytes.

## IPC limits

- Maximum NDJSON frame size: **256 KiB**
- Maximum single `fs.read` / `fs.write` chunk: **256 KiB**
- Maximum sandboxed file size (via chunked I/O): **16 MiB**
- FS paths must use `${pluginData}` prefix; resolved roots must stay under plugin install directory.
- Symlinks rejected on FS access.
- 
## Install security

- Zip-slip protected bundle extract (`pathsafe.UnderRoot`).
- Bundled and user-installed plugins validate `SHA256SUMS` when present; hash mismatches hard-reject the plugin. Signed plugins require `SHA256SUMS` — the manifest signature binds to the checksum file digest so tampering a binary and recalculating checksums alone cannot pass verification.
- Protocol ID conflicts rejected at discovery.

## Portable data layout (ADR-006)

All writable plugin and vault storage lives under `<exeDir>/data/`.
**ADR-006 exception:** read-only bundled plugins may ship in `<exeDir>/plugins/` next to the executable (no writes required). This is a deliberate fallback for portable/USB distributions that ship reference plugins without pre-populating `data/plugins/`.

Plugin discovery scans, in order:

1. `<dataRoot>/plugins` (user-installed; writable portable state)
2. `<exeDir>/plugins` (bundled read-only fallback)

User-installed plugins **override** bundled plugins with the same manifest `id`.

## Host trust boundary (ADR-007)

The host application (Wails UI) operates on the user's filesystem **without a sandbox root** via `domain.HostFileSystem`. This is intentional: an SSH client must list, transfer, and open files anywhere the user can access.

| Caller | FS access | Sandboxed |
|--------|-----------|-----------|
| Host UI (Local Files, transfers, dialogs) | `HostFileSystem` | No |
| Portable internal state (temp, layout) | `PortableDataStore` | Yes (`<exe>/data`) |
| Plugin child process (`fs.*` IPC) | `FSProxy` | Yes (manifest `${pluginData}`) |

Plugins **cannot** invoke Wails host methods or `HostFileSystem`. Their only filesystem surface is manifest-gated IPC.

See [adr/007-host-filesystem-trust.md](adr/007-host-filesystem-trust.md).

## Session embed surfaces (ADR-008)

Embed sessions serve plugin `ui/` assets and WebSocket tunnels through a **core-hosted broker** at `/embed/s/{token}/…` (same origin as the Wails app). Plugins do not bind listening HTTP ports by default.

| Threat | Mitigation |
|--------|------------|
| IDOR (another session's embed) | 256-bit token bound to `sessionId`; revoked on close/crash |
| Token guessing | Rate-limit failed `/embed/` lookups |
| Path traversal in UI assets | Token lookup → plugin `ui/` root only; `pathsafe` checks |
| XSS in plugin UI | CSP on embed routes; sandbox `allow-scripts allow-same-origin` |
| Arbitrary external iframe URLs | Host rejects; only registered descriptors |
| Secret in URL | Forbidden; tokens are path segments, not query params with secrets |
| WS hijacking | Token required at tunnel upgrade |
| Memory exhaustion | 64 KiB max frame; 32 MiB/s default bandwidth; inactive tab backpressure |

Mode B (`localEmbedServer`) is opt-in with install consent and loopback-only binding. See [adr/008-session-embed-surfaces.md](adr/008-session-embed-surfaces.md).

Tunnel payload bytes are **not** audit-logged. Control events (`session.embed.register`, `session.embed.revoke`, auth failures) may be logged without secrets.

