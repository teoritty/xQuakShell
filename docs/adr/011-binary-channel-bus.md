# ADR-011: Binary Duplex Channel Bus for Plugins

## Status

Proposed

## Context

The current plugin transport is JSON-RPC 2.0 over NDJSON on stdin/stdout, capped at 256 KiB per frame, designed for control-plane traffic (`session.connect`, `resize`, terminal text via `session.writeTerminal`). Three upcoming features need something the current transport cannot serve well:

1. **Graphical session protocols (VNC/RDP)** — framebuffer updates are large, high-frequency, and latency-sensitive. Encoding them as base64-in-JSON adds ~33% size overhead plus JSON parse cost on every frame, and forces chunking around the 256 KiB frame cap.
2. **Discovery plugins (Docker/Kubernetes/DB)** — enumerating and connecting to resources living *behind* an already-authenticated parent connection (e.g., containers on a server reached over SSH) requires running a remote helper process and speaking a binary/streamed protocol to it (`docker system dial-stdio`, `kubectl exec`, a raw Postgres wire connection through a jump host) — not a single request/response RPC call.
3. **Non-TCP transports** — none of this is a `tcp:host:port` dial the existing `TunnelDialProxy`/`NetProxy` model was built for. It's a duplex byte stream riding on top of a channel the *host* already owns (an SSH session, a jump chain), not a fresh network connection the plugin opens itself.

All three are really one problem: **plugins need a raw, bidirectional, binary-safe stream, multiplexed alongside the existing JSON-RPC control channel, whose other end is something the host already has authenticated access to.** Building this three times (once per feature) would produce three incompatible, narrower abstractions instead of one reusable primitive — the opposite of the atomic-API goal.

## Decision

Introduce a single new primitive, the **channel bus**, as a peer to the existing JSON-RPC control plane — not a replacement for it.

### 1. Transport & framing

Multiplex both JSON-RPC and binary channel traffic over the same stdin/stdout pipe pair already used for the plugin process, using a thin length-prefixed frame header in front of every message:

```
[4 bytes: length][1 byte: kind][4 bytes: channel id][payload]
kind = 0x01 JSON-RPC (existing NDJSON message, unchanged)
kind = 0x02 Binary channel data
kind = 0x03 Credit update (backpressure signal only — see Flow control; not used for open/close/error, which stay on the JSON-RPC control plane)
```

`length` counts the payload only (the 5-byte `kind` + `channel id` header is not included). Both `length` and `channel id` are big-endian (network byte order). `channel id` is a `uint32`; the host is the sole allocator (assigned in the `channel.opened` response, never proposed by the plugin) and ids are never reused for the lifetime of a given plugin process connection, even after a channel closes — this rules out a whole class of stale-frame-misrouted-to-a-reused-id bugs. `kind` values `0x04`–`0x0F` are reserved and unused in v1: any frame with a `kind` in that range is a protocol violation and follows the fail-fast path in §2a.

- `channel id = 0` is reserved for the existing JSON-RPC control plane, so **existing plugins and the existing IPC contract are untouched** — this is additive, not a breaking transport change.
- Binary channel frames carry no JSON envelope, no base64 — raw bytes, length-prefixed only.
- The existing 256 KiB cap stays for JSON-RPC frames; binary channel frames get their own, larger, configurable cap (e.g. 1 MiB) since framebuffer tiles and exec output need more headroom.

### 2. Channel lifecycle (control-plane driven)

Opening a raw channel is still negotiated over JSON-RPC — the binary path is *only* for data, never for negotiation. This keeps auditability and the capability gate in one place:

```
plugin → host   channel.open   {purpose, parentSessionId, hint}   (JSON-RPC request)
host   → plugin channel.opened {channelId}                        (JSON-RPC response)
...binary frames flow on channelId until...
either side      channel.close {channelId, reason}                (JSON-RPC notification)
```

`purpose` is an enum the host validates against the plugin's manifest-declared capability (see below) before it ever wires the channel to anything real — a plugin cannot open an arbitrary raw pipe to an arbitrary destination just by asking.

