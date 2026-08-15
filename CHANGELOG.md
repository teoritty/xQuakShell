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

## [1.2.0]

### Compatibility

| Axis | Version |
|---|---|
| `pluginApi` | 1.0.0 (unchanged) |
| Capabilities | `session` 1.0.0 — gains the `trust.verifyPeer` RPC, gated on an existing capability; all others 1.0.0, unchanged |
| Manifest schema | unchanged |
| `bundleFormat` | 1.0.0 (unchanged) |
| Vault schema | 4 (unchanged — `peerTrust` is an additive, omitted-when-empty field) |
| Audit schema | 1 (unchanged) |

### BREAKING

*Nothing.*

### Added

**Peer trust: `known_hosts` for protocols that are not SSH.** A plugin that has observed the
identity of the peer it connected to can ask the host whether it is trusted, through the new
`trust.verifyPeer` RPC. An unknown or changed identity stops the session in a new `trust-required`
state and asks the user, showing a `SHA256:` fingerprint the host computes itself. Answering
"trust" records the material in the vault and reconnects; answering "reject" fails the session.

The mechanism is deliberately separate from `known_hosts` in every layer — its own vault field, its
own repository, its own dialog — so that no path exists from a plugin to the trust the SSH client
reads. What is trusted is decided by the host and not by the caller: the subject comes from the
connection record, the scope from the session binding, and the fingerprint is computed from the
bytes that will be stored. A plugin that names a subject other than the one its session connects to
is refused rather than prompted for.

**Trusted Peers**, a new screen next to Known Hosts, lists every recorded identity and revokes any
of them; the next connection to a revoked peer asks again. Trust questions, decisions and
revocations are written to the audit log, without the material.

**Plugin UI assets may now include `.wasm`.** The plugin asset CSP gains `'wasm-unsafe-eval'` —
the narrow opt-in that permits WebAssembly compilation and nothing else, already in force on the
embed broker — and `.wasm` responses carry an explicit `application/wasm`.

## [1.1.0] — 2026-08-12

### Compatibility

| Axis | Version |
|---|---|
| `pluginApi` | 1.0.0 (unchanged) |
| Capabilities | `session` 1.0.0 — `localEmbedServer` feature removed · all others 1.0.0, unchanged |
| Manifest schema | `capabilities.session.localEmbedServer` removed |
| `bundleFormat` | 1.0.0 (unchanged) |
| Vault schema | 4 (was 3 — migrated on first unlock) |
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

**The vault on-disk schema moves from 3 to 4, and every existing vault is migrated.** Private keys
are no longer stored in the shape they were imported in. Each key is now an OpenSSH private key
encrypted with bcrypt_pbkdf: an unprotected key you imported is re-wrapped under a random data key
held in the vault, and a key with its own passphrase keeps that passphrase. Before this, importing
`~/.ssh/id_ed25519` put a directly usable private key into the vault snapshot, protected only by
the vault being locked — which it is not while you are using the application.

**What you will see.** The first unlock after upgrading opens a migration screen instead of the
normal one. It lists every key that carries its own passphrase and asks for each, once. Keys
without a passphrase are converted with nothing to answer. Before anything is written, the vault is
copied to `vault.age.v3.bak` in the same directory; that copy is never overwritten and nothing
deletes it.

**A passphrase you cannot remember does not cost you the vault.** Any key can be skipped. A skipped
key keeps its original bytes, still authenticates exactly as before, and is marked as needing
attention in the key manager, where you can finish or delete it later. A passphrase entered wrongly
is treated the same way rather than failing the upgrade — the alternative would lock you out of
every connection and known host over one forgotten key.

**Downgrading is not supported.** An older build refuses a schema 4 vault by design. To go back,
restore `vault.age.v3.bak` over `vault.age`.

**No plugin version axis moved, and that is deliberate.** `pluginApi` versions the protocol envelope —
framing, handshake, lifecycle, error space — none of which changed. The `session` capability did not
take a major either: capability majors are matched exactly, so bumping it would refuse every plugin
that grants `session` until its manifest named the new number, including plugins whose behaviour is
identical either way, while catching nothing the per-feature check does not already catch. The
removal is recorded by name in `removedFeatures` (`api_contract_test.go`) and `removedSchemaFields`
(`manifest_schema_test.go`) instead, where it is reviewed rather than inferred from a number.

