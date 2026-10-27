# xqsyncd — the xQuakShell sync server

`xqsyncd` is the server the xQuakShell clients sync through ([ADR-023](adr/023-state-sync.md)). It
is a single static binary with an embedded SQLite database. It stores ciphertext, public keys and
signed logs, and **cannot open any vault it holds**: every record is encrypted and signed on the
user's devices before it leaves them.

This guide is for the person running the server. Users connect from **Settings → Sync** in the
application.

## What the server can and cannot see

| The server sees | The server never sees |
|---|---|
| Usernames (which may be pseudonyms) and how many devices each account has | Any field of any record: host names, addresses, user names, connection and folder names |
| Random vault ids and which accounts share which vault, with which role | A record's type, plugin ids, vault names |
| Record counts, sizes rounded up by padding, commit times, client IP addresses | Passwords, private keys, vault keys |
| Account and device public keys | Anything from which a password can be guessed offline **without** `server.key` |

A server you do not trust cannot read, forge or silently roll back a vault. It can withhold data
or show two devices different histories; clients detect the second the moment they see the other
branch (see the [security model](security-model.md#state-sync-adr-023)).

## Install

Release archives: `xqsyncd-<version>-windows-amd64.zip`, `xqsyncd-<version>-linux-amd64.tar.gz`,
`xqsyncd-<version>-linux-arm64.tar.gz`. Check them against the release's `SHA256SUMS` before
installing.

### Linux (systemd)

```sh
sudo install -m 0755 xqsyncd /usr/local/bin/xqsyncd
sudo install -m 0644 xqsyncd.service /etc/systemd/system/xqsyncd.service
sudo systemctl daemon-reload
sudo systemctl enable --now xqsyncd
journalctl -u xqsyncd | grep fingerprint
```

The unit runs the server as a transient user (`DynamicUser=yes`) that owns only
`/var/lib/xqsyncd`, with the filesystem read-only, no capabilities, and a system-call filter. The
first start creates the key file and a self-signed TLS certificate for the machine's host name and
prints the **identity fingerprint**. Later starts change nothing.

To listen on a port below 1024, set `CapabilityBoundingSet=CAP_NET_BIND_SERVICE` and
`AmbientCapabilities=CAP_NET_BIND_SERVICE` and change `--addr` in `ExecStart`. To serve a
certificate of your own, add `--tls-cert` and `--tls-key` (see below).

### Windows (service)

From an elevated prompt, with `xqsyncd.exe` in a permanent place such as
`C:\Program Files\xqsyncd\`:

```bat
xqsyncd.exe service install --host sync.example.org
```

This registers the service `xqsyncd` under its own virtual account `NT SERVICE\xqsyncd` (no
password, not LocalSystem), creates `%ProgramData%\xqsyncd` with an access list that grants only
Administrators and that account, initialises it, starts the service and prints the fingerprint.
`--name`, `--dir` and `--addr` (default `:8443`) change the defaults. The key file is also sealed
with DPAPI, so a copy of the directory taken off the machine does not carry the keys in the clear.
The server logs to the Windows event log under the service's name.

`xqsyncd.exe service uninstall` stops and removes the service and **keeps the data directory**.

### Docker

Every release publishes a multi-architecture image (`linux/amd64`, `linux/arm64`):

```text
ghcr.io/teoritty/xqsyncd:1.5.0    exactly this release
ghcr.io/teoritty/xqsyncd:1.5      the newest 1.5.x
ghcr.io/teoritty/xqsyncd:1        the newest 1.x
ghcr.io/teoritty/xqsyncd:latest   the newest release
```

Pre-releases are published under their own version only. The image holds the server binary and
nothing else - no shell - and runs as an unprivileged user. The data directory is `/data`; put it
on a volume.

With Compose, take [`packaging/docker/compose.yaml`](../packaging/docker/compose.yaml), replace
`sync.example.org` with the name your users connect to, and:

```sh
docker compose up -d
docker compose logs xqsyncd | grep -E 'fingerprint|connect link'
```

Without Compose:

```sh
docker run -d --name xqsyncd --restart unless-stopped -p 8443:8443 -v xqsyncd-data:/data \
  ghcr.io/teoritty/xqsyncd:1 serve --dir /data --addr :8443 --init --host sync.example.org
```

`serve --init` creates the key file and the certificate on the first start and changes nothing on
later ones; `--host` only names the server in the printed connect link. Every command below runs in
the container as the binary itself:

```sh
docker exec xqsyncd xqsyncd admin invite create --dir /data
docker exec xqsyncd xqsyncd fingerprint --dir /data --host sync.example.org --port 8443
```

The volume holds both `server.key` and `xqsyncd.db`, which must not be backed up together (see
[Backups](#backups)). Do not snapshot or copy the whole volume as your backup. Copy `server.key` out
once, to a separate place, and back the database up with `admin backup` into a directory mounted
for it:

```sh
docker cp xqsyncd:/data/server.key ./server.key   # once; store it apart from the database backups
docker run --rm --volumes-from xqsyncd -v xqsyncd-backups:/backup \
  ghcr.io/teoritty/xqsyncd:1 admin backup --to /backup --dir /data
```

A named volume at `/backup` is writable as it comes. A host directory mounted there instead must be
writable by uid 65532, the user the image runs as.

Behind a reverse proxy every client arrives from the proxy's address and shares one rate limit (see
[Limits](#limits)); raise `--rate-per-second` and `--burst` on `serve` accordingly. The proxy must pass
TLS through rather than terminate it: clients pin the certificate the server's identity key signed.

To upgrade, pull the new tag and recreate the container; the volume carries everything over.

### Anywhere else

```sh
xqsyncd init --dir /srv/xqsyncd --host sync.example.org
xqsyncd serve --dir /srv/xqsyncd --addr :8443
```

`serve --init --host sync.example.org` does both in one command and is safe to run on every start.
Keep the directory readable by the server's own user only.

## First connection: the fingerprint

The fingerprint is twelve groups of five digits, for example `04512 99031 …`. Print it again at any
time:

```sh
xqsyncd fingerprint --dir /var/lib/xqsyncd
```

When a user enters the server address, the application shows the fingerprint the server presents
and asks the user to compare it with the one you gave them, over a channel other than the network
they are connecting through. Once confirmed, the client pins the server's identity key and refuses
any other — including a certificate an attacker on the path managed to get issued.

### The connect link

`init`, `fingerprint` and every `serve` start also print a **connect link** — the server's address
with the fingerprint in its fragment:

```text
connect link: https://sync.example.org:8443/#fp=04512-99031-…
```

Give users the link instead of the address and the fingerprint separately. Pasted into
Settings → Sync, it fills the address and makes the comparison for them: the application connects
only if the server presents exactly that fingerprint, and refuses with nothing sent otherwise. The
fragment never reaches the server. Hand the link over the same kind of channel you would use for
the fingerprint — whoever can change the link in transit can change a fingerprint read out there
too.

`fingerprint` names the certificate's first host and port 8443; `--host` and `--port` override
both, for a server behind a different name or port. `serve` logs the port it actually listens on.

## TLS certificates

`init` creates a self-signed certificate, and that is sufficient: clients do not trust TLS
certificate authorities for this connection, they check that the certificate is the one the
server's identity key signed. You can serve a certificate of your own (for example from your
existing ACME setup) with `serve --tls-cert FILE --tls-key FILE`. Replacing the certificate needs a
restart and nothing from users: the identity key, and therefore the fingerprint, stays the same.

## Accounts and invites

Registration needs an invite. There is no administrative network API; every command below works
on the data directory directly, on the server machine.

```sh
xqsyncd admin invite create [--ttl 24h] --dir D    # prints a one-time code; give it to one person
xqsyncd admin invite list --dir D                  # id, expiry, used or unused - never the code
xqsyncd admin invite revoke ID --dir D             # deletes an unused invite
xqsyncd admin account list --dir D
xqsyncd admin account freeze NAME --dir D          # ends every session, refuses logins
xqsyncd admin account unfreeze NAME --dir D
xqsyncd admin account delete NAME --dir D          # the account and every vault only it belongs to
xqsyncd admin stats --dir D
```

Freeze an account when its owner reports a lost password or a device you cannot otherwise stop. You
cannot read an account's data, reset its password, or add anyone to its vaults; nobody can.

## Limits

Each client address may make 20 requests a second (bursts of 200); the application waits when told
to. Each personal vault may hold 256 MiB of ciphertext and each team vault 1 GiB; change these with
`serve --personal-quota-mib N --team-quota-mib N`. Every vault may also hold at most 100 000
records, items in the trash included - verifying a commit costs time in proportion to the number of
records, and the byte quotas alone do not bound that; change it with `serve --max-records N`.

Requests are limited per client address: 20 a second sustained, 200 at once. A client's sync pass
fetches the device log and membership of every vault it syncs, so a member of many teams needs the
burst. Users behind one NAT share one address; raise both with
`serve --rate-per-second N --burst N` if they report "Too many attempts". Sign-in attempts have
their own backoff per username and address, independent of this limit.

Across all clients the server serves 64 requests at once and asks the rest to retry a second
later, so a crowd arriving together cannot exhaust its memory; raise it with
`serve --max-requests N` on a large host. Open notification sockets do not count against it.

## Backups

The data directory holds two things that must be backed up **differently**:

| File | Back up | Why |
|---|---|---|
| `server.key` | Once, after `init`, somewhere other than the database backups | Without it no account can log in, and every user sees a different fingerprint. With it, a database copy becomes a target for offline password guessing — so never store the two together |
| `xqsyncd.db` | Regularly, with `admin backup` | Everything else: accounts, vaults, history |

```sh
xqsyncd admin backup --to /var/backups/xqsyncd --dir /var/lib/xqsyncd   # --max-age 720h by default
```

`admin backup` writes a consistent copy while the server keeps running and deletes earlier backups
older than `--max-age`. Keep backups no longer than the users' trash window (30 days unless a vault
owner changed it): a backup outliving it still holds what users deleted and shredded, readable with
the password valid at the time.

To restore, stop the server, put the backup in place as `xqsyncd.db` beside the original
`server.key`, and start it. Devices that synced after the backup was taken detect that the server
went back in time and stop syncing until their owners act; this is the same alarm a malicious
rollback raises, by design.

## Upgrades

Replace the binary and restart. A newer server that changes the database format copies the file
aside before converting it. An older server refuses a database written by a newer one rather than
damage it — to go back, restore the copy.

## Logs

Linux: `journalctl -u xqsyncd`. Windows: Event Viewer → Windows Logs → Application, source
`xqsyncd`. The log records operations, account and vault ids and sizes; it never contains a record
body, a password or a key.
