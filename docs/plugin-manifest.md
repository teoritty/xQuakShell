# Plugin manifest reference

`plugin.json` describes a plugin package consumed by xQuakShell.

## Top-level fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Reverse-DNS id, e.g. `com.example.myplugin` |
| `name` | string | yes | Display name |
| `version` | string | yes | Semver string |
| `description` | string | no | Short description |
| `minCoreVersion` | string | no | Minimum xQuakShell version |
| `engine` | object | yes | How to launch the plugin |
| `capabilities` | object | no | Permission declarations |
| `contributions` | object | no | UI and protocol contributions |
| `activationEvents` | string[] | no | Lazy activation triggers |
| `isolation` | string | no | `per-plugin` (default) or `per-session` |
| `signature` | string | no | Base64 Ed25519 signature (Phase 6) |

## Engine

```json
{
  "type": "go-binary",
  "entry": "my-plugin.exe",
  "args": []
}
```

Only `go-binary` is supported in v1.

## Capabilities

```json
{
  "capabilities": {
    "filesystem": {
      "read": ["${pluginData}"],
      "write": ["${pluginData}"]
    },
    "network": {
      "outbound": ["tcp:example.com:443"]
    },
    "vault": {
      "readConnectionFields": ["host", "port"],
      "getSecret": ["password"]
    },
    "session": {
      "connectProtocols": ["my-protocol"],
      "terminal": true,
      "allowMultiSession": false
    },
    "events": {
      "subscribe": ["core.session.*"],
      "publish": ["plugin.com.example.myplugin.*"]
    }
  }
}
```

Rules:

- FS patterns must start with `${pluginData}` and resolve under the plugin install directory.
- Every `contributions.connectionProtocols[].id` must be listed in `capabilities.session.connectProtocols`.
- Event subscribe allowlist: `core.session.*` or explicit `core.session.opened|closed|stateChanged`. Broad `core.*` is rejected.
- Event publish must use namespace `plugin.<yourPluginId>.*`. Publishing to `core.*` is rejected.
- User-disabled plugins are stored in app settings (`plugins.disabled`).
- **`terminal: true` requires `isolation: per-session`** unless `allowMultiSession: true` is set (install shows a warning and is audit-logged).
- **`allowMultiSession`:** when `false` (default) and `isolation: per-plugin`, only one bound session per plugin process is allowed; a second bind is rejected.
- View `entry` paths must live under `ui/` (default `ui/index.html`).

## Contributions

### Commands

```json
"commands": [
  { "id": "myplugin.action", "title": "Do Action", "category": "Tools" }
]
```

### Connection protocols

```json
"connectionProtocols": [
  { "id": "telnet", "label": "Telnet", "defaultPort": 23 }
]
```

### Views (WebView panels)

```json
"views": [
  {
    "id": "myplugin.panel",
    "location": "sidebar.bottom",
    "title": "My Panel",
    "type": "webview",
    "entry": "ui/index.html"
  }
]
```

### Status bar

```json
"statusBar": [
  { "id": "myplugin.status", "text": "Ready", "tooltip": "Plugin status", "priority": 10 }
]
```

## Activation events

Examples:

- `onStartup`
- `onManual` — allow **Settings → Start plugin** (`StartPluginManual`)
- `onCommand:myplugin.action`
- `onProtocol:telnet`
- `onView:<panelId>` or `onView:*` — start when a contributed WebView panel is opened

## Signature

When present, `signature` is base64 Ed25519 over canonical JSON of the manifest with the `signature` field omitted.

Trusted publisher public keys are configured in application settings. Unsigned plugins can still be installed with explicit user confirmation unless **Require signed plugins** is enabled.

## Bundle format

`.xqs-plugin` files are ZIP archives. `xqs-plugin pack` adds `SHA256SUMS` with SHA-256 hashes of all files except the checksums file itself.

**Every plugin directory** (bundled under `<exe>/plugins` or user-installed under `data/plugins`) **must** ship with a valid `SHA256SUMS` file. Discovery rejects plugins without checksums or with hash mismatches — no exceptions.

Generate or refresh checksums after changing plugin files:

```bash
xqs-plugin checksums -dir .
```

## Validation rules

- `id`, `name`, `version`, `engine.entry` required
- `engine.type` must be `go-binary`
- Capability patterns validated at install
- Binary must exist and match host GOOS at discovery/install

See also: [plugin-api.md](./plugin-api.md)
