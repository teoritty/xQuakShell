# xQuakShell Usage Guide

This guide explains how to use xQuakShell for managing remote connections, organizing your infrastructure, and working with sessions.

---

## Table of Contents

1. [Vault and Master Password](#vault-and-master-password)
2. [Connections](#connections)
3. [SSH Connections](#ssh-connections)
4. [Authentication](#authentication)
5. [Jump Chains (Bastion)](#jump-chains-bastion)
6. [Folders and Organization](#folders-and-organization)
7. [Importing Connections](#importing-connections)
8. [Sessions and Tabs](#sessions-and-tabs)
9. [Tiling the Workspace](#tiling-the-workspace)
10. [Terminal](#terminal)
11. [Local Terminal](#local-terminal)
12. [SFTP File Transfer](#sftp-file-transfer)
13. [Port Forwarding](#port-forwarding)
14. [Agent Forwarding](#agent-forwarding)
15. [Plugins](#plugins)
16. [Sync Between Devices](#sync-between-devices)
17. [Teams](#teams)
18. [Known Hosts](#known-hosts)
19. [Audit Log](#audit-log)
20. [Settings and Lockout](#settings-and-lockout)

---

## Vault and Master Password

All data—connections, SSH keys, passwords, known hosts—is stored in a single encrypted file: `vault.age`. The vault is protected by a **master password**.

### First launch

- Enter a password of at least 8 characters to **create** a new vault. It becomes your master
  password. A strength meter rates it, but only the length is enforced — a weak-but-long-enough
  password creates the vault with a warning. (Turning on [sync](#sync-between-devices) needs at
  least 12.)
- The vault then shows its **recovery key** — see below. Tick the box confirming you have read what
  losing it means.
- On subsequent launches, enter the same password to **unlock** the vault. Attempts are
  rate-limited.

### Recovery key

Every vault has a second credential: thirty-two characters, accepted in the same field as the
master password. Forget the password and type the key instead; the next screen asks for a new
password and issues a new key, and the ones you just used stop working for good.

The key is shown **exactly once**, on a dialog that cannot be dismissed by clicking away or pressing
Escape, and whose **Done** button counts down for fifteen seconds. **Copy** puts it on the
clipboard; **Download** saves it as a text file wherever you choose. Once you press Done, nothing —
not the application, not the file on disk — can produce that key again. Lose both the key and the
password, and the vault is gone.

Dashes, spaces and case are ignored when you type it back, and the alphabet has no I, L, O or U, so
a zero is never read as an O.

**Settings → Security** changes the master password and issues a new recovery key. Changing the
password always issues a new key and revokes the old one. A vault that never had its key confirmed
— the application closed before **Done** — gets one there too.

### Where is data stored?

xQuakShell is **fully portable**. All application data lives **next to the executable**:

```
xQuakShell.exe
data/
  vault.age            # encrypted vault
  audit.db             # audit log (if enabled)
  github-repos.json    # plugin repositories you have added
  plugins/             # installed plugins, one directory per plugin ID
    com.example.myplugin/
      data/            # that plugin's own runtime data
  sync/
    state.db           # sync cursors and pending changes (only once sync is on)
  locales/             # optional: your own interface translations, <code>.json
  tmp/                 # staging area for plugin installs
```

Everything the app writes stays inside `data/`. Deleting that directory resets the app to a first
launch — the way to start over if you have lost both the master password and the recovery key.

Copy the entire folder to a USB stick or another PC — settings, vault, and plugins move with it.

### Starting it twice

Only one copy runs per folder. Starting xQuakShell again from the same folder brings the window
that is already open to the front instead of opening the vault a second time, because two copies
would each save their own version of it and the last one to save would quietly undo the other. If
the open window cannot be found — it is still starting, or still saving on its way out — the new
copy waits a few seconds and then says xQuakShell is already running. Copies in different folders
are separate installations with separate vaults and run side by side.

### Important

- **Keep the recovery key somewhere safe and offline.** It is the only way back into the vault
  without the master password; nobody else can recover it for you.
- The vault is encrypted (age + scrypt). Stealing the exe or vault file alone does not give access without the master password.
- Lock the vault when leaving your desk: use the lock button or rely on idle/minimize auto-lock (if enabled in Settings).

---

## Connections

### Creating a connection

1. Right-click in the sidebar (on a folder or the root).
2. Choose **New → Connection**.
3. Fill in the connection details in the right panel.

### Connection fields

| Field | Description |
|-------|-------------|
| **Name** | Display name (e.g., "Production DB Server"). |
| **Host / Port** | SSH host and port (default 22). A host name, an IPv4 address, or an IPv6 address with or without brackets — `2a0f::2` and `[2a0f::2]` are the same server. |
| **Users** | One or more users; each can use a key or password. |
| **Default user** | The user used when connecting. |
| **Tags** | Optional tags for filtering (e.g., "prod", "backup"). |
| **Jump chain** | Bastion hops (see [Jump Chains](#jump-chains-bastion)). |

### Editing and deleting

- Click a connection to select it; the details panel opens on the right.
- Changes are auto-saved.
- Right-click → **Delete** to remove a connection.

### Duplicating

Right-click a connection → **Duplicate**. The copy appears directly beneath it as "<name> - copy",
then "- copy 2", "- copy 3"; duplicating a copy numbers it rather than adding another suffix. With
several connections selected, each is duplicated.

The copy uses the same saved keys and passwords, as two connections set up by hand to share them
would. Everything else is its own: editing or deleting either leaves the other as it was, port
forwards get ids of their own, and a plugin connection's secret fields are copied rather than
shared.

---

## SSH Connections

xQuakShell is **SSH-first**: every connection uses SSH for terminal and SFTP file access.

- Host, port (default 22), username.
- Auth: SSH key or password (stored encrypted in vault).
- Supports PTY terminal, SFTP, jump chains.

Additional session protocols can be added later via the plugin seam (`SessionConnector`); the core ships without built-in non-SSH connectors.

---

## Authentication

### SSH keys

Keys live in the vault, managed on the **SSH Keys** screen (key button in the top bar):

- **Add a key** — generate one (Ed25519 recommended, RSA or ECDSA) or import an existing private
  key file, including encrypted ones. `.ppk` keys come in through [Import → From PuTTY](#from-putty).
- **Rename**, **export** (protected with a passphrase of your choice; asks for the master password),
  **delete**, and **copy the public key**. Each key shows its SHA256 fingerprint and the connections
  using it.
- **Publish** — add the public key to `authorized_keys` on a server you are connected to. A key that
  is already there is left alone.

Every key is stored encrypted, in the format `ssh-keygen` writes, and you choose what protects it:

- **Opened by the vault** — unlocking the vault is enough to use the key.
- **Passphrase required** — the key stays shut even while the vault is open; you are asked for its
  passphrase when you connect, up to three tries.

Per key you also choose how long an entered passphrase is remembered — until the vault locks, for a
number of minutes, or never — whether plugins may read it, and whether it may be
[forwarded](#agent-forwarding). Assign a key to a connection user in the connection's editor.

### Passwords

- Passwords can be stored per connection user or per hop in a jump chain.
- They are encrypted in the vault.
- Use **Import password** to add a password and reference it by ID.

### Per-connection users

- A connection can have multiple users.
- Each user has: username + auth method (key or password).
- **Default user** is used when you double-click to connect.

If the key or password a connection names is no longer in the vault — deleted, or never imported —
the tab says so before anything is sent to the server, instead of reporting "Authentication
failed". Pick another one in the connection's editor.

---

## Jump Chains (Bastion)

A jump chain routes your connection through one or more intermediate SSH hosts:

```
You → Bastion1 → Bastion2 → Target
```

### Configuring a jump chain

1. Select a connection.
2. In the **Jump Chain** section, add hops.
3. Each hop: host, port, username, auth (key or password).
4. Hops are traversed in order.

### Example

- **Hop 1:** `bastion.company.com`, port 22, user `jump`, key `~/.ssh/jump_key`
- **Hop 2:** `inner-gw.internal`, port 22, user `admin`, password
- **Target:** Your final SSH host (from the main connection fields).

---

## Folders and Organization

### Creating folders

- Right-click in the sidebar → **New → Folder**.
- Name the folder.
- Folders can be nested arbitrarily.

### Drag and drop

- **Move connections:** Drag a connection onto a folder to move it there.
- **Reorder:** Drag a connection or folder to reorder within the same level. Drop zones (before/after/into folder) are highlighted.
- Works at any depth: root, nested folders, etc.

### Favorites

- Mark connections as favorites for quick access in the sidebar.

### Search and reachability

- The search box above the tree filters connections and folders by name. Resources a discovery
  plugin reports are not searched.
- **Expand all** / **Collapse all** and **Move to** are in the tree's menu.
- Every host is checked for reachability by a TCP connect and shown with a status dot — on an
  interval, or only when a connection's settings change (**Settings → Network → Connection ping**).

---

## Importing Connections

The **Import** button in the sidebar toolbar (download icon) opens a menu with two sources.

### From SSH config

Imports hosts from an OpenSSH client config (`~/.ssh/config`).

1. Click **Import → From SSH config…**. The file at `~/.ssh/config` is detected automatically; use **Browse…** to pick a different one.
2. Review the parsed hosts. Each row shows the alias, `user@host:port`, and badges for referenced keys and jump chains.
3. Tick the hosts to import, choose the destination folder (or create a new one), and confirm.

**What is understood:** `Host`, `HostName`, `User`, `Port`, `IdentityFile`, `ProxyJump` and `Include`. Wildcard blocks such as `Host *` are applied as defaults to the concrete hosts rather than imported as connections. `ProxyJump` becomes a [jump chain](#jump-chains-bastion), with each hop resolved through its own `Host` block.

**Precedence** follows ssh(1): the *first* value obtained for a setting wins. If you keep a `Host *` block at the top of your config, its values override the specific blocks below it — the import behaves the same way your `ssh` command does.

**Duplicates:** a host that matches an existing connection by host, port and user is marked *Already in vault* and starts unticked. Nothing is ever overwritten; ticking one creates a second connection.

**Keys:** private keys named by `IdentityFile` are imported only if you tick **Also import the referenced private keys**. Keys are stored in the vault as-is — an encrypted key stays encrypted, and you are asked for its passphrase when you first connect, not at import time. A key shared by several hosts is stored once.

**Not imported:** `Match` blocks (their conditions can only be evaluated while connecting) and `ProxyCommand` (no jump-chain equivalent). Anything skipped is listed in the warnings panel at the bottom of the dialog.

### From PuTTY

- **`.ppk`** — imports a PuTTY private key as an identity. Enter the passphrase if the key is encrypted.
- **`.reg`** — imports saved sessions from a registry export into the currently selected folder.

To produce a `.reg` file, run: `regedit /e putty-sessions.reg HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions`

---

## Sessions and Tabs

### Opening a session

- Double-click a connection (or use Connect from the context menu).
- A new tab opens with SSH terminal + SFTP file panels.

### Multiple tabs

- Each tab is independent. You can have many SSH sessions open at once.
- Tabs show connection name and status (connecting, ready, error, closed).
- Close a tab with the X button.

### Session status

- **Green dot:** Active session.
- **Gray/Red:** Disconnected or error.
- Status updates immediately when a session is closed.

### Reconnect

- If a session fails or disconnects, use **Reconnect** to try again.
- When an established session loses its connection, **Reconnect** counts down and reconnects on its
  own: after 5, 15 and 30 seconds, then every 60 seconds until it succeeds. Click it to reconnect
  now, or **Stop retrying** to keep the tab as it is. A shell closed with `exit` and a connection
  that failed at the first attempt are not retried automatically.
- The reconnected session keeps its tab, its tile and, by default, the terminal output. Programs
  that were running on the server are not restored.
- Both behaviours can be turned off in **Settings → Network → Reconnect**.

---

## Tiling the Workspace

Sessions do not have to take turns in one pane. Drag a session tab onto an open session and drop
zones light up along its edges: drop on an edge to **split** the area and show both sessions at
once, drop in the middle to **merge** the tab into that group's tab bar. Tiles can be nested, and
the divider between any two is draggable.

This is the same drag as reordering tabs — where you release it decides whether you get a new tile
or a moved tab.

---

## Terminal

- Full PTY terminal via xterm.js.
- Supports colors, resize, copy/paste.
- Input is logged to the audit log (with heuristic password masking).

### Keyboard shortcuts

Terminal keys go to the remote shell: Ctrl+C, Ctrl+L and friends behave as they would in any
terminal, and copy/paste uses the system shortcuts or the terminal's context menu.

The application keeps a few shortcuts of its own, deliberately chosen not to collide with common
shell bindings. All of them are listed on the start screen and rebindable in **Settings → Hotkeys**,
which reports a clash with another shortcut before saving:

| Action | Default |
|--------|---------|
| New session from the selected connection | `Ctrl+Shift+N` |
| New local terminal | `Ctrl+Shift+T` |
| Next session | `Ctrl+Tab` |
| Previous session | `Ctrl+Shift+Tab` |
| Close session | `Ctrl+Shift+Q` |
| Search terminals | `Ctrl+Shift+F` |
| Type into several terminals | `Alt+M` |
| Stop typing into several terminals | `Alt+Shift+M` |

These stay active while the terminal has focus — otherwise they would be unreachable exactly when
you need them.

### Searching terminals

`Ctrl+Shift+F` turns the sidebar into a search view, the way an editor's search replaces its file
tree: a query box with **match case**, **whole word** and **regular expression** switches, the
terminals to search, and every match grouped by terminal underneath.

- **Enter** / **Shift+Enter** walk the matches, bringing each one's terminal to the front with the
  match scrolled into view and selected.
- **Escape** hands the keyboard back to the terminal and brings the connection tree back as it was.
- Every match is tinted inside the terminals too, the current one brighter.
- The search covers the whole scrollback, finds a word the terminal wrapped onto the next line, and
  follows new output while the panel is open. It works in SSH, local and plugin terminals alike.
- It starts on the terminal you were in; opening it with text selected searches for that text.

The binding is not `Ctrl+F` because inside a terminal `Ctrl+F` belongs to the program running there
— cursor movement in bash and zsh, paging in vim, less and man. Rebind it to `Ctrl+F` if you prefer.

### Typing into several terminals at once

1. Press `Alt+M`. Every open terminal is outlined in red and numbered; a terminal hidden behind
   another tab is chosen by its tab, which carries the same number.
2. Click a terminal or press its number to choose it — it turns green. **A** chooses all, **N**
   none, **Escape** leaves everything as it was.
3. Press **Enter**.

From then on whatever you type into a green terminal — pastes included — reaches all of them, and
the green outline stays so it is always clear what a keystroke will touch. Typing into a terminal
outside the group reaches that terminal alone. `Alt+M` again changes the group.

`Alt+Shift+M` ends it, as do **Stop** in the top bar and **Enter** in the picker with fewer than two
chosen. Escape cannot, because it belongs to whatever runs in the terminal. A terminal that closes
leaves the group, and a group down to one ends on its own. On SSH sessions each command is recorded
in the audit log of every server it reached.

### Scripts & Commands

**Scripts** in the top bar opens a list of commands to run in the active terminal:

- **Presets** for everyday checks — disk space, memory usage, listening ports, service status,
  running containers, following or searching log files. A preset can be edited or deleted.
- **Custom script** — type any command, optionally **Save as preset** with a name and a hint
  (hold Ctrl and hover a preset to see it). The editor flags an unclosed quote or heredoc before it
  is sent.
- Each preset can carry a **hotkey** of its own; pressing it runs the preset in the active terminal.
  A hotkey already taken is reported with the name of what holds it.

---

## Local Terminal

A shell on **your own machine**, opened without a connection. It is a tab like any other: same
renderer, same tab bar, same tiling, and `Ctrl+Shift+Q` closes it.

### Opening one

- The terminal button in the connections-tree toolbar, beside "New connection".
- Or `Ctrl+Shift+T`.

### Which shell it starts

**Settings → Appearance** offers only the shells actually found on this computer — PowerShell 7,
Windows PowerShell or the Command Prompt on Windows; your `$SHELL`, bash, zsh, fish or sh
elsewhere. The choice applies to the next terminal you open. Pick one and later uninstall it, and
the next terminal quietly falls back to this machine's default rather than refusing to open.

On Windows the console is switched to UTF-8 before you see it, so filenames with non-Latin
characters read correctly instead of arriving as mojibake.

### What it does and does not do

- **Nothing is saved.** No connection appears in the tree. Closing the tab ends the shell and
  everything it started, so a `ping -t` or a running build does not survive as an orphan.
- **Plugins cannot reach it.** There is no capability, no host method and nothing in the plugin API
  that opens a local shell — see [Security Model](./docs/security-model.md#local-terminal-adr-020).
- **Your keystrokes are not recorded.** An SSH session logs the commands you submit because that
  trail is about somebody else's machine; this is your own computer, where you already have a shell
  history. The audit log records that a shell was opened and which one it was.
- **It survives locking the vault.** It holds nothing from the vault, so the lock screen hides it
  rather than killing whatever is running behind it.

The full reasoning is in [ADR-020](./docs/adr/020-local-terminal.md).

---

## SFTP File Transfer

Available for **SSH** sessions.

### Layout

The file panels share one column to the **right of the terminal**:

- **Top of the column:** remote file tree (the SFTP side).
- **Below it:** local file tree.
- **Bottom of the session:** transfer panel (uploads/downloads in progress).

Every divider is draggable: between terminal and file column, between the two trees, and — since
the sidebar became resizable — the sidebar's right edge as well.

### Browsing

- Click folders to expand. Use the path bar to jump to a directory.
- Right-click for: New file, New folder, Delete, Rename, Edit, and Permissions on the remote side.
- Sort by name, size, date or owner; show or hide the date, owner and permissions columns and hidden
  files.
- A folder you cannot read keeps the pane where it was, with the reason in the pane's header.
- **Hide files** on a session's tile gives the whole tile to the terminal.

### Permissions

**Permissions** on a remote file or folder sets read, write and execute for owner, group and other,
or an octal mode. On a folder you can **recurse into subdirectories** and apply the change to files
only, directories only, or both; a long recursive change shows its progress in the transfer panel
and can be cancelled there.

### Editing files in your own editor

Right-click a file → **Edit**.

- **A remote file** (up to 5 MB) is downloaded to a temporary folder and opened in the editor set in
  **Settings → Files → External editor**, or in the system's default application for that file type
  when none is set. When you save, it is uploaded back to the same path on the server and appears in
  the transfer panel.
- **A local file** opens for editing straight away.

Name the editor by the path to its executable — for example
`C:\Users\you\AppData\Local\Programs\Microsoft VS Code\Code.exe` or `/usr/bin/gvim`.

### Upload

- Drag files from the local tree to the remote tree, or use the upload button.
- Or drag files and folders straight from your system's file manager onto either pane.
- Progress is shown in the transfer panel.
- When a file already exists at the destination you are asked what to do, unless **Settings →
  Files** sets a default for uploads and for downloads.
- **Settings → Network → File transfer** limits how many transfers run at once, their speed, and
  the connection timeout.

### Download

- Drag files from the remote tree to the local tree, or use the download button.
- Choose the local destination directory.

### Transfer panel

- Lists active and completed transfers, and long operations such as a recursive delete or
  permission change.
- **Cancel** to abort; a cancelled transfer simply says "Cancelled".
- **Retry** for failed or cancelled transfers.

---

## Port Forwarding

Forwarding rules belong to a connection and come up with its session. Add them in the connection
details panel, under **Forward rules**:

| Kind | ssh equivalent | What it does |
|------|----------------|--------------|
| **Local** | `-L` | A port on your machine is forwarded to a host reachable from the remote end |
| **Remote** | `-R` | A port on the remote host is forwarded back to a host reachable from yours |
| **Dynamic** | `-D` | A local SOCKS proxy that routes through the session |

Local and remote rules take a bind address (default `127.0.0.1`), a bind port, and the target host
and port. A rule can be disabled without deleting it. Plugins may also contribute tunnel providers,
which appear as an extra kind once the plugin has been granted tunnel-provider consent in its
settings.

---

## Agent Forwarding

A shell can lend your vault's keys to the server you opened it on, so from there `git pull` from a
private repository, `ssh` to the next host, and `scp` or `rsync` to another server work without
copying a key anywhere.

It takes two yeses:

1. In the **Key Manager**, allow **Agent forwarding** for the key. It is off for every key,
   including existing ones.
2. In the connection's editor, choose which of those keys the connection forwards.

Every signature then asks first, naming the server that asks, the key, the user it logs in as and
where the signature will be used — the destination host by its known_hosts name when the server's
OpenSSH proves it (8.9 and later), and a plain warning when it does not. A connection can also offer
**allow for N minutes**, which covers only the same key to the same proven host in the same tab.

What the server cannot do: get anything signed other than a login (no git commits), add, remove or
lock keys, or reach the agent after the tab closes or the vault locks. Jump hosts and plugin
connections never get the agent. Every request, allowed or refused, is in the audit log by key
fingerprint, and tabs with a forwarded agent carry a key mark.

---

## Plugins

Everything beyond SSH — VNC, Telnet, Docker discovery — is an out-of-process plugin. Plugins have a
screen of their own, opened from the puzzle-piece button in the top bar. It is organised by what a
plugin is currently doing — active, disabled, needing attention.

### Installing

- **From GitHub or GitLab.** Add a repository by its URL, then install a release from it. The host
  in the URL selects which API is spoken; anything other than `github.com` or `gitlab.com` is
  refused. Plugins that ship a user interface must be published as an `.xqsp` bundle; a release that
  offers this platform only a bare binary is refused at install time, with the missing asset named.
- **From a file.** Install a downloaded `.xqsp` bundle, or point the installer at an unpacked
  plugin directory — useful while developing one.

### What a plugin may do

A plugin's manifest declares the files, hosts and vault fields it needs. Anything outside that
declaration is refused before it runs, and refusals are recorded in the audit log. Capabilities that
carry real risk — tunnel providers among them — additionally require your explicit consent on the
plugin's row in the Plugins screen, separately from installing it.

Whether an unsigned plugin may run at all, and which publisher keys count as trusted, is the plugin
trust policy in **Settings → Security**. A new vault requires signed plugins; changing the policy asks
for the master password.

Plugins can contribute commands; `Ctrl+Shift+P` opens the **command palette** to search and run
them.

Each plugin runs as its own OS process with memory and handle limits, and is killed when the app
shuts down. Uninstalling removes the plugin itself; the data it stored under `data/plugins/<id>/`
is kept unless you ask for that to go too, so reinstalling does not silently discard its state.

With sync on, a plugin installed on one device is offered on the others, never installed by itself —
see [Plugins from other devices](#plugins-from-other-devices).

---

## Sync Between Devices

Sync keeps the vault of several devices in step through `xqsyncd`, a sync server you run yourself
([admin guide](./docs/sync-server.md)). Everything is encrypted and signed on the device: the server
cannot read a host name, a password or a folder name, cannot alter a record, and cannot roll a vault
back or add a device without every device noticing.

Sync is off until you turn it on in **Settings → Sync**, and needs a master password of at least 12
characters.

### Signing in

- **First device:** paste the connect link the server prints (`https://host:8443/#fp=…`) — the
  application checks the server's fingerprint itself — or enter the address and compare the
  fingerprint it shows with the one the server's operator gave you. Choose a sync password of at
  least 12 characters.
- **Every further device** signs in with the sync password. The devices already signed in announce
  the new one.

Fingerprints, the device list, the sync password and **Shred now** are under **Advanced**.

### Devices

Every device lists the others and can remove one, which replaces the account keys so the removed
device can read nothing written afterwards. **Not mine, remove it** — for a device you do not
recognise — also offers a new sync password: filled in, the removal and the new password are one
step, so whoever typed the old one cannot sign a device in again. Left empty, the device is still
removed at once, and you are asked to change the password afterwards.

### Conflicts and changes that wait for you

Edits made offline on two devices merge field by field. The same credential edited on both becomes a
**conflict** you resolve; until you do, the sync indicator in the top bar is red and its tooltip
says why.

A change from another device that would weaken this one waits for this device's master password in
**Changes waiting for you**: turning off plugin signature checks, loosening the lockout, replacing
or deleting a known host key, a peer identity or a key. This holds for team vaults as for your own.

### Trash and Shred now

A deleted synced item stays in the **Trash** for the vault's retention window — 30 days by default —
and can be restored. After that it is purged from the server, and within one and a half more
windows the keys that covered it are destroyed on every device. **Shred now** does all of it at
once; change the sync password afterwards to make even old copies the server might keep unreadable.

### Plugins from other devices

A plugin installed on one device is offered on the others under **Plugins from your other devices**.
Installing the offer runs the usual download, checksum, signature and consent steps, then checks that
the build is exactly the one your other device installed; a release replaced behind the same tag is
refused unless you accept it as a new version. A plugin may declare files of its own that follow you
between devices; they are written only while the plugin is stopped, and the Sync page shows what
waits for a restart.

---

## Teams

A team vault shares connections with other accounts on the same sync server. Each team appears in
the connection tree under its own row, below your personal connections.

### Inviting and joining

1. The person you invite copies their **contact code** (`xqs-member:name#fp=…`) from their Teams
   page and sends it to you in a chat or a mail.
2. You paste it into the invite field and pick a role: **admin**, **editor** or **viewer**. The
   application compares the code with the key the server gives for that account; a code that does
   not match leaves nothing to invite. Pasting a code into an existing member's row verifies that
   member the same way.
3. The invited person sees the invitation under **Invitations** with your fingerprint. Nothing from
   the team reaches their connections until they accept; declining leaves the team. Accepting on
   one device joins on all of them.

### Working with team connections

- Create a connection or folder under the team's row to keep it in the team, or move one in and out
  from the tree's menu, the editor's **Stored in** field, or by dragging it onto another vault.
- **Credentials are not shared.** Each member links their own key or password to a team connection
  under **Your credentials for team connections**. Moving a connection into a team links your key or
  password for you; moving it back takes that link with it as the connection's credential.
- A member who changes a shared connection's address suspends everyone else's link to it until each
  of them confirms the new address in **Settings → Sync**, so nobody's password follows a connection
  to a host they did not choose.
- A **viewer** sees and uses the team's connections but cannot add, change or move them.
- Removing a member moves the team to new keys, so nothing written afterwards opens for them.

---

## Known Hosts

- **Known Hosts** (shield icon) lists all stored host keys.
- When you connect to a new host, you are prompted to accept its key.
- If a host key changes (e.g., reinstall), you get a mismatch warning. Choose **Replace** to update or **Cancel** to abort.
- Remove entries from Known Hosts if you want to be prompted again on next connect.
- A host may have several keys, one per type, as OpenSSH keeps them; hashed entries imported from an
  OpenSSH `known_hosts` file are understood.

### Trusted Peers

Protocols other than SSH get the same check through **Trusted Peers** (fingerprint button in the top
bar). A plugin that has seen its peer's identity asks the application whether it is trusted; an
unknown or changed one stops the session and shows you a `SHA256:` fingerprint. **Trust** records it
and reconnects, **Reject** fails the session. The screen lists every recorded identity with when it
was trusted, and revoking one makes the next connection ask again. It is kept apart from Known Hosts
in every layer, so no plugin can reach the trust the SSH client reads.

---

## Audit Log

The audit log is **off by default**; turn it on in **Settings → Audit Log**. It is stored locally in
`data/audit.db`.

- **Audit Log** (document icon) has two tabs: **Command history** — every command submitted in an SSH
  session, on Enter — and **Program audit** — what the application itself did, such as trust
  decisions about hosts and peers and plugin calls that were refused.
- Both are searchable (SQLite FTS5) and filterable by connection or session. A command can be
  copied, or **re-run in the active session**.
- Heuristic masking hides password-like input (e.g., after "password:" prompts). **Log secrets this
  session** turns that off for the current session only, for debugging.
- **Privacy** settings choose whether the connection name, host and user name are recorded;
  **Retention** deletes old entries by age or by count.
- A local terminal contributes one entry saying a shell was opened and which one — never what you
  typed into it.
- Logs, errors and the audit log stay in English whatever the interface language.

The **debug log** is separate: **Settings → About → Developer** opens its window and sets its level
(`warn` by default; set `info` or `debug` before reproducing a problem).

---

## Settings and Lockout

### Settings

Settings opens on eight tabs:

| Tab | What lives there |
|-----|------------------|
| **About** | Version, plugin API version, update check (on startup or now), releases and issue links, and the debug log under **Developer** |
| **Appearance** | Interface language, theme and colors, UI scale, terminal font, and which local shell to start |
| **Audit Log** | Whether commands are recorded, privacy, retention, and logging secrets for this session |
| **Files** | The external editor, and what to do when an upload or download meets an existing file |
| **Hotkeys** | The shortcuts above |
| **Network** | Connection ping, automatic reconnect, and file transfer limits |
| **Security** | Master password and recovery key, lockout (idle timeout, lock on minimize), and the plugin trust policy |
| **Sync** | The sync account, devices, teams, conflicts, waiting changes, trash, and plugins from other devices |

The search box at the top of Settings finds a setting by name on any tab.

### Interface language

**Settings → Appearance → Language** switches the interface at once, the master password screen
included. English and Russian ship built in. To add a language, drop `<code>.json` into
`data/locales`; a file for a language that already ships overrides it key by key, so correcting one
phrase is a file with one entry, and anything a file leaves out falls back to English. A file on disk
cannot reword a security warning — host key prompts, plugin consent, irreversible deletes — and is
always shown as text, never as markup.

Plugins used to be a tab here and are now a screen of their own, reached from the top bar. The
Terminal tab folded into Appearance, where the terminal font sits beside the interface font it was
always adjusted against.

### Lockout behavior

- When lockout triggers, the vault is locked. Sessions stay connected.
- Re-enter the master password to unlock.
- Passphrase cache is cleared on lock.

---

## Tips

1. **Organize by environment:** Use folders like `Production`, `Staging`, `Dev`.
2. **Use tags:** Tag connections (e.g., `mysql`, `web`) for quick filtering.
3. **Jump chains for locked-down networks:** Route through a single bastion instead of exposing all hosts.
4. **Portable build:** Use `make portable` to create a fully offline distribution with WebView2 bundled.
5. **Backup the vault:** Copy `vault.age` to a safe place. It contains everything; keep it secure, and keep the recovery key apart from it.
6. **Stay current:** only the latest release receives fixes; the update check in **Settings → About** says when there is a newer one.

---

## Troubleshooting

### "Host key verification failed"

- The host key is unknown or changed. Use the host key dialog to add or replace it.

### "Connection refused" / "Network unreachable"

- Check host, port, and firewall.
- For jump chains, ensure each hop is reachable from the previous one.

### App won't start (WebView2 error)

- Install WebView2 Runtime, or use `make portable` to bundle it with the app.

### "xQuakShell is already running"

- Another copy from the same folder is open, or still closing. Switch to its window; if it has just
  been closed, wait a few seconds and start again. See [Starting it twice](#starting-it-twice).

### Vault corrupted or won't unlock

- Ensure you are using the correct master password. If you have forgotten it, type the
  [recovery key](#recovery-key) into the same field.
- If the vault file was truncated or corrupted, recovery may not be possible. Restore from backup if available.
- After upgrading to 1.5, the first unlock keeps the previous vault as `vault.age.v4.bak` next to
  it. Versions 1.4 and older refuse the upgraded vault; to go back to one, restore that backup.
