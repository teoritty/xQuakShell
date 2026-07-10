# ADR-009: SessionManager Decomposition

## Status

Accepted

## Context

`SessionManager` (`internal/usecase/session_manager*.go`) grew into a God Object: one struct with 46 methods spanning five unrelated concerns — session registry (map + mutex), SSH connection/auth, PTY/RemoteFS IO, plugin-session protocol, and embed-tunnel protocol. Multiple files read and write `m.sessions` and `m.mu` directly, creating data-race and lock-ordering risk on every change.

`PluginSessionBridge` and `EmbedTunnelService` already exist as the correct extraction targets but plugin/embed protocol methods still live on `SessionManager`, and embed handlers reach into `pluginBridge.plugins.Registry()` from outside the bridge (Law of Demeter violation).

## Decision

Decompose `SessionManager` into single-responsibility components with a thin facade preserved for `internal/presentation/wails` and `app.go` (Wails facade):

| Component | Responsibility |
|-----------|----------------|
| `SessionRegistry` | Sole owner of `map[sessionID]*sessionEntry` and its mutex |
| `SSHConnector` | Pure SSH handshake (auth, jump chain, host key) — no session state side effects |
| `SessionIOService` | PTY, RemoteFS, Exec, keepalive — uses registry only |
| `PluginSessionBridge` | All plugin-session protocol (connect, crash, recovery, terminal write) |
| `EmbedTunnelService` | All embed-tunnel protocol including manifest validation |
| `SessionLifecycleService` | Orchestrates Open/Close/Retry/NotifyDisconnected |
| `SessionManager` (facade) | One-line delegates only; ≤120 lines |

**Invariants enforced in CI** (`scripts/check-session-manager-boundaries.ps1`):

- Only `session_registry.go` may touch the sessions map/mutex directly.
- `session_manager.go` stays delegate-only.
- `InitSessionIO` blocks on a ready channel, not a 100 ms ticker poll.

External method signatures visible to presentation layer remain unchanged.

## Consequences

- Each concern can be unit-tested in isolation (e.g. `SSHConnector` with fake repos, zero session knowledge).
- Plugin/embed wiring (`SetCrashHandler`, `SetSessionOwnershipChecker`, inbound handlers) points at `PluginSessionBridge` / `EmbedTunnelService`, not the facade internals.
- New session features must land on the appropriate component, not back on the facade — guard script blocks regression.
- Refactoring is behavior-preserving; existing and new characterization tests are the safety net.

## Alternatives considered

1. **Keep God Object, split files only** — Rejected: does not fix shared mutable state or testability.
2. **Rename and expose five services to presentation** — Rejected: hundreds of call sites; facade preserves stability.
3. **Interface-only SessionManager with same struct** — Rejected: does not reduce coupling inside the package.

## References

- [architecture.md](../architecture.md)
- ADR-001 — SSH fast path vs plugin sessions
- ADR-008 — Session embed surfaces
