# Plugin Security Model



This document summarizes how xQuakShell constrains out-of-process plugins.



## Capability gate



Every plugin→core RPC passes through a manifest-driven gate. Methods such as `fs.read`, `vault.getConnection`, `vault.getSecret`, `events.publish`, and `net.dial` require matching entries in `plugin.json` `capabilities`.



Denied calls return `ErrCapabilityDenied` and are audit-logged without secret material. Policy denials (unknown method, disallowed capability, blocked resolved IP) use `-32001`. Transport and dial failures after policy checks use `-32603` without leaking host/port details in plugin-visible messages.



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
- Plugins should use structured `log.write` with a `fields` map (`pluginsdk.LogInfo`). Sensitive field keys (`password`, `secret`, `token`, `key`, …) are stripped at the IPC boundary.
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



## Session protocols



- Every contributed `connectionProtocols[].id` must appear in `capabilities.session.connectProtocols`.

- `session.connect` is rejected at runtime when the connection protocol is not declared for the target plugin.



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



## Network outbound (SSRF)

- Manifest patterns are validated (`tcp:host:port` only; no wildcards).
- Host resolves the target before dial; loopback, RFC1918, link-local, and metadata IPs are blocked unless the manifest explicitly allowlists that IP literal.
- Dial uses the resolved IP address to prevent DNS rebinding between policy check and connect.



## Terminal backpressure



Plugin terminal output is written to a bounded channel. If the UI consumer does not read within **2 seconds**, the host returns `ErrTerminalBackpressure` instead of silently dropping bytes.



## IPC limits



- Maximum NDJSON frame size: **256 KiB**

- Maximum single `fs.read` / `fs.write` chunk: **256 KiB**

- Maximum sandboxed file size (via chunked I/O): **16 MiB**

- FS paths must use `${pluginData}` prefix; resolved roots must stay under plugin install directory.

- Symlinks rejected on FS access.



## Install security



- Zip-slip protected bundle extract (`pathsafe.UnderRoot`).

- **All** plugins (bundled and user-installed) require `SHA256SUMS` at discovery; missing or mismatched checksums hard-reject the plugin.

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

