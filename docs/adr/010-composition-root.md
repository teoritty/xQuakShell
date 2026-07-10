# ADR-010: Composition Root Policy

## Status

Accepted

## Context

The technical specification requires that **usecase** and **infra** are wired together only in the composition root (`main.go` and `main_*.go`). Historically, `app.go` contained `NewApp()` with full dependency injection — including `SSHAuthWiring` (bridging `usecase.PluginAuthBridge` with `infra/ssh.PluginAuthMethodBuilder`). This blurred the boundary between the Wails binding facade and the composition root, and conflicted with internal docs that listed `app.go` as a wiring location.

## Decision

The composition root is **`package main`**, limited to:

| File | Role |
|------|------|
| [`main.go`](../main.go) | Process entry: Wails bootstrap, calls `composeApp()` |
| [`main_compose.go`](../main_compose.go) | Core DI: repos, SSH stack, `NewAppAPI`, post-wire hooks |
| [`main_ssh_auth.go`](../main_ssh_auth.go) | Plugin SSH auth: `SSHAuthWiring` (usecase ↔ infra glue) |
| [`main_plugins.go`](../main_plugins.go) | Plugin runtime, embed tunnels, GitHub services |
| [`main_connectors.go`](../main_connectors.go) | Non-SSH `SessionConnector` registry |

[`app.go`](../app.go) is the **Wails binding facade** only: `App` struct, lifecycle (`startup`/`shutdown`), and method delegates to `presentation.AppAPI`. It must **not** import `internal/infra/*`, `internal/pkg/conlimit`, or `internal/pkg/ratelimit`, and must not define package-level `New*` constructors.

**Rules:**

1. usecase ↔ infra binding happens **only** in `main_*.go` (or orchestrated from `main.go` via `composeApp()`).
2. Dependencies are passed downward through constructors (`presentation.NewAppAPI`, usecase configs) — never constructed inside `presentation` or `usecase` production code.
3. `SSHAuthWiring` assembly lives exclusively in `main_ssh_auth.go` (`wireSSHAuth`).

## Consequences

- Adding a new infra adapter or cross-layer port requires a change in the appropriate `main_*.go` file, not `app.go`.
- CI enforces the policy via `scripts/check-composition-root.ps1` and `test/unit/architecture/composition_root_test.go`.
- `app.go` remains `LayerMain` for layer-import rules in `rules.go`, but a separate guard blocks infra imports in the facade.
- ADR-009 references to wiring in `app.go` are superseded by this ADR for composition concerns.

## References

- Technical specification section 1.9 (composition root)
- [`docs/architecture.md`](../architecture.md)
- [`AI_GUIDELINES.md`](../../AI_GUIDELINES.md) section 1.3
