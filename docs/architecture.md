# Architecture (xQuakShell)

This document complements [CONTRIBUTING.md](../CONTRIBUTING.md) with layer rules and where to add features.

## Layer diagram

```mermaid
flowchart TB
  subgraph layers [Dependency direction]
    main[main]
    pres[presentation/wails]
    uc[usecase]
    dom[domain]
    infra[infra]
    main --> pres
    main --> uc
    main --> infra
    pres --> uc
    pres --> dom
    uc --> dom
    infra --> dom
  end
```

- **main** (`app.go`) wires repositories, SSH adapters (`internal/infra/ssh`), portable layout, and plugin runtime.
- **presentation/wails** — Wails facade: `api.go`, handler files, DTOs, events. Handlers delegate to use cases; no direct infra imports.
- **presentation/logwindow** — debug log viewer subprocess and TCP stream server; depends on `domain.LogStream` only.
- **usecase** — orchestration (`SessionManager`, `TransferService`, `AuditService`, `SettingsService`, plugins). Depends only on **domain** and stdlib.
- **domain** — entities and ports split across `vault_data.go`, `app_settings.go`, `repositories.go`, `host_fs.go`, `portable_data.go`, etc.
- **infra** — persistence, SSH dialer, SFTP, audit log, host FS, portable data store, plugin host, etc.

## Filesystem zones (ADR-007)

Three trust boundaries — do not mix:

| Zone | Domain port | Infra implementation | Path policy |
|------|-------------|------------------------|-------------|
| Host user FS | `HostFileSystem` | `internal/infra/host/host_fs.go` | No sandbox root; trusted host UI |
| Portable app data | `PortableDataStore` | `internal/infra/portable/data_store.go` | Jailed to `<exe>/data` |
| Plugin sandbox | (IPC only) | `internal/infra/plugin/capability/fs_proxy.go` | Manifest roots + symlink checks |

Run `powershell -File scripts/check-fs-boundaries.ps1` to verify zone separation.
See [adr/007-host-filesystem-trust.md](adr/007-host-filesystem-trust.md).

## Import rules (summary)

| Package | May import |
|--------|-------------|
| `internal/domain` | stdlib, `golang.org/x/crypto/ssh`, `internal/domain/*` — **not** `internal/presentation`, `internal/infra`, `internal/pkg`, `main` |
| `internal/usecase` | `internal/domain`, `internal/pkg/safego`, stdlib — **not** `internal/infra/*`, other `internal/pkg/*`, third-party |
| `internal/infra/*` | `internal/domain`, `internal/pkg`, third-party, stdlib |
| `internal/presentation/*` | `internal/domain`, `internal/usecase`, `internal/pkg/safego`, stdlib |
| `main` | all internal packages as needed for composition |

Run `powershell -File scripts/check-imports.ps1` to verify layer imports.

## Goroutine policy

Background goroutines in production code must use [`internal/pkg/safego`](../internal/pkg/safego) — raw `go` launches are forbidden outside tests and plugin fixtures.

- Use **`safego.GoNamed("component.action", fn)`** with dotted names (`ipc.readLoop`, `session.initPTY`, `vault.debounceFlush`).
- Panics in background goroutines are recovered and logged via `slog.Error`; they must not crash the process.
- Cleanup (`defer`, `WaitGroup.Done`, context cancellation) remains the caller's responsibility inside `fn`.

Run `make check-goroutines` (or `powershell -File scripts/check-goroutines.ps1`) to enforce this rule.

## SSH types in domain

The project uses a **thin domain** over `golang.org/x/crypto/ssh`: interfaces such as `SSHClient`, `SSHClientConfig`, and `KnownHostsRepository` use `ssh` types in signatures. New domain ports should not introduce unrelated third-party types; keep SSH as the single external crypto dependency in `domain`.

## Plugin seam (SessionConnector)

SSH sessions are handled natively in `SessionManager.connectSession`. Non-SSH protocols can be added as **plugins** by implementing `domain.SessionConnector` and registering the implementation in `main_connectors.go`.

The core ships with an **empty connector registry** (`newSessionConnectors()` returns `nil`). When a connection uses a non-SSH `protocol` value and no plugin is registered, `OpenSession` transitions to error: `protocol X not yet implemented`.

Plugin connectors receive `ConnectorHooks` to set PTY bridge, SFTP (`RemoteFS`), SSH client, and to call `OnStreamReady` for stream-based terminals.

## Where to add features

