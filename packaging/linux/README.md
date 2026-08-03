# xQuakShell — portable Linux archive

Everything in this directory belongs to one installation. Unpack it wherever you like — a home
directory, a USB stick, a shared drive — and run it from there. Nothing is written outside this
directory, nothing needs root, and there is no installer.

## Run it

```sh
./xquakshell.sh
```

The launcher starts `xQuakShell` next to it. Starting the binary directly works too; the launcher
exists so that a desktop entry can find it and so that a missing system library is reported as an
instruction rather than a window that never opens.

## Add it to the application menu

```sh
./install-desktop-entry.sh
```

This writes a `.desktop` file into `~/.local/share/applications/` pointing at this directory, which
is what puts xQuakShell in the menu with its icon. The application itself stays here. To undo:

```sh
./install-desktop-entry.sh --remove
```

Move this directory and the entry stops working — run the script again from the new location.

## Which archive is this

xQuakShell draws its interface with WebKitGTK, which is part of your system rather than something
that can be shipped in an archive. Distributions carry one of two incompatible versions of it, so
each release has two Linux archives:

| Archive | WebKitGTK | Typical systems |
|---------|-----------|-----------------|
| `…-linux-amd64-webkit4.1.tar.gz` | 4.1 | Ubuntu 22.04 and newer, Debian 12+, Fedora 40+, Arch |
| `…-linux-amd64-webkit4.0.tar.gz` | 4.0 | Older systems still carrying the 4.0 runtime |

Prefer the 4.1 archive. Version 4.0 was retired upstream in 2026 and the 4.0 archive is there for
systems that have not moved yet. Both are the same application, built the same way.

If you picked the wrong one, the launcher says so and names the package to install.

Both archives are built against glibc 2.35 (Ubuntu 22.04), so they need that version or newer.

## Where your data lives

In `data/`, next to the binary:

- `data/vault.age` — the encrypted vault: connections, credentials, keys
- `data/audit/` — the audit log
- `data/plugins/` — installed plugins and their own data

Back up `data/`, or copy the whole directory, and your setup moves with it. Deleting this directory
removes the application and its data together.

## If the window is blank or does not appear

Some GPU and virtual-machine driver combinations do not work with WebKitGTK's default rendering
path. Both of these are WebKitGTK settings, not application settings:

```sh
WEBKIT_DISABLE_DMABUF_RENDERER=1 ./xquakshell.sh   # try this first
WEBKIT_DISABLE_COMPOSITING_MODE=1 ./xquakshell.sh  # then this
```

If one of them helps, put it in the `Exec=` line of the installed desktop entry so the menu launcher
uses it too.

## Verifying the download

The release publishes `SHA256SUMS` alongside the archives:

```sh
sha256sum -c SHA256SUMS --ignore-missing
```

---

Documentation and source: https://github.com/teoritty/xQuakShell
