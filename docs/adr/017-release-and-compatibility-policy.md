# ADR-017: Release and Compatibility Policy

## Status

Accepted. Extends [ADR-012](012-plugin-api-versioning.md) from the plugin API to every published
contract, and adds the release-side rules that ADR-012 assumed but never wrote down.

## Context

ADR-012 froze the plugin API and gave it a compatibility oracle and two CI guards. Everything else
the first release turns into a public promise was undefined:

- **Nothing said how many releases are supported.** For a tool that stores SSH keys, "just upgrade"
  is only an answer if the user can find out an upgrade exists.
- **Three other contracts were frozen in practice and versioned in nothing.** The `plugin.json`
  schema had no version and no snapshot, so tightening one branch in `Validate` would silently stop
  every published plugin from installing. The `.xqsp` bundle layout had no version at all, so an
  older host meeting a newer bundle could only report "signature invalid". The on-disk formats had
  a number but no rules around it.
- **There was no way to announce a breaking change.** Release notes were generated from commit
  subjects, which tells a plugin author nothing about whether their plugin still loads.
- **`wails.json` `productVersion` was hardcoded** and read by nothing, so it would have kept
  reporting 1.0.0 through every later release.

Two defects found while writing this ADR show what the missing rules were costing:

- `vault.Decrypt` compared the stored schema version with `!=` and there were no migrations, so
  raising `CurrentVaultVersion` by one would have bricked every existing vault. The single error it
  returned was mapped nowhere and shown to the user raw.
- `audit.db` had no schema version and its DDL was one `CREATE TABLE IF NOT EXISTS` block, which
  does nothing against a table that already exists. Every column added after the first release never
  reached a database carried across an upgrade, and every insert naming one failed at runtime.

Neither is a versioning subtlety. Both are what happens when nobody has written down that a format
is a contract.

## Decision

### 1. Six version axes, independent by default

| Axis | Format | Source of truth |
|---|---|---|
| Application version | SemVer, git tag `v1.2.3` | tag → `AppVersion` ldflag, `wails.json` `info.productVersion` |
| `pluginApi` | SemVer | `internal/domain/plugin/api_registry.go` |
| Capability versions + feature flags | SemVer each | same file, `hostRegistry` |
| `plugin.json` schema | no number | golden snapshot; the invariant is behavioural |
| `bundleFormat` | SemVer | `internal/domain/plugin/bundle_format.go` |
| On-disk schemas | monotonic integer | `domain.CurrentVaultVersion`, `PRAGMA user_version` in `audit.db` |

**The application version and `pluginApi` are fully independent.** A breaking plugin API change does
not force a major application release and vice versa. They answer to different readers — one to a
user deciding whether to upgrade, the other to a plugin author deciding whether their code still
compiles against the contract — and tying them together is the same conflation ADR-012 rejected.

The manifest schema deliberately has **no** number. The manifest is read only by the host; there is
no second party to negotiate with, so a version would be a number nobody consults. The obligation is
stated as an invariant instead — *a manifest that was valid stays valid* — and enforced by a golden
snapshot that records both the field shape and, by clearing one field at a time from the smallest
accepted manifest, which fields `Validate` actually requires.

### 2. Frozen at 1.0.0

Four contracts are frozen: the plugin API (ADR-012), the `plugin.json` schema, the on-disk formats,
and the bundle format. Freezing means changes are additive within a major; anything else needs a
major bump on that axis and a deprecation window.

Explicitly **not** frozen, so the rule is not over-applied: the Wails bindings between the frontend
and Go, every `internal/` package, the UI layout, and the log format. These are implementation.

### 3. Per-contract rules

| Contract | Additive within a major | Breaking requires | Window |
|---|---|---|---|
| `pluginApi`, capabilities | new capabilities, minor bumps, new feature flags | major bump on that axis | ADR-012: ≥ 2 minors and until the next major |
| `plugin.json` schema | new **optional** fields | major `pluginApi` bump | same |
| `bundleFormat` | new archive entries, new optional metadata | major `bundleFormat` bump | same |
| On-disk schemas | new fields with defaults | a version bump | none — backup and refusal instead |

A new **required** manifest field, or a tightened validation branch, is a breaking change even
though the code still compiles: a plugin that installed yesterday stops installing today.

### 4. On-disk data: refuse forward, back up before migrating

A deprecation window makes no sense for a file, so the rule is different in kind:

1. **A build never touches data written by a newer build.** Not a partial read, and above all not a
   write: writing back only the fields this build understands would silently drop the rest. The two
   directions are separate errors (`ErrVaultVersionTooNew`, `ErrVaultVersionTooOld`) because they
   need opposite actions from the user, and both are mapped to a message that says which.
2. **Any migration copies the data aside first**, named after the version being left behind
   (`vault.age.v3.bak`). `vault.BackupVaultFile` refuses to overwrite an existing backup — a crash
   mid-upgrade or a downgrade and re-upgrade would otherwise replace the last pre-migration copy
   with already-migrated bytes. Nothing deletes these files.