| Area | Entry points |
|------|----------------|
| **Vault / connections** | Repositories in `internal/infra/persistence`, DTOs in `dto_connection.go`, handlers in `handlers_vault.go`. |
| **SSH sessions** | `internal/usecase/session_manager*.go`, PTY/SFTP init via `SessionManager.InitSessionIO`, handlers in `handlers_sessions.go`. |
| **Remote file browser** | `handlers_remote_fs.go` (DTO mapping); SSH exec via `SessionManager.Exec`. |
| **Local file browser** | `domain.HostFileSystem`, `internal/infra/host/host_fs.go`, `handlers_local_fs.go` (routing table in file header). |
| **Portable temp / data paths** | `domain.PortableDataStore`, `internal/infra/portable/data_store.go`. |
| **Transfers** | `internal/usecase/transfer_service.go`, handlers in `handlers_transfers.go`. |
| **Settings / ping / audit** | `settings_service.go`, `audit_service.go`, `ping_manager.go`, `handlers_settings_ping_audit.go`. |
| **Debug log window** | `domain.LogStream`, `internal/infra/loghub`, `internal/presentation/logwindow`. |
| **Plugins** | `internal/usecase/plugin_*.go`, handlers in `handlers_plugin*.go`, manifest FS checks in `infra/plugin/bundle/capabilities_validate.go`. |
| **Plugin connection fields** | Manifest: `internal/domain/plugin/fields.go`, validation in `manifest_fields_validate.go`; persistence: `PluginFieldsService`, `Connection.pluginFields`, `VaultData.pluginSecrets`; UI: `PluginConnectionFields.svelte`, `GetPluginConnectionProtocols`. |
| **Plugin protocols** | Out-of-process plugins via `PluginSessionBridge` and `session.connect` (with optional `fields`). |
| **Session embed** | `EmbedTunnelService`, `internal/infra/embed/broker_handler.go`, `SessionEmbedPanel.svelte`, `session.registerEmbed` / tunnel IPC. See [adr/008-session-embed-surfaces.md](adr/008-session-embed-surfaces.md). |

## SRP: plugin GitHub usecase

One type (`GitHubPluginService`), one file per reason to change — same pattern as `session_manager*.go`.

| File | Reason to change |
|------|------------------|
| `github_plugin_service.go` | Public facade: struct + constructor only (≤100 lines) |
| `github_metadata_cache.go` | Metadata cache keys and invalidation |
| `github_metadata_fetch.go` | GitHub API metadata fetch (list + by-tag) |
| `github_release_validator.go` | Release tag validation |
| `github_binary_fetch.go` | Release asset download via downloader port |
| `github_plugin_preview.go` | Install preview DTO |
| `github_plugin_install.go` | Install orchestration pipeline |
| `github_plugin_uninstall.go` | Uninstall + cache invalidation |
| `github_ports.go` | GitHub usecase ports (domain DTOs only) |

Pure platform/checksum parsing lives in `internal/domain/plugin/github_platform.go`. Security-critical flows stay isolated: list metadata must not download assets; release metadata may download checksums; install downloads verified binaries.

## SRP: plugin process host

One type (`ProcessHost`), one file per reason to change.

| File | Reason to change |
|------|------------------|
| `process_host.go` | Struct, constructor, interface assertion (≤100 lines) |
| `managed_process.go` | Managed process state + resource cleanup |
| `process_host_registry.go` | Process map, lookup, state queries |
| `process_host_lifecycle.go` | Wait, finalize, reservation rollback |
| `process_spawner.go` | Child process spawn and pipes |
| `process_sandbox.go` | Job objects, resource limits, data dir |
| `process_ipc_factory.go` | IPC server and capability proxy wiring |
| `process_initializer.go` | Plugin initialize handshake |
| `process_host_start.go` | Start pipeline orchestration |
| `process_host_stop.go` | Stop and StopAll |
| `process_host_rpc.go` | Call and Notify |

**SRP rules (enforced in CI):** new concern → new file; `internal/usecase` must not import `internal/infra`; `make check` includes `check-file-size` (300 lines for concern files, 100 for facades).

## Session embed data flow

```mermaid
flowchart LR
  UI[SessionEmbedPanel] -->|iframe + WS| Broker[Embed broker /embed/s/token]
  Broker --> Tunnels[EmbedTunnelService]
  Tunnels -->|session.tunnelFrame RPC| Plugin[Plugin process]
  Plugin -->|net.dial| Target[VNC/RDP server]
  Broker -->|session.tunnelData notify| Plugin
```

## Tests

- Use case SSH flows: `internal/usecase/session_manager_ssh_test.go` (no network; mocked ports).
- Broader unit tests: `test/unit/`.