### 2a. Framing robustness — fail-fast, not fail-soft

Both sides of this transport are stdio pipes between the host and a process the host itself spawned — not a network link. Bit-level corruption in transit is not a realistic threat; the realistic threat is a *length-accounting bug* on either side. Attempting stream resynchronization (e.g., scanning for a magic marker to recover mid-stream) adds a second piece of parser logic that itself needs to be at least as correct as the thing it's recovering from, for a failure mode that already indicates a serious bug regardless of whether we "recover" from it.

Decision: **any framing anomaly is fatal for the whole plugin connection, not just the affected channel.** Concretely:
- `length` is validated against a hard per-`kind` ceiling (256 KiB for `kind=0x01`, a separate configurable ceiling for `kind=0x02`) **before** allocating a receive buffer — an attacker/bug cannot force a large allocation via a bogus length field.
- An invalid `kind`, an out-of-range `length`, or a `channel id` referencing a channel that isn't open is treated as a protocol violation: the host closes stdio and terminates the plugin process immediately, then goes through the existing crash-recovery path (supervisor restart, up to 3 attempts, exponential backoff from 200 ms — already implemented for the terminal-session case).
- This is logged loudly and audited as a protocol violation, distinct from an ordinary crash, so it's visible in diagnostics rather than silently retried into a loop.

### 2b. Flow control (backpressure)

Blocking the underlying OS pipe on a full buffer is not viable here, because the pipe is shared by every multiplexed channel *and* the JSON-RPC control plane — a slow consumer on one channel would head-of-line-block control-plane traffic (e.g., a keepalive) for everything else on that plugin connection.

Decision: **credit-based flow control per channel, at frame granularity, carried entirely as binary `kind=0x03` control frames — never JSON-RPC.** Routing credit updates through JSON-RPC on channel 0 would reintroduce exactly the head-of-line-blocking this section exists to avoid, so `windowUpdate` is a fixed-layout binary frame (`channel id` + a 4-byte credit count), not a JSON message.

