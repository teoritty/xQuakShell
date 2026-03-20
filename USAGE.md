# xQuakShell Usage Guide

This guide explains how to use xQuakShell for managing remote connections, organizing your infrastructure, and working with sessions.

---

## Table of Contents

1. [Vault and Master Password](#vault-and-master-password)
2. [Connections](#connections)
3. [Protocols](#protocols)
4. [Authentication](#authentication)
5. [Jump Chains (Bastion)](#jump-chains-bastion)
6. [Proxy and VPN](#proxy-and-vpn)
7. [Folders and Organization](#folders-and-organization)
8. [Sessions and Tabs](#sessions-and-tabs)
9. [Terminal](#terminal)
10. [SFTP File Transfer](#sftp-file-transfer)
11. [Known Hosts](#known-hosts)
12. [Audit Log](#audit-log)
13. [Settings and Lockout](#settings-and-lockout)

---

## Vault and Master Password

All data—connections, SSH keys, passwords, known hosts—is stored in a single encrypted file: `vault.age`. The vault is protected by a **master password**.

### First launch

- Enter any password to **create** a new vault. This password becomes your master password.
- On subsequent launches, enter the same password to **unlock** the vault.

### Where is the vault stored?

- **Portable mode:** If the executable directory is writable (e.g. you run from a USB stick or a folder on your Desktop), `vault.age` is created **next to the exe**.
- **Otherwise:** `%AppData%\xQuakShell\` (e.g. `C:\Users\<you>\AppData\Roaming\xQuakShell\`).

### Important

- **Remember your master password.** There is no recovery mechanism. If you lose it, the vault cannot be decrypted.
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
| **Protocol** | SSH, RDP, Telnet, Serial, or HTTP. |
| **Host / Port** | For SSH: main host and port. For RDP/Telnet: use protocol-specific fields. |
| **Users** | One or more users; each can use a key or password. |
| **Default user** | The user used when connecting. |
| **Tags** | Optional tags for filtering (e.g., "prod", "backup"). |
| **Jump chain** | Bastion hops (see [Jump Chains](#jump-chains-bastion)). |
| **Proxy** | SOCKS4/SOCKS5 proxy (optional). |
| **VPN** | VPN profile to use before connecting (optional). |

### Editing and deleting

- Click a connection to select it; the details panel opens on the right.
- Changes are auto-saved.
- Right-click → **Delete** to remove a connection.

---

## Protocols

### SSH (default)

- Host, port (default 22), username.
- Auth: SSH key or password (stored encrypted in vault).
- Supports PTY terminal, SFTP, jump chains, proxy, VPN.

### RDP (Remote Desktop)

- Host, port (default 3389), username, domain, password.
- **Windows:** Uses `mstsc.exe` with a temporary `.rdp` file.
- **Linux:** Uses `xfreerdp` (FreeRDP). Install: `sudo apt install freerdp2-x11` (or equivalent).
- Supports proxy, VPN, jump chain in the UI (routing depends on implementation).

### Telnet

- Host, port (default 23), username, password.
- Opens a terminal session over Telnet.

### Serial

- Port (e.g., `COM1`, `/dev/ttyUSB0`), baud rate, data bits, stop bits, parity.
- For serial console access.

### HTTP/HTTPS

- URL, method (GET, POST, etc.), optional auth.
- For HTTP-based tools or APIs.

---

## Authentication

### SSH keys

1. Go to **Settings** (or the identity management section).
2. **Import** a private key (PEM format).
3. If the key is encrypted, enter the passphrase once; it is cached for the session.
4. Assign the key to a connection user.

Supported key types: RSA, ECDSA, Ed25519.

### Passwords

- Passwords can be stored per connection user or per hop in a jump chain.
- They are encrypted in the vault.
- Use **Import password** to add a password and reference it by ID.

### Per-connection users

- A connection can have multiple users.
- Each user has: username + auth method (key or password).
- **Default user** is used when you double-click to connect.

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

## Proxy and VPN

### SOCKS proxy

- Enable **Proxy** for a connection.
- Type: SOCKS4 or SOCKS5.
- Host, port, optional username/password.
- Outbound SSH (and optionally other traffic) goes through the proxy.

### VPN

- **Import VPN profile:** Paste a WireGuard or OpenVPN config (base64 or raw).
- Assign the profile to a connection.
- When connecting, the app establishes the VPN tunnel first, then connects through it.

VPN support is in progress; WireGuard and OpenVPN connectors exist but may require additional setup.

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

---

## Sessions and Tabs

### Opening a session

- Double-click a connection (or use Connect from the context menu).
- A new tab opens. For SSH: terminal + SFTP. For RDP: external RDP client launches.

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

---

## Terminal

- Full PTY terminal via xterm.js.
- Supports colors, resize, copy/paste.
- Input is logged to the audit log (with heuristic password masking).

### Keyboard shortcuts

- Standard terminal shortcuts apply (e.g., Ctrl+C, Ctrl+L).
- Copy/paste: use system shortcuts or the terminal’s context menu.

---

## SFTP File Transfer

Available for **SSH** sessions.

### Layout

- **Left:** Remote file tree.
- **Right:** Local file tree.
- **Bottom:** Transfer panel (uploads/downloads in progress).

### Browsing

- Click folders to expand. Use the path bar to jump to a directory.
- Right-click for: New folder, Delete, Rename.

### Upload

- Drag files from the local tree to the remote tree, or use the upload button.
- Progress is shown in the transfer panel.

### Download

- Drag files from the remote tree to the local tree, or use the download button.
- Choose the local destination directory.

### Transfer panel

- Lists active and completed transfers.
- **Cancel** to abort.
- **Retry** for failed or cancelled transfers.

---

## Known Hosts

- **Known Hosts** (shield icon) lists all stored host keys.
- When you connect to a new host, you are prompted to accept its key.
- If a host key changes (e.g., reinstall), you get a mismatch warning. Choose **Replace** to update or **Cancel** to abort.
- Remove entries from Known Hosts if you want to be prompted again on next connect.

---

## Audit Log

- **Audit Log** (document icon) shows a searchable log of terminal input.
- Uses SQLite FTS5 for full-text search.
- Heuristic masking attempts to hide password-like input (e.g., after "password:" prompts).
- Useful for compliance and debugging.

---

## Settings and Lockout

### Settings

- **Session lockout:** Idle timeout, minimize lock, etc.
- **Theme:** Dark/light (if supported).
- Other app preferences.

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
5. **Backup the vault:** Copy `vault.age` to a safe place. It contains everything; keep it secure.

---

## Troubleshooting

### "Host key verification failed"

- The host key is unknown or changed. Use the host key dialog to add or replace it.

### "Connection refused" / "Network unreachable"

- Check host, port, firewall, and VPN/proxy settings.
- For jump chains, ensure each hop is reachable from the previous one.

### RDP: "Incorrect connection file specified"

- On Windows, the app generates a temporary `.rdp` file. Ensure the path is writable and the file is valid (UTF-16 LE with BOM for mstsc).

### App won't start (WebView2 error)

- Install WebView2 Runtime, or use `make portable` to bundle it with the app.

### Vault corrupted or won't unlock

- Ensure you are using the correct master password.
- If the vault file was truncated or corrupted, recovery may not be possible. Restore from backup if available.
