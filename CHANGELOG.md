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

## [Unreleased]

### Compatibility

| Axis | Version |
|---|---|
| `pluginApi` | 1.0.0 (unchanged) |
| Capabilities | `session` 1.0.0 — `localEmbedServer` feature removed · all others 1.0.0, unchanged |
| Manifest schema | `capabilities.session.localEmbedServer` removed |
| `bundleFormat` | 1.0.0 (unchanged) |
| Vault schema | 3 (unchanged) |
| Audit schema | 1 (unchanged) |

### BREAKING

**`capabilities.session.localEmbedServer` and the `session.reportLocalEmbed` RPC are removed.** This
was ADR-008 Mode B: an opt-in loopback HTTP server inside the plugin process. It was the only path
on which a plugin opened a listening socket of its own, and that is incompatible with the OS-level
isolation being built — a sandbox cannot deny a plugin the network and simultaneously let it serve
HTTP. Embed sessions use the host-side broker (Mode A), which is what every existing plugin already
uses.

**Who is affected:** a plugin that names `localEmbedServer` in
`requires.capabilities.session.features` is now refused at the handshake with `-32010`. Nothing else
changes. A plugin that granted `localEmbedServer` in `capabilities` without requiring it by name
still loads; the field is ignored, and the RPC it used to unlock is gone.

**Nothing that worked stops working.** The removed feature never functioned:
`HandlePluginReportLocalEmbed` returned `ErrLocalEmbedNotSupported` unconditionally, so the manifest
field parsed and the capability gate allowed the call while only the handler refused. The documented
install consent for it did not exist either — the warning was computed and never read, and the
vault grant map was never written or checked.

**No version axis moved, and that is deliberate.** `pluginApi` versions the protocol envelope —
framing, handshake, lifecycle, error space — none of which changed. The `session` capability did not
take a major either: capability majors are matched exactly, so bumping it would refuse every plugin
that grants `session` until its manifest named the new number, including plugins whose behaviour is
identical either way, while catching nothing the per-feature check does not already catch. The
removal is recorded by name in `removedFeatures` (`api_contract_test.go`) and `removedSchemaFields`
(`manifest_schema_test.go`) instead, where it is reviewed rather than inferred from a number.

### Added

- **Plugin processes are confined by the operating system on Windows and Linux.** A plugin now runs
  inside a Windows AppContainer or a Linux Landlock ruleset that grants it read and execute on its
  own installed files, read and write on its own instance data directory, and nothing else. Your
  SSH keys, the vault, another plugin's directory and another session's directory are denied by the
  OS rather than by the plugin's good behaviour.

  Each plugin's settings row says which boundary its running processes are actually behind, and the
  two platforms do not get the same words:

  - **Windows** reports **sandboxed**. An AppContainer is granted no network capability, so the
    plugin has no sockets at all — not outbound, not inbound, not even loopback.
  - **Linux** reports **sandboxed (files only)**. Landlock's network rules cover TCP only, and only
    on kernel 6.7 and newer, so a confined plugin can still open a UDP socket. The filesystem side
    is complete.
  - **macOS** still reports **not sandboxed**, and says why.

  Linux needs kernel 5.13 or newer with Landlock enabled; Windows needs the AppContainer profile
  API, which is present on every supported build and needs no administrator rights. Where the
  platform cannot confine a plugin it starts exactly as before and the row says so — that is not an
  error.

  **Where the platform can confine a plugin and the attempt fails, the plugin does not start.** A
  sandbox that quietly fell back to an unconfined process whenever it broke would keep reporting
  success while protecting nobody. **Settings → Plugins → Trust policy** carries the escape hatch for
  a machine the confinement will not work on — *Start a plugin unconfined if its sandbox cannot be
  applied*, off by default. A start taken under it is logged and the plugin's row reads **not
  sandboxed**.

  A plugin that reads or writes outside its own directories will now fail where it used to succeed.
  None of the published plugins do; the host performs every network and filesystem operation on a
  plugin's behalf already.

### Fixed

- A plugin's `TMPDIR` was never set, only `TEMP` and `TMP`, so on Linux and macOS every plugin's
  temporary files went to the shared `/tmp` instead of its own instance directory.
- **Plugin settings could not be saved.** `SaveSettings` deliberately carries the stored plugin
  section across so that changing the theme cannot clear a capability grant — and that also
  discarded what the plugin settings dialog itself wrote, so "require signed plugins" silently never
  persisted. The dialog now writes that section through its own path, merging onto the stored copy
  rather than replacing it.
- **On Windows, a plugin could be refused access to its own files if your account name is long.**
  Windows gives such a profile an 8.3 alias — `C:\Users\RUNNER~1\…` alongside
  `C:\Users\runneradmin\…` — and the checks that keep a path inside its allowed directory resolved
  one side of the comparison to the canonical long form while leaving the other in whatever spelling
  it arrived. The two never matched, so reading or writing inside the plugin's own data directory
  came back as `plugin capability denied`, an installed plugin's own binary could be reported as
  escaping its bundle, and its UI assets were served as `403`. The same comparison guards SFTP and
  the local file manager.

  **Who is affected:** anyone on Windows whose account name is longer than eight characters or
  contains a space, which is most people. Both sides are now compared in the same spelling.
- **A path that is not valid UTF-8 could hang the check that decides whether a path escaped its
  root.** Windows accepts such a string as a UNC volume name and hands back a replacement character
  when it resolves it, after which the standard library's path comparison spins forever. This is
  hardening rather than a defect anyone hit: the roots involved come from the application's own
  configuration and not from anything a user or a remote host supplies, and no shipped code path is
  known to reach it. It was found by fuzzing the plugin sandbox's arguments. Such a path is now
  refused outright, in the deny direction, on every entry point.

### Removed

- `capabilities.session.localEmbedServer` manifest field, its `localEmbedServer` feature id, and the
  `session.reportLocalEmbed` RPC together with the dead `localEmbedServerAccessGranted` vault
  settings map, which nothing ever read or wrote.

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