3. **Every store carries a schema version.** The vault has `VaultData.Version`; `audit.db` has
   `PRAGMA user_version`.

The vault has no migration registry and will not get one until there is a migration to write. The
gate in §6 is what makes sure the first person to need one finds out before shipping.

### 5. Support window: the latest release only

Fixes, including security fixes, ship as a new release. There are no long-lived release branches and
no backports. A maintainer who promises a support window they cannot meet leaves users on an old
build believing they are covered, which for a tool holding SSH keys is worse than promising nothing.

Severity decides timing; there is no fixed SLA. When `main` is not in a releasable state, the
mechanism is a branch from the last release tag carrying the fix alone.

Because only the latest release is supported, the application must be able to say that a newer one
exists: it checks GitHub Releases at startup and shows a banner when it is behind. Otherwise
"upgrade" is advice the user has no way to act on. It downloads and installs nothing, and can be
turned off.

Plugins built against a superseded major keep their installed state and their data. They do not
load, and the plugins panel says exactly why and against which version. The host does not carry two
live protocol majors at once: that doubles the attack surface of a security tool to save an upgrade.

### 6. Enforcement

The rules that fail a build, because a policy nothing enforces is a policy nobody follows:

| Gate | Where | Catches |
|---|---|---|
| Version convergence | `test/unit/release/version_test.go` | `wails.json productVersion` disagreeing with the newest changelog version; an entry with no `Compatibility` or `BREAKING` block; an older entry left undated |
| Publish gate | `.github/workflows/release.yml`, `publish` job | a tag with no changelog section, a section still marked unreleased, a missing block, a `productVersion` that disagrees with the tag |
| Manifest schema | `internal/domain/plugin/manifest_schema_test.go` | any change to the field shape or to which fields `Validate` requires; separately, a removal or a tightening |
| Vault schema tripwire | `test/unit/release/schema_version_test.go` | `CurrentVaultVersion` moving without the changelog moving with it |

Existing ADR-012 guards (`TestFrozenAPISurface`, `TestAPISurfaceAdditiveOnly`) continue to cover the
plugin API surface.

### 7. Announcing a change

One hand-written `CHANGELOG.md` in Keep a Changelog format. Every release entry carries:

- a **`Compatibility`** table listing all six axes, so "does my plugin still work" and "what happens
  to my data" are answered without reading a diff;
- a **`BREAKING`** section, present even when it says *Nothing* — an optional section is missing
  both when there is nothing to report and when someone forgot, and those must not look alike. When
  it is not empty it says what to do, not only what changed.

The newest versioned heading is the release being prepared, so there is always exactly one number
for the gates to hold `wails.json` and the release tag against.

## Runbook

- **Ship a breaking plugin API change.** Deprecate the item first (ADR-012 `DeprecationInfo`), keep
  it working for the window, then bump that axis's major, regenerate the golden, review the diff,
  and write the `BREAKING` section with the migration steps.
- **Add a manifest field.** Optional, with a working default. Regenerate the manifest golden and
  check the requirements half is unchanged. Never make an existing field required.
- **Change the bundle layout.** Additive: bump `CurrentBundleFormat`'s minor. Breaking: bump its
  major and give the window. Remember an absent `bundleFormat` means `1.0.0` forever and must not be
  redefined to track the host.
- **Bump an on-disk schema.** Write the migration, call `BackupVaultFile` before the first write at
  the new version, prove an older build refuses the result, and record the bump in the changelog.
- **Cut a release.** Date the newest changelog heading, set `wails.json productVersion` to match,
  `make gates`, tag `vX.Y.Z`. The publish job refuses anything incomplete.

## Rejected alternatives

- **One version for the whole product.** The conflation ADR-012 already rejected for the plugin API,
  and it gets worse with six axes: a UI fix would bump a number plugin authors gate on.
- **Supporting the previous minor for 90 days.** Requires release branches and fixing every bug
  twice. A single maintainer would stop doing it within two releases, and a support promise that has
  quietly lapsed is worse than none.
- **Generated release notes plus a `BREAKING CHANGE:` commit trailer.** Nothing to maintain, but
  commit subjects do not tell a reader what to do, and there is nowhere to put the compatibility
  table.
- **Running two plugin API majors side by side.** Kinder on upgrade, but two live protocols in a
  security tool doubles the surface that has to be right.
- **Full backward and forward compatibility for the vault within a major.** Would let a downgrade
  keep working, but requires preserving unknown fields across a read-modify-write cycle. Refusing to
  open is a fraction of the work and cannot corrupt anything.
- **A migration registry with no migrations in it.** Framework for a case that has never occurred.
  The tripwire gets the same outcome — nobody bumps the version unaware — at no cost.
