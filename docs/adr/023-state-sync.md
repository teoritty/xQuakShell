# ADR-023: State Sync Through a Blind Server

## Status

Proposed. Extends [ADR-017](017-release-and-compatibility-policy.md) with new version axes and
narrows one sentence of [ADR-021](021-vault-recovery-key.md). Design:
`docs/superpowers/specs/2026-10-13-sync-design.md` (kept local, not committed).

## Context

Users run xQuakShell on several Windows and Linux machines and re-create connections, keys and
settings on each. Teams want to share hosts and credentials. ADR-021 recorded that xQuakShell "has
no server and is not going to grow one to hold the credential that opens every vault it ships";
the reason was that such a server would be the single most valuable target the project could
create.

## Decision

**An optional, self-hosted server, `xqsyncd`, that stores only ciphertext and signed metadata and
cannot open anything it stores.**

- Every record (connection, key, password, setting section, plugin install, plugin file) is sealed
  on the device under a fresh per-version data key, wrapped under a per-vault, per-epoch key that is
  itself HPKE-wrapped (ML-KEM-768 + X25519) to each member's account key.
- The account key bundle is stored on the server encrypted under a key derived from the sync
  password through OPAQUE (RFC 9807, Argon2id at fixed suite parameters). The server never sees the
  password or anything it could guess against without its separately stored secret key file.
- The server orders commits; clients decide. Each commit carries a vault head signed by the
  committing device: previous head hash, Merkle root over every record, membership position. A
  server that drops, reorders, replays, rolls back or swaps anything is detected; a fork is
  detected when the branches meet.
- Device enrolment is by password, as the owner chose. Every enrolment is in a signed device log
  that every other device shows; revocation rotates the account keys and the vault epochs.
- Changes that weaken security (plugin trust, key flags, agent forwarding, lockout, host-key
  replacement) arrive pending and need the local master password, through the same
  `domain.PluginTrustWeakened` rule local saves use. One-way latches (`NonExportable`) are never
  cleared by sync.
- Deleted data stays in an encrypted trash for the vault's retention window, then is purged; epoch
  keys retire one window after their successor starts, after which the data is unrecoverable from
  current state and current credentials.

### What changes in ADR-021

The sentence "xQuakShell has no server" no longer holds. What ADR-021 protected still holds: no
server holds a credential that opens a vault. The bundle on the server opens only with the sync
password, which the server never receives.

### New version axes (extends ADR-017 §1)

| Axis | Format | Source of truth |
|---|---|---|
| `syncProtocol` | SemVer | `internal/domain/vaultsync/version.go` |
| Sync crypto suite | integer, never negotiated down | `internal/domain/vaultsync/suite.go` |
| Record schemas | integer per record type; any change bumps it; older clients treat newer as read-only | record type registry |
| Sync state schema | `PRAGMA user_version` of `data/sync/state.db` | client store |
| Server store schema | `PRAGMA user_version` of the server database | server store |

The vault schema moves 4 → 5 when the client gains `vaultId` and `Sync` fields, because an older
build rewriting the vault would silently drop them and turn team items into personal ones.

### Package boundaries

`internal/domain/vaultsync` is a leaf: stdlib only. The server lives under `internal/syncserver/`
(`usecase` may import only `internal/domain` and `safego`; `store` and `transport` may add
`internal/pkg`, `internal/infra/synccrypto`, `internal/infra/opaque` and third-party code) and is
composed in `cmd/xqsyncd`. The application composition root never imports `internal/syncserver`,
and nothing in the application imports it either. `test/unit/architecture/rules.go` enforces this.

## Consequences

- Password compromise equals account compromise. The device log makes it visible; the server admin
  can freeze an account; nothing cryptographic can prevent it under password-only enrolment.
- A server backup taken before a purge stays readable with the password valid at that time.
  Documented, with `admin backup --max-age`.
- A viewer in a team vault can read every secret in it. End-to-end encryption cannot grant "use
  without seeing"; `credentialSource: personal` shares a host without its credential.
- A new dependency, `github.com/bytemare/opaque`, enters behind a port, pinned, with RFC vectors in
  CI and a recorded source review.

## Alternatives rejected

- **Syncing `vault.age` as a file.** No per-record shredding, no field merge, and the server sees
  the size of every change to the whole vault.
- **A full CRDT.** Silently merges values only a person should choose between, such as two
  different passwords for one host.
- **SRP instead of OPAQUE.** SRP's verifier allows precomputation; OPAQUE's OPRF does not, and its
  server key can live outside the database.
- **Device approval for enrolment.** Stronger against a stolen password; declined by the owner in
  favour of password-only enrolment. Recorded as a possible future suite option.
- **Shipping plugin binaries through the server.** A compromised device could then distribute code.
  Plugins sync by reference and each device installs from the source with the usual checks.