### Added

- **A key manager, so your SSH keys can live in the vault instead of in `~/.ssh`.** Generate a key
  (ed25519, RSA or ECDSA), import one you already have, rename it, change or remove its passphrase,
  delete it, export it, and publish its public half to a server you are already connected to. Each
  key shows its SHA256 fingerprint and its public key, and lists the connections using it.

  Every key is stored encrypted, in the format `ssh-keygen` writes. You choose what protects each
  one:

  - **The vault** — unlocking the vault is enough to use the key. This is what an imported key with
    no passphrase of its own becomes; it is no longer kept as a plain file inside the vault.
  - **A passphrase you set** — the key stays shut even while the vault is open. Nothing stored
    anywhere opens it without you.

  Per key you can also decide how long an entered passphrase is remembered (until the vault locks,
  for a set number of minutes, or never), and whether plugins may read it.

  **A connection picks its key from the manager, not from a file.** Key authentication now offers
  the keys the manager holds, with their fingerprints, and a way to add a new one without leaving
  the dialog. The old "Import Key" button, which put a file straight into the vault with no name
  and no choices, is gone — along with the RPCs behind it, so there is one way in and it is the one
  that asks the questions that matter.

  A key can be marked **never exportable** when you create it. That is permanent by design: a
  promise that a key cannot leave the vault is worth nothing if a checkbox can take it back.
  Exporting any other key asks for your master password and is recorded in the audit log.

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

### Security

- **Changing the plugin trust policy now asks for your master password.** Adding a trusted publisher
  key, turning off *Require signed plugins*, allowing the unsandboxed fallback, or granting a plugin
  a capability all take effect only after the password is re-entered.

  These settings are the root of trust for plugin signatures: the list of trusted publisher keys is
  the only thing a manifest signature is ever checked against. They were writable through the same
  Wails bridge the UI uses, with no confirmation, so anything that got hold of that bridge — a script
  injected into the UI, a plugin with a WebView — could add its own key and then install its own
  plugin showing a *signature verified* badge. The master password never crosses that bridge, which
  is what makes it the right thing to ask for.

  **Tightening the policy still costs nothing.** Revoking a key, turning the signature requirement
  on, or taking a grant away go through without a prompt. A security control that charges you to
  switch it on is one that stays off.

- **A plugin release asset is no longer installed without a checksum to verify it against.** If a
  release publishes no `SHA256SUMS` (or `checksums.txt`), or leaves the asset you are installing out
  of it, the install now stops. The `.xqsp` bundle path has always required checksums; the
  bare-binary path skipping them was an asymmetry rather than a policy, and it left TLS to
  github.com as the only thing between a tampered release and your machine.

- **A failed `SHA256SUMS` download is now an error instead of "this release has no checksums".**
  The two were the same value internally, so a rate limit, a 5xx, or a dropped connection silently
  turned integrity checking off and nothing reported it. Failing one HTTPS request — not forging it,
  just failing it — was enough to get an unverified install.

- **The plugin binary inside a release archive is taken from the path the manifest declares.** It
  used to be found by searching the whole extracted tree for the file's *base* name and taking
  whichever the directory walk reached first. An archive containing both `a/plug` and `bin/plug`
  installed `a/plug`, because `a` sorts before `bin` — so whoever wrote the archive could plant a
  decoy beside the real binary and the decoy is what got the execute bit and got launched. A
  tarball wrapped in a single top-level directory, which is how they are usually built, still works;
  anything more ambiguous is now refused rather than guessed.

- **A new vault requires signed plugins by default.** This was off because the setting is a boolean
  that defaults to false, not because anyone chose it. Existing vaults keep whatever they have —
  false there may have been deliberate, and overriding a security setting a user picked is its own
  kind of wrong. Those installations are covered by the mandatory checksum above and by the master
  password now being required to turn the setting off.

