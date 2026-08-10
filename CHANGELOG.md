# Changelog

Notable changes to xQuakShell. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

**Every release carries a `Compatibility` block and a `BREAKING` section.** `BREAKING` is present
even when there is nothing to report, so its absence always means the entry is incomplete rather
than that the release was safe. The compatibility numbers are separate axes and move independently
of the application version — see [ADR-017](docs/adr/017-release-and-compatibility-policy.md).

The newest versioned heading is the release being prepared. Only the latest release is supported;
fixes, including security fixes, ship in a new release rather than as patches to an older one
(see [SECURITY.md](SECURITY.md)).

## [1.0.0] — 2026-08-10

### Compatibility

| Axis | Version |
|---|---|
| `pluginApi` | 1.0.0 |
| Capabilities | `network` 1.0.0 · `filesystem` 1.0.0 · `events` 1.0.0 · `vault` 1.0.0 · `session` 1.0.0 · `auth` 1.0.0 · `tunnel` 1.0.0 · `channel` 1.0.0 · `discovery` 1.0.0 · `ui` 1.0.0 |
| Manifest schema | unchanged |
| `bundleFormat` | 1.0.0 (new axis) |
| Vault schema | 3 |
| Audit schema | 1 (new axis) |

All four public contracts are frozen as of this release: the plugin API, the `plugin.json` schema,
the on-disk data formats, and the bundle format. Breaking any of them now requires a major bump on
that axis and a deprecation window.

### BREAKING

Nothing.

### Added

- `bundleFormat` — the `.xqsp` archive layout is now a version axis of its own, declared in
  `plugin.json` and independent of `pluginApi`. Omitting the field means `1.0.0`, so bundles
  published before it existed install unchanged.
- The vault is copied aside before any future schema migration, named after the version being left
  behind (`vault.age.v3.bak`). Existing backups are never overwritten and nothing deletes them.
- The audit database records its schema version in `PRAGMA user_version`.
- An update check: the app tells you when the release you are running has been superseded, since
  only the latest release is supported. It runs once the vault is unlocked (the setting permitting
  it lives in the vault), makes one anonymous request, downloads and installs nothing, is recorded
  in the audit log, and can be turned off under Settings → About.

### Fixed

- **The audit log was silently broken on any database carried across an upgrade.** Schema creation
  was a single `CREATE TABLE IF NOT EXISTS` block, which does nothing against a table that already
  exists, so columns added after the first release (`category`, `connection_name`, `host`,
  `username`, `redacted`) never reached an existing `audit.db` and every insert failed at runtime.
  Opening the database now reconciles it against the expected columns and repairs it in place.
- A vault written by a newer build is now distinguished from one that predates this build. The two
  need opposite actions from the user and previously produced the same unmapped error, shown raw.
- **Saving settings wiped plugin trust.** `AppSettingsDTO` never carried the plugin section, and the
  save assigns the whole settings struct, so changing the theme cleared `RequireSignedPlugins`,
  every granted capability and the disabled-plugin list.

## [1.0.0-rc.3] — 2026-08-07

### Compatibility

| Axis | Version |
|---|---|
| `pluginApi` | 1.0.0 |
| Capabilities | all 1.0.0; `ui` added |
| Manifest schema | `capabilities.ui`, `contributions.views`, `contributions.discoveryIcons` added |
| `bundleFormat` | not yet an axis |
| Vault schema | 3 |
| Audit schema | not yet an axis |

### BREAKING

Nothing for plugins built against rc.2. A plugin that contributes a UI must now publish `.xqsp`
bundle assets rather than a bare binary — a bare binary carries no `ui/` tree, so those files were
missing and every request for them returned 404. Installs of such a plugin are refused with that
reason instead of failing later.

### Added

- Plugin UI surfaces (ADR-015): plugins draw their own tabs, dialogs and node-details panels; a
  `ui` capability declares which surfaces they may use.
- Plugin dialogs with a shared field renderer, wire-schema validation and audited submissions.
- Discovery node details, verbs and a sidebar panel; a plugin-marked action can be bound to Delete.
- `keyValue` and `code` field types in both the connection editor and plugin dialogs.
- The GitHub install route can carry a whole `.xqsp` bundle and picks it over a bare binary.
- Size, function and comment budgets for Go and the frontend, with a ratchet on recorded debt.
- `make gates` runs every pre-merge check in one command; nightly mutation testing with a ratchet.

### Fixed

- IPC routing keys on a message's method rather than its id.
- A plugin surface keeps its viewer mounted across state changes and can become a tab.
- An auth RPC is audited whichever entry point it arrives through.
- `discovery.publishDetails` is rate-limited on the shared discovery budget, and action text is
  sanitized and bounded.

## [1.0.0-rc.2] — 2026-08-04

First tagged pre-release: a portable SSH client with an encrypted vault, terminal, SFTP, bastion
chains, audit logging and a versioned plugin system.

### Compatibility

| Axis | Version |
|---|---|
| `pluginApi` | 1.0.0 — frozen here (ADR-012) |
| Capabilities | `network`, `filesystem`, `events`, `vault`, `session`, `auth`, `tunnel`, `channel`, `discovery`, all 1.0.0 |
| Manifest schema | `requires{}` introduced; `minCoreVersion` rejected |
| `bundleFormat` | not yet an axis |
| Vault schema | 3 |
| Audit schema | not yet an axis |

### BREAKING

`minCoreVersion` is no longer accepted in a manifest; declare `requires{}` instead. No plugin ever
shipped against the pre-1.0 API, so nothing in the wild needed migrating.

### Added

- Plugin API versioning (ADR-012): a frozen protocol envelope plus independently versioned
  capabilities with named feature flags, negotiated at install and re-checked at `initialize`.
- Plugin-drawn discovery subtrees inside the connection tree (ADR-014).
- Binary duplex channel bus for plugins (ADR-011) and embedded session surfaces (ADR-008).
- Explicit vault creation split from unlocking, with an offline password strength meter.
- `ssh_config` import with includes, wildcards and proxy jump.
- FileZilla-style transfer conflict resolution and a live scanning counter.
- Tag-driven release workflow; portable Windows and Linux archives with checksums.
