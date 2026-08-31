# ADR-021: A Second Credential Opens the Vault

## Status

Accepted. Adds an on-disk axis governed by [ADR-017](017-release-and-compatibility-policy.md).

## Context

A forgotten master password lost everything. `vault.age` was an age v1 file with a single scrypt
recipient derived from that password, and `VaultRepo` kept the password itself in a field so it could
re-encrypt the whole file on every flush. There was one credential, one recipient stanza, and no
second way in. Users reported losing entire vaults this way.

The obvious fix — a recovery credential — must not weaken anything. Whatever it is has to be as
expensive to attack as the password, must not become a cheaper target that makes the password
irrelevant, and must not turn "I forgot my password" into "anyone with my machine can reset it".

## Decision

**A random vault key sits between the credentials and the data. Each credential holds a separately
wrapped copy of it.**

The file becomes a plaintext JSON envelope:

```
{ "envelope": 1,
  "wraps": [ {"kind":"password","data":"…"}, {"kind":"recovery","data":"…"} ],
  "payload": "…" }
```

- `payload` is the vault JSON encrypted to an age X25519 recipient. No key derivation: the vault key
  is 256 bits of machine entropy that no human types, so there is nothing to stretch.
- each wrap is the vault key's private half, encrypted under one credential with the same
  `age.NewScryptRecipient` at log2N=18 the whole file used to use.
- the recovery key is 160 bits rendered as 32 Crockford base32 characters.

### Why not simply add a second age recipient

age files support several recipients natively, and the file key is already wrapped per recipient —
on paper this is the same design for none of the work. It does not survive contact with how the
vault is written.

`VaultRepo` re-encrypts and rewrites the file on every flush, and a flush fires whenever a connection
is edited. Re-encrypting to *both* recipients requires *both* credentials in memory. After an unlock
with the password, the recovery key is not there and cannot be derived. The first save after any
password unlock would therefore write a file with one recipient, silently revoking the recovery key
that the user has on paper and believes still works.

That failure is invisible until the day it matters. The indirection is what removes it: a flush
re-encrypts the payload to a key the session already holds, and carries the other credential's wrap
across untouched without ever knowing what opens it.

### Consequences we chose deliberately

- **The repository holds the vault key, not the master password.** The password is used once, to
  unwrap, and dropped. This was not the goal and is the largest single security improvement here.
- **A flush no longer runs scrypt.** The ~256 MiB transient cost moved from every edit to the unlock
  and to credential changes.
- **The envelope is a version of its own, not a vault schema bump.** The vault schema describes the
  JSON *inside* the ciphertext and is unreadable without a credential; the envelope is outside it and
  readable with none. A build that cannot open a file has to say which of the two is wrong, and only
  a number outside the encryption can tell it. Bumping `CurrentVaultVersion` instead would also have
  triggered the key-migration wizard on every existing vault, for a migration with nothing to do.
- **Conversion happens on the first unlock**, after `vault.BackupVaultFile` keeps the original bytes.
  Those backups stay readable under the password in force when they were written; a later password
  change does not reach into them and nothing deletes them. That is a real limitation, recorded in
  the changelog rather than papered over.

### Rules the implementation must keep

1. **One error for every failed credential.** A wrong password and a wrong recovery key return
   `ErrVaultDecryptFailed` worded identically. Distinguishing them tells someone holding the file
   which wrap is the cheaper target.
2. **Both wraps are tried whatever the input looks like.** The shape of the input only picks the
   order. Skipping a wrap on shape would refuse a master password that happens to look like a key.
3. **Separate throttles, chosen by shape.** A key is read off paper, so it gets one free attempt and
   a five-second-to-five-minute backoff against the password's three and one-to-thirty. They are
   independent, so neither credential's failures can lock out the other. Deciding which counter to
   charge from the shape of the input discloses nothing: it is a fact about characters the caller
   just typed.
4. **`VerifyMasterPassword` refuses the recovery key.** Re-authentication before exporting a private
   key or trusting a plugin must not accept the credential most likely to be on paper beside the
   machine.
5. **The key is shown once and never stored.** The backend holds it only while its dialog is open, so
   the Save As handler can write the file without the key crossing back from the WebView. Pressing
   Done drops it. Nothing derived from it is written to the vault, to settings, or to the audit log —
   the audit database is readable while the vault is locked, so a stored hash would be an offline
   verifier.
6. **Any credential change revokes every earlier one.** A password change issues a new recovery key
   too, because a password change is what people do when they believe their credentials have been
   seen.

## Alternatives rejected

- **A second age recipient on the same file.** See above; it revokes the recovery key on the first
  flush after a password unlock.
- **Storing `Enc_recoveryKey(masterPassword)` as a sidebar blob.** Keeps the password as the thing
  the write path needs, so it stays in memory for the life of the process and the recovery path
  becomes "recover the password" rather than "open the vault". Strictly worse on both counts.
- **Escrow, a support-recoverable key, or any server.** xQuakShell has no server and is not going to
  grow one to hold the credential that opens every vault it ships.
- **A shorter key, or one derived from a passphrase the user picks.** The point of the second
  credential is that it has entropy the user could not have chosen. A memorable recovery phrase is a
  second password, with a second chance to forget it.