- **The host key you accept is now the one the server actually presented.** The prompt used to send
  the host name and the key back to the backend as arguments, so the frontend was telling the
  backend what to trust. Anything reaching the Wails bridge could call it with a host it had never
  connected to and a key of its own, and silently replace the recorded key for, say, your production
  server. The backend now reads both from the session that is waiting on the prompt; the only thing
  the UI supplies is your answer.

- **`AddKnownHost` is gone.** It wrote an arbitrary host-and-key pair straight into the vault with no
  connection, no prompt and no confirmation — and nothing in the UI used it. Trust for a host the
  user had never reached could be planted in advance, and the first-connection prompt then never
  appeared because the key already matched. Removing a known host is unchanged.

- **A host with more than one key no longer raises a false "host key changed" alarm.** OpenSSH keeps
  several keys per host — commonly Ed25519 and RSA — and the check stopped at the first entry whose
  host matched. If the server negotiated the key recorded second, you were told the host key had
  changed and you might be under attack, on a perfectly good connection. Every recorded key for the
  host is now considered; a mismatch is reported only when none of them match.

- **Accepting a changed key keeps the host's other keys.** It used to delete every entry for the
  host. Combined with the false alarm above, one spurious warning permanently destroyed a key you
  had verified. Only the key of the same type is replaced.

- **Hashed `known_hosts` entries are understood.** Trust imported from an OpenSSH file written with
  `HashKnownHosts` matched nothing, so hosts you had already verified came back as unknown and you
  were walked through first-contact verification again — which is exactly the moment worth
  attacking.

- **A remote forward that binds beyond the SSH server's loopback now has to be asked for.** Such a
  rule publishes whatever its target resolves to *on your machine* to every host that can reach the
  server — a target of `127.0.0.1:22` means your own SSH daemon, on the server's network. It was
  reachable by writing one string into a saved connection, with nothing to acknowledge and nothing
  shown. Ordinary loopback remote forwards are unaffected.

  **If you already have such a rule** it will refuse to start until you tick the new acknowledgement
  on it, in the connection's forward rules.

- **A loopback-only forward now verifies where it actually bound.** The rule was checked as a
  string, and `localhost` means whatever the resolver says it means — a line in the hosts file
  points it at an external interface and a forward the validator called loopback-only listens to
  the local network. The listener's real address is now checked, and one that landed somewhere
  routable is closed rather than served.

- **The SSH algorithms this client will negotiate are now written down instead of inherited.** They
  used to be whatever the vendored crypto library defaulted to, which moves when the dependency is
  bumped, with no review and nothing that fails when it does. SHA-1 host key signatures (`ssh-rsa`),
  DSA, the SHA-1 key exchanges, CBC ciphers, RC4 and HMAC-SHA1 are all excluded; everything a
  current OpenSSH server offers is kept.

- **A server offering an RSA host key below 2048 bits is refused.** Algorithm negotiation cannot
  catch this — `rsa-sha2-256` is a sound signature algorithm and says nothing about the size of the
  key signing with it — so a 1024-bit host key arrived through a policy that correctly refused
  `ssh-rsa`. The connection now stops before the trust prompt, because accepting it would record a
  key an attacker can factor as the thing every later connection is checked against.

- **Master password attempts are now rate-limited.** There was no limit of any kind: the unlock
  call went straight through to the vault, so anything that could reach the Wails bridge could guess
  in a loop as fast as the machine allowed. The key derivation made each guess expensive but never
  made the ten-thousandth harder than the first.

  Three attempts cost nothing; after that the wait doubles from one second, capped at thirty. It is
  a delay and not a lockout, and the count is deliberately not written to disk — a vault that
  refuses its owner after N wrong guesses is a denial of service anyone who can reach the prompt can
  trigger. Repeated failures are logged.

- **A plugin can no longer raise its own resource limits.** `channel.maxConcurrent`,
  `channel.maxThroughputKbps` and a tunnel provider's `maxConcurrentChannels` were read out of
  `plugin.json` and used as the host's limit whenever they were greater than zero — but
  `plugin.json` is the plugin's own file, so a plugin asking for a million concurrent channels got
  a million. Declaring *less* than the host default still works and is still honoured; declaring
  more is now capped.