Credit is measured in **frames, not bytes** — this deliberately decouples flow control from `maxFrameSize` (the 1 MiB-class cap on a single frame's payload, defined in framing/§1) so the two limits can never conflict with each other:

- On `channel.opened`, the receiver grants an initial credit (e.g. 4 frames).
- The sender may have at most `credit` frames in flight, unacknowledged, on that channel — regardless of how large or small each individual frame's payload is, as long as each stays under `maxFrameSize`.
- The receiver sends a `kind=0x03` credit-update frame as it drains/processes frames, replenishing the sender's count.
- **Per-`purpose` semantics differ on credit exhaustion:**
  - `exec` / `tcp-relay` / `udp-relay`: the backend stops **reading from the underlying source** (the SSH exec stdout, the relayed TCP connection, or the UDP socket) when credit is exhausted — this is real backpressure that propagates upstream (e.g., to the remote process via its own stdout buffer filling), not unbounded local buffering inside the backend. A backend that keeps draining its source into an ever-growing in-memory queue while "blocked" reintroduces the exact unbounded-queue failure mode credit-based flow control exists to remove, and is not a correct implementation of this section. For `udp-relay` specifically, suspending the socket read lets the OS receive buffer bound and **drop** excess datagrams — the correct, still-bounded behavior for an inherently lossy transport, as opposed to the upstream-propagating backpressure of the stream purposes.
  - `embed-stream`: instead of blocking the producer, the **host-side buffer drops the oldest unsent frame** when credit is exhausted — consistent with the latest-frame-wins policy already described for video. The plugin is not required to implement drop logic itself; it just stops being able to push once the host stops granting credit, and the host discards what it's already holding.
- The host enforces credit limits server-side regardless of what the plugin claims locally (defense in depth); a plugin that sends past its granted credit is a protocol violation and follows the fail-fast path in 2a.
- **Throughput cap (`maxThroughputKbps`) is enforced, not merely declared.** A manifest field with no corresponding runtime check would be worse than no field at all. Enforcement is a token-bucket limiter applied on the same write path as credit accounting, independent of (and in addition to) the frame-count credit — credit bounds how much can be in flight unacknowledged, the token bucket bounds sustained throughput over time.

### 3. What sits on the other end of a channel — host-mediated, never plugin-dialed

This is the core security property: **the plugin never gets a raw socket or raw credentials.** It asks the host, by capability, for a channel whose other end the host itself constructs and owns:

| `purpose` | What the host actually does |
|---|---|
| `exec` | Runs a command over the **already-authenticated parent session's** SSH connection (an exec channel on the existing `ssh.Client`), pipes stdin/stdout/stderr onto the binary channel. No new auth, no new dial. |
| `embed-stream` | Wires the channel to the session's video/embed surface (VNC/RDP framebuffer path) for a `capabilities.session.embed` plugin. |
| `tcp-relay` | Falls back to the *existing* `TunnelDialProxy` dial policy (host:port allowlist, no wildcards) — for cases that genuinely are a fresh TCP dial, not exec. |
| `udp-relay` | Same dial policy as `tcp-relay`, reused verbatim with a `udp:` allowlist token, but a **direct** host→target UDP dial (`net.DialUDP`) — SSH has no native UDP forwarding, so this is the "genuinely a fresh dial" case only, never tunnelled through the parent SSH chain. Matches real topologies like **mosh** (SSH launches `mosh-server`, then UDP flows host↔server directly). Bound to `parentSessionId` for ownership/lifecycle even though the dial is direct. |

This reuses and extends the pattern already established by `TunnelDialProxy`/`TunnelLocalProxy`/`FSProxy`: the plugin describes *what it wants to happen*, the host — which holds the real credentials and the real session — is the only thing that ever touches them.

## Security model

Security-first means the new primitive must not become a bypass for every gate the existing capability system already enforces. Concretely:

- **New manifest capability, `channel`:**
  ```json
  "capabilities": {
    "channel": {
      "purposes": ["exec", "embed-stream"],
      "maxConcurrent": 4,
      "maxThroughputKbps": 0
    }
  }
  ```
  Declared purposes are allow-listed per plugin, same as `session.connectProtocols` today. A plugin that never declares `"exec"` cannot open one — checked at `channel.open` time by the same `Gate` (`capability/gate.go`) that already denies undeclared RPC methods with `-32001` and audit-logs the denial.

- **`exec` requires explicit install-time consent**, exactly like `auth.provider` and `allowArbitraryOutbound` do today — running arbitrary commands over the user's authenticated session is high-impact and should never be silently grantable.

- **IDOR protection carries over unchanged:** a channel's `parentSessionId` is checked against the same session-binding/ownership rule that already protects `vault.getSecret` and the tunnel proxies ("plugin may act on a session only while it owns an active binding for it"). This is exactly the property your existing `tunnel_local_proxy_idor_test.go` already guards — the new capability should get an equivalent test before it ships, not after.

- **Resource limits, enforced host-side, not trusted from the plugin:** max concurrent channels per plugin, max frame size, and an optional throughput cap (reusing the existing `maxTunnelBandwidthKbps` pattern) — a compromised or buggy plugin should not be able to open unbounded channels or flood the host process.

- **Audit logging:** `channel.open` / `channel.close` go through the same `Audit` port every other capability call does. A raw exec channel is exactly the kind of event you want in the audit log with full context (which plugin, which session, what purpose, when). For `tcp-relay`, the audit entry records the **canonicalized, post-validation** target, not the raw `hint` string — logging the raw value would let two different encodings of the same address (DNS name vs. IP literal vs. non-canonical IP form) look like different events, or worse, let a bypass attempt look benign in the log.

- **`tcp-relay` `hint` validation reuses the existing dial policy verbatim, not a parallel implementation.** `hint` is not a trusted free-form string — it is checked through the *same* function `TunnelDialProxy`/`NetProxy` already use (`net_dial_policy.go` / `network_pattern.go`) against the plugin's manifest-declared allowlist, including the existing `allowPrivateNetworks`/loopback distinction. Building a second, purpose-specific validator for channel `hint` would create exactly the class of bug where two authorization checks quietly drift apart over time — one gets updated, the other doesn't. There is deliberately no new validation code path here, only a new caller of the existing one.

- **No new privilege for the plugin process itself.** The plugin still runs out-of-process, still sandboxed the same way (`process_sandbox.go`, per-OS job objects/process groups). The channel bus changes *what data can flow*, never *what the plugin process itself can touch on disk or network directly*.

Net effect: the attack surface added is "one more capability-gated, audited, consent-required RPC verb" — the same shape as everything else in `capability/`, not a parallel, weaker security model living next to it.

## Application to graphical protocols (VNC/RDP)

A `capabilities.session.embed` plugin for VNC/RDP:
1. Session connects as today (`session.connect`, `session.updateState`).
2. Plugin opens a channel with `purpose: "embed-stream"`.
3. Host wires that channel directly to the embed surface's video pipe.
4. Framebuffer tiles/frames flow as raw binary frames — no JSON, no base64.
5. Backpressure policy for this purpose is **latest-frame-wins**: the host-side channel buffer holds at most N frames and drops the oldest on overflow, rather than the unbounded-queue behavior appropriate for terminal text. This is a per-`purpose` policy, not a global one — `exec` channels (command output) should *not* silently drop bytes, `embed-stream` channels should.

## Application to discovery (Docker / Kubernetes / DB)

Discovery becomes a thin layer on top of the same primitive, plus one new manifest capability for the "produces child connections" part:

```json
"capabilities": {
  "discovery": {
    "parentProtocols": ["ssh"],
    "childProtocol": "docker-shell"
  },
  "channel": { "purposes": ["exec"] }
}
```

- **Docker:** plugin opens an `exec` channel running `docker system dial-stdio` over the parent SSH session, speaks the Docker Engine API on that duplex stream to list containers, returns structured child-connection descriptors (`discovery.list` over JSON-RPC, unchanged shape from what we discussed earlier). Clicking a discovered container opens a *new* `exec` channel running `docker exec -it <id> sh`, wired to the session's terminal — architecturally identical to a normal SSH terminal session, just a different remote command.
- **Kubernetes:** same shape — `exec` channel running `kubectl exec -it <pod> -- sh` (or port-forward for API-server access), no new core mechanism needed.
- **Databases (Postgres/MySQL/Redis):** here the channel purpose is `tcp-relay` instead of `exec` — the host dials the DB port through the *existing* jump-host/tunnel chain (reusing `TunnelDialProxy`'s allowlisted dial, not a new mechanism), and the plugin speaks the DB wire protocol on top. A discovery plugin here could enumerate databases/schemas by running a query over that relayed connection.

The important architectural point: **discovery plugins don't get a new, separate transport** — they get the same `channel` capability everyone else gets, just with `exec` or `tcp-relay` purpose depending on whether the target lives *inside* the parent host (needs a remote process) or is *reachable through* it (needs a relayed dial). That's the one dimension of variation; everything else — framing, capability gating, audit, session binding — is shared.

## Session lifecycle coupling

A channel's `parentSessionId` binding is one-directional: **the parent session owns the channel's lifetime; the channel never owns the parent's.**

- When the parent session closes for any reason — user closes the tab, SSH connection drops, host initiates `session.disconnect`, plugin crash-recovery kicks in — the host **synchronously closes every channel bound to that session** before tearing down the session object. This should be added as an explicit step in the existing session-close sequence in `plugin-api.md` (alongside the existing `session.disconnect` notification step), not left implicit.
- The reverse never happens: closing a channel (e.g., an `exec` channel for one `docker exec` finishing) has no effect on the parent session or any of its other channels.
- Channel state is a small explicit machine — `opening → open → closing → closed` — with `close` idempotent on both sides: whichever side didn't initiate the close still needs to handle receiving `channel.close` for a channel it already considers closed as a no-op, not an error.
- **After sending `channel.close`, the host ignores all further frames for that `channelId`; the plugin must consider the channel closed and stop sending on it.** This applies equally whether the close was initiated by normal completion (command exited) or by an external event (user closed the tab, cascading from session close) — the plugin does not get a grace window to flush additional data after receiving `channel.close`.

## Protocol framing bootstrap

There is no prior shipped version of this transport and no installed base of plugins speaking raw NDJSON to preserve — the project has no users yet and this API is still under active development. Given that, the framing bootstrap has a clean answer instead of a negotiated one: **framing is mandatory from the first byte, for every plugin, unconditionally.** There is no raw-NDJSON fallback mode and therefore no chicken-and-egg problem to solve — `initialize` is simply the first JSON-RPC message sent on `channelId = 0` inside an already-framed stream, like every other control-plane message. The `kind` byte range `0x01–0x0F` stays reserved for core-defined frame types so the format can grow additively later, but no per-plugin version negotiation is needed for v1 because there is nothing yet to be compatible or incompatible with.

## Implementation note: multiple `exec` channels over one SSH connection

SSH natively multiplexes independent channels over a single transport connection, and the underlying Go SSH client used by `SSHConnector` supports opening multiple concurrent sessions on one `*ssh.Client`. Two implementation-level points worth flagging (not architectural, but easy to get wrong):
- Channel open/close must go through the same lock discipline ADR-009 already established (`SessionRegistry` as sole owner of shared session state) — new exec-channel code must not touch the SSH client directly outside that discipline.
- `maxConcurrent` bounds channels on the host side, but the remote `sshd` typically has its own independent limit (`MaxSessions`, OpenSSH default 10) — an operational constraint, not a security one, but worth documenting so a discovery plugin opening many concurrent `docker exec` channels doesn't hit it unexpectedly.

## Test matrix

Beyond the IDOR-style ownership test already required, the new primitive needs, at minimum:

| Area | Case |
|---|---|
| Concurrency | N channels open simultaneously with interleaved binary frames; assert correct per-channel demultiplexing and no cross-channel data bleed |
| Concurrency | One channel's consumer is slow; assert other channels and the JSON-RPC control plane are not head-of-line-blocked |
| Framing | Truncated frame header/payload across multiple `Read()` calls (simulate a syscall splitting a frame mid-flight) parses correctly once complete |
| Framing | Oversized `length` field is rejected before buffer allocation, connection terminated per the fail-fast policy |
| Limits | `maxConcurrent` is enforced **before** a new channel's resources are created, not after |
| Limits | `maxFrameSize` is enforced against the header before the payload is read/allocated |
| Lifecycle | Both sides sending `channel.close` concurrently for the same channel is idempotent, no panic/double-free |
| Lifecycle | Parent session close cascades to close of all bound channels, synchronously, before session teardown completes |
| `tcp-relay` | `hint` values that would be rejected by the existing `TunnelDialProxy` allowlist are rejected identically here (shared test fixtures with the existing dial-policy tests, not a separate suite) |

## v1 resolved parameters

The following were left as illustrative (`e.g.`) values in earlier drafts and are now fixed, non-configurable-by-plugin constants for v1 (deliberately not manifest-tunable — adding per-plugin knobs here would be exactly the kind of API-surface creep the "API, not SDK" principle rules out):

| Parameter | Value |
|---|---|
| Initial credit, `exec` / `tcp-relay` / `udp-relay` | 4 frames |
| Initial credit, `embed-stream` | 8 frames |
| `maxFrameSize` (kind=0x02) | 1 MiB, single global constant across all purposes |
| `maxConcurrent` default (manifest field absent or `0`) | 4 |
| `maxThroughputKbps` default (`0`) | host default, same numeric default as `maxTunnelBandwidthKbps` (32 MiB/s) — not "unlimited" |
| `channel.open` (plugin→host) RPC timeout | 10 s (matches `initialize`, not the standard 5 s RPC timeout — spawning an exec process or dialing a relay is comparably slow setup work) |
| `channel.close` grace period | 0 — immediate, no flush window, per §Session lifecycle coupling |
| kind=0x03 payload layout | `channel id` (4 bytes) + `credit count` (4 bytes), 8 bytes total, no subtype byte |

## `exec` command authorization — allowlisted argv templates, not free-form strings

`exec` is the highest-risk `purpose` — it runs a command over the user's already-authenticated session. It is **not** gated by consent alone; the set of runnable commands is closed and declared in the manifest as **argv-array templates**, never shell strings:

```json
"capabilities": {
  "channel": {
    "purposes": ["exec"],
    "execCommands": [
      { "argv": ["docker", "system", "dial-stdio"] },
      { "argv": ["docker", "exec", "-it", "{containerId}", "sh"],
        "params": { "containerId": "^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$" } }
    ]
  }
}
```

- Every element of `argv` is either a literal string or a named `{placeholder}`; every placeholder used in a template must have a corresponding regex in `params`, checked by the host before the process is spawned.
- The host always executes via an argv array (e.g. Go's `exec.Command(argv[0], argv[1:]...)`), **never** by constructing and running a shell string — this eliminates command injection as a class of bug rather than relying on escaping.
- A `channel.open` request for `exec` whose requested command doesn't match any declared template (or whose placeholder value fails its regex) is rejected at the capability gate, same as an undeclared RPC method — denied with the existing `-32001` shape and audit-logged.
- This closes what would otherwise be a genuine gap directly introduced by the new `channel` capability (not a pre-existing atomicity issue out of scope per §Atomicity) — `exec` never means "run anything the plugin asks for," only "run one of the specific, parameterized commands its manifest declared and the user consented to."

## On "full abstraction vs. purpose-limited"

Explicitly **not** a fully generic "open any pipe to anything" abstraction. `purpose` stays a closed, host-validated enum (`exec` / `embed-stream` / `tcp-relay` / `udp-relay`), each backed by host-side logic the plugin cannot influence beyond its declared parameters. A fully generic raw-socket abstraction would collapse the entire capability/consent model back to "trust the plugin" — which is exactly the failure mode `docs/security-model.md` is presumably built to avoid. New purposes can be added later — `udp-relay` was added exactly this way, as a deliberate, reviewed core purpose sharing the existing dial policy — but each one is such an addition, never something plugins can synthesize themselves, and never a generic "any protocol" destination the plugin picks freely.

## Atomicity

This ADR only introduces `channel.open` / `channel.close` as new atomic verbs plus the binary frame kind — it does not touch existing non-atomic areas of the API. Cleaning up the existing atomicity gaps you mentioned should land as its own ADR/PR before or alongside this one, so the new capability is added onto a clean baseline rather than inheriting old inconsistencies.

## Consequences

- One transport primitive serves VNC/RDP, Docker/K8s discovery+exec, and DB relays — not three bespoke mechanisms.
- Existing JSON-RPC plugins are completely unaffected (channel id 0 reserved, framing is additive).
- New attack surface is fully contained inside the existing capability/consent/audit/IDOR machinery — no parallel trust model.
- Plugin authors get a small, closed set of `purpose` values rather than a generic socket API, keeping the security review surface bounded as new plugin types are added.

## Alternatives considered

1. **Base64-in-JSON for binary data, no new transport.** Rejected — overhead and framing limits make it unworkable for video, and it doesn't solve the "duplex stream to a remote process" problem discovery needs at all.
2. **Separate OS pipe/socket per binary channel instead of multiplexing on stdio.** Rejected for v1 — multiplexing avoids extra fd/handle plumbing per-OS (relevant given `procattr_windows.go`/`procattr_linux.go` already carry per-OS complexity); can be revisited later if throughput demands it.
3. **Let plugins dial the target directly (raw socket/exec) themselves.** Rejected — reintroduces the exact credential-exposure and IDOR risk the existing proxy pattern was built to avoid.

## References

- [plugin-api.md](../plugin-api.md)
- [security-model.md](../security-model.md)
- ADR-008 — Session embed surfaces
- ADR-009 — SessionManager decomposition
- `internal/infra/plugin/capability/tunnel_local_proxy_idor_test.go` — precedent for required IDOR coverage on new proxies