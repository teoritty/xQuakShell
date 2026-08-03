# ADR-007: Host Filesystem Trust

## Status

Accepted

## Context

xQuakShell is an SSH client with a local file browser, SFTP transfers, and native file dialogs. Users must browse and transfer files anywhere on their machine (e.g. `C:\Users\...`, `/home/...`), not only under the portable data directory (`<exe>/data`).

A previous implementation routed all host local filesystem operations through a single adapter jailed to `<exe>/data` using `pathsafe.ResolveUnderRoot`. That policy is correct for **portable application state** and for **plugin IPC**, but incorrect for **host UI** — the core was sandboxing itself like an untrusted plugin.

## Decision

Split filesystem access into three zones with separate ports and path policies:

| Zone | Port | Policy |
|------|------|--------|
| Host user FS | `domain.HostFileSystem` | `pathsafe.ResolveHostPath` — normalize only, no root jail |
| Portable app data | `domain.PortableDataStore` | `pathsafe.ResolveUnderRoot(<exe>/data)` |
| Plugin sandbox | `capability.FSProxy` | `pathsafe.SecurePathUnderRoots` + manifest `${pluginData}` |

The host process is the trust anchor for `HostFileSystem`. Callers are Wails-bound UI methods and use cases that serve the UI (e.g. `TransferService`). Plugins cannot invoke this port; their only FS surface is `fs.*` IPC through `FSProxy`.

Local Files browser defaults to `os.UserHomeDir()`.

## Consequences

- Local Files panel and SFTP transfers work on the full user filesystem.
- Portable vault, plugin data, and temp dirs remain jailed under `<exe>/data`.
- Plugin isolation is unchanged.
- `AppAPI` holds two fields (`hostFS`, `portableData`); no mode flags on a shared adapter.
- `scripts/check-fs-boundaries.ps1` guards against mixing zones in CI.

## Alternatives considered

1. **Single sandbox for everything** — Rejected: breaks SSH client UX; incorrectly treats host as untrusted.
2. **Reuse FSProxy for host browser** — Rejected: wrong threat model; would require manifest roots covering the entire disk for the host.
3. **`LocalFileSystem` with `sandbox \| trusted` mode** — Rejected: ambiguous API; invites tech debt and regressions.

## References

- [security-model.md](../security-model.md) — Host trust boundary
- [architecture.md](../architecture.md) — Filesystem zones
- ADR-006 — Portable data layout