- **The install screen now names the filesystem paths a plugin is asking for.** It used to say
  "Read files in declared sandbox paths" and list none of them, while nothing validated what was
  declared — so a plugin asking for `/` or `C:\` was presented to you in the language of a sandbox.
  The paths are listed the way the outbound network patterns always have been, and a grant that
  covers your whole filesystem or home directory is called out as such.

- **Uninstalling a plugin now revokes what you granted it.** Capability grants — secret access,
  auth provider, tunnel provider, multi-session, arbitrary network — were stored against the
  plugin's id and left behind when the plugin was removed, along with its disabled marker. The id
  is chosen by the plugin author and verified against nothing, so anything installed later under
  that id inherited consent you gave to something else. Uninstalling is the natural way to take a
  permission back, and now it does.

- **A plugin download can no longer be redirected off TLS.** The URL for a release asset arrives
  inside the GitHub API response rather than being built locally, and it was followed with the
  default HTTP client — which walks up to ten redirects anywhere, including from `https` to plain
  `http`, and says nothing. The asset itself is checksum-verified now, but `SHA256SUMS` is fetched
  by the one path that cannot be (it is the list), so the transport is the only thing protecting
  the file everything else is checked against. A chain that starts on TLS must stay on it, and a
  redirect onto anything that is not HTTP or HTTPS is refused outright.

- **Declining a plugin install no longer leaves it on disk.** The plugin was copied into place
  before your consent was checked, and a refusal returned an error without removing it — so the
  files stayed, and the next start of the application discovered them and loaded the plugin you had
  just declined. A refused install is now cleaned up.

- **Deleting a plugin's `SHA256SUMS` no longer disables its integrity check.** An installed plugin is
  verified against that list every time it loads, but the check fell through to "fine" whenever the
  list simply was not there — so anyone able to edit a plugin's files could also remove the file they
  would have been checked against, and the modified plugin loaded silently. Every plugin you install
  is given a `SHA256SUMS`, so its later absence is not a plugin that shipped without one; it is a
  plugin that had one and does not any more, and that now refuses to load.

- **A plugin can no longer write terminal control characters into the log.** Its output reaches the
  log window, the application log and the process stderr a developer or a CI job reads. The window
  renders it as text, but a terminal obeys what it is sent — a carriage return rewrites the line
  already printed and an escape sequence clears the screen, so a plugin could compose a log line
  saying whatever it liked and erase the one above it. Control characters are now stripped; tabs and
  ordinary text, including non-Latin scripts, are untouched.

- **A plugin can no longer flood the application with concurrent requests.** Every request a plugin
  sent was handled on its own goroutine with no ceiling, and only log writes were rate limited — so a
  plugin could issue them as fast as the pipe carried them and the application grew a goroutine for
  each one. A plugin's own process is memory-capped; the application is not. At most 64 of a
  plugin's requests are now handled at once, and the rest are answered with a rate-limit error
  rather than queued.

- **A recursive permission change no longer widens itself when something goes wrong.** Recursive
  chmod and chown take a scope — files, directories, or both — and an unrecognised value became
  *both*, the widest of the three. So a typo, or a mismatch between the interface and the backend,
  quietly applied the change to everything under the directory instead of the part you asked for.
  An unrecognised scope is now an error, and a scope that was never set changes nothing at all.

- **A hostile server can no longer hang the client with an endless directory tree.** Walking a remote
  directory — to plan a download, or to apply a recursive permission change — recursed as far as the
  server said the tree went, and the structure is the server's to invent. A tree with no bottom costs
  a compromised or malicious server nothing to serve, and the client either ran out of stack or grew
  its file list until it ran out of memory; stopping it needed you to notice and cancel. Walks now
  stop at a depth and an entry count, and say so rather than quietly returning half a tree.

- **A remote file whose name lies about what it is no longer reaches your disk.** The SFTP server
  controls every byte of a directory listing, and three shapes were getting through: a name carrying
  a right-to-left override, which renders `gnp.<RLO>exe` as `exe.png` so the extension you read is
  not the extension the file has; a Windows device name like `CON` or `NUL.log`, where writing the
  download opens the console and the transfer reports success having stored nothing; and a trailing
  dot or space, which Windows silently strips so `report.txt.` overwrites `report.txt`. Ordinary
  names in any script — Arabic, Hebrew, Cyrillic, CJK — are unaffected, and the Windows rules apply
  only on Windows, where they are rules.

### Fixed

- **A plugin's channel can no longer be pointed somewhere it was never authorized for.** Each
  channel resolves and checks its destination when it is opened, and the code required — in a
  comment — that every open get its own backend to hold that decision. Nothing enforced it, so a
  wiring change that shared one backend would have let a second channel overwrite the destination
  the first was approved for: the authorization still happens, for a target that is no longer the
  one in use. Every backend now refuses to be authorized twice.

- **A plugin can no longer exhaust the application's memory by listing a directory, or ask the
  filesystem for a file of exabytes.** Listing read every entry of a directory before anything was
  filtered, and a plugin can fill a directory it is allowed to write to; the plugin's own process is
  memory-capped, the application is not. Listings are now bounded, and a directory past the bound is
  refused with a distinct error rather than quietly returned short. Separately, a write at an offset
  near the maximum of a 64-bit integer wrapped the size check into a negative number and passed it,
  reaching a seek far beyond any legal file size. The offset is now bounded before it is added to.

- **"Do not allow private networks" now covers the networks Go does not call private.** A plugin
  granted arbitrary outbound access, with private networks declined, could still reach anything in
  `100.64.0.0/10` — the range Tailscale, ZeroTier and carrier-grade NAT use, which is to say the
  user's own machines — because the standard library's definition of "private" is RFC 1918 and
  nothing else. The same held for `0.0.0.0/8`, which reaches the local host on Linux and macOS, and
  for an IPv4 address smuggled inside an IPv6 one: `64:ff9b::7f00:1` and `2002:7f00:1::` are
  127.0.0.1 on a network that routes them, and both read as ordinary public addresses. All of these
  are refused now. A plugin whose manifest names such an address outright still reaches it — that is
  consent you gave when you installed it.

- **A download that fails partway no longer destroys the file it was replacing.** The local file was
  opened and truncated before the first byte arrived, so a dropped connection left you with neither
  the old copy nor the new one — and what remained wore the real name, with nothing to say it was
  incomplete: a truncated archive, a config missing its tail, a binary that no longer runs. A
  dropped connection costs a hostile server nothing to arrange. Downloads now write alongside the
  destination and take its name only once the last byte has landed, so an interrupted one leaves
  what was already there untouched. Downloaded files are also no longer created readable by every
  other account on the machine.

- **The number of concurrent transfers and the transfer connection timeout are now bounded on the
  backend.** Both had a floor and no ceiling, unlike every other numeric setting. The settings
  dialog offers 1–16 and 5–300 seconds, but an HTML `max` attribute constrains the dialog, not the
  RPC behind it — and the concurrency figure becomes the transfer slot count directly, so an
  out-of-range value is that many simultaneous SFTP operations with their goroutines and buffers.
  The stored bounds are the ones the dialog already showed, so no existing configuration changes.

- **A plugin can no longer read any private key belonging to a connection it was invoked for.**
  Holding the vault capability used to be enough to be handed the raw private key, and the cached
  passphrase with it — a decision far too coarse for "this third-party binary may read this
  particular key". Both are now off unless you turn them on for that key, including for keys that
  predate the setting, and every release is recorded in the audit log by fingerprint.

- **A remembered key passphrase can now be made to expire.** It used to be held until the vault
  locked, with no way to bound it, so an unlocked machine left unattended went on being able to
  authenticate indefinitely.

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
- **The Windows executable now carries a readable version.** Its properties dialog showed no
  product name, product version or copyright, because the version resource Wails ships by default
  labels its string table with the language-neutral id `0000`, which `VerQueryValue` cannot look
  up. The build now labels it `0409` and stamps the numeric product version as well as the file
  version, so the Details tab agrees with the version the About panel reports.

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
