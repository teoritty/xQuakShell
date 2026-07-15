# Plugin manifest reference

`plugin.json` describes a plugin package consumed by xQuakShell.

## Top-level fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Reverse-DNS id, e.g. `com.example.myplugin` |
| `name` | string | yes | Display name |
| `version` | string | yes | Semver string |
| `description` | string | no | Short description |
| `minCoreVersion` | string | no | **Deprecated** — use `requires` instead (see below). A value `< 1.0.0` is rejected. |
| `requires` | object | no | Plugin API / capability version requirements (ADR-012) |
| `engine` | object | yes | How to launch the plugin |
| `capabilities` | object | no | Permission declarations |
| `contributions` | object | no | UI and protocol contributions |
| `activationEvents` | string[] | no | Lazy activation triggers |
| `isolation` | string | no | `per-plugin` (default) or `per-session` |
| `signature` | string | no | Base64 Ed25519 signature (Phase 6) |

## Requires (API versioning)

The `requires` block declares the versioned API surface the plugin depends on (ADR-012). The
plugin is checked against the host at install and again at `initialize`; the host is the authority.

```json
{
  "requires": {
    "pluginApi": "1.0.0",
    "capabilities": {
      "vault": { "min": "1.0.0", "features": ["getSecret"] }
    }
  }
}
```

- `pluginApi` (required in the block): the frozen protocol envelope version. Compatible when the
  host has the same major and a minor ≥ yours.
- `capabilities.<name>.min` (required per entry): minimum capability version, same rule.
- `capabilities.<name>.features` (optional): named feature flags you call; each must be offered by
  the host or the plugin is rejected by name.
- All versions are strict `MAJOR.MINOR.PATCH` with **no** pre-release suffix.
- You may only require a capability you also declare in `capabilities{}`. A granted capability with
  no explicit requirement gets an implicit baseline (`<major>.0.0`).

Migration: `minCoreVersion` is deprecated. `>= 1.0.0` is auto-migrated to a `pluginApi` requirement
(with a warning); `< 1.0.0` (built against the pre-1.0 API) is rejected — rebuild against
`pluginApi 1.0` and declare `requires`.

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

Runtime IPC methods and session lifecycle are documented in [plugin-api.md](./plugin-api.md#plugin-ipc-reference).

```json
{
  "capabilities": {
    "filesystem": {
      "read": ["${pluginData}"],
      "write": ["${pluginData}"]
    },
    "network": {
      "outbound": ["tcp:example.com:443"],
      "allowArbitraryOutbound": false,
      "allowPrivateNetworks": false
    },
    "vault": {
      "readConnectionFields": ["host", "port"],
      "getSecret": ["password"]
    },
    "session": {
      "connectProtocols": ["my-protocol"],
      "terminal": true,
      "embed": false,
      "localEmbedServer": false,
      "remoteFs": false,
      "allowMultiSession": false,
      "maxTunnelBandwidthKbps": 0
    },
    "events": {
      "subscribe": ["core.session.*"],
      "publish": ["plugin.com.example.myplugin.*"]
    },
    "auth": {
      "provider": true,
      "methods": ["keyboard-interactive", "publickey"]
    },
    "tunnel": {
      "provider": true,
      "maxConcurrentChannels": 64
    },
    "channel": {
      "purposes": ["exec", "tcp-relay", "udp-relay", "embed-stream"],
      "maxConcurrent": 4,
      "maxThroughputKbps": 0,
      "execCommands": [
        { "argv": ["docker", "system", "dial-stdio"] },
        { "argv": ["docker", "exec", "-it", "{containerId}", "sh"],
          "params": { "containerId": "^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$" } }
      ]
    }
  }
}
```

**`auth` capability**

| Field | Type | Description |
|-------|------|-------------|
| `provider` | bool | `true` when the plugin implements SSH auth RPC (`auth.prepare`, `auth.answerChallenge`, `auth.sign`). Install requires explicit user consent (`authProviderAccessGranted`). |
| `methods` | string[] | Allowed auth kinds: `keyboard-interactive`, `publickey`. Each must have a matching `contributions.authMethods[]` entry. |

**`tunnel` capability**

| Field | Type | Description |
|-------|------|-------------|
| `provider` | bool | `true` when the plugin handles dynamic forward rules via `tunnel.*` RPC. |
| `maxConcurrentChannels` | int | Per-plugin limit for concurrent `tunnel.dial` channels (0 = host default). |

**`channel` capability**

Declares access to the binary channel bus — a raw duplex byte stream, multiplexed alongside JSON-RPC, whose far end the host constructs and owns. Full wire format, `channel.open`/`channel.close`, credit model, and purpose semantics: [plugin-api.md — Channel bus](./plugin-api.md#channel-bus).

| Field | Type | Description |
|-------|------|--------------|
| `purposes` | string[] | Closed enum: `exec`, `embed-stream`, `tcp-relay`, `udp-relay`. A plugin can only open channels for purposes it declares here. |
| `maxConcurrent` | int | Per-plugin limit for concurrent open channels (0/absent = host default **4**). Channels are heavier than tunnels — `exec` spawns a real remote process — so this default is deliberately lower than `tunnel.maxConcurrentChannels`. |
| `maxThroughputKbps` | int | Per-channel byte/sec token-bucket cap (0/absent = host default, same numeric default as `session.maxTunnelBandwidthKbps`, 32 MiB/s). Enforced independently of the frame-count credit window. |
| `execCommands` | array | Allowlisted argv templates for the `exec` purpose (see below). Required if `exec` is declared with any narrower-than-arbitrary command set; entries here require `exec` to also be listed in `purposes`. |

**`exec` command allowlist (`execCommands`)**

`exec` is the highest-risk purpose — it runs a command over the user's already-authenticated session — so the runnable commands are a closed, declared set of **argv-array templates**, never shell strings:

```json
"channel": {
  "purposes": ["exec"],
  "execCommands": [
    { "argv": ["docker", "system", "dial-stdio"] },
    { "argv": ["docker", "exec", "-it", "{containerId}", "sh"],
      "params": { "containerId": "^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$" } }
  ]
}
```

| Field | Type | Description |
|-------|------|--------------|
| `argv` | string[] | Command and arguments. Each element is either a literal or a named `{placeholder}`. The host always invokes via an argv array (never a shell string), eliminating command injection as a class of bug. |
| `params` | object | Regex (ReDoS-safe subset, same checker as connection-field `pattern`) for every `{placeholder}` used in `argv`. Required for each placeholder — a placeholder with no matching `params` entry fails manifest validation. |

A `channel.open` request for `exec` whose requested command doesn't match any declared template, or whose placeholder value fails its regex, is rejected at the capability gate (`-32001`, audit-logged) before any process is spawned.

**`udp-relay` and the network allowlist**

`tcp-relay` and `udp-relay` validate their `hint` target against the same `capabilities.network.outbound` allowlist used by `net.dial` (see [Rules](#rules) below), just with a proto-prefixed pattern form:

```json
"capabilities": {
  "network": {
    "outbound": ["tcp:db.internal:5432", "udp:relay.internal:51820"]
  },
  "channel": {
    "purposes": ["tcp-relay", "udp-relay"]
  }
}
```

- `tcp:host:port` — the existing form, used by both `net.dial` and `tcp-relay` channels.
- `udp:host:port` — same grammar (explicit host, no wildcards), used by `udp-relay` channels. `udp-relay` dials the target UDP endpoint directly from the host (`net.DialUDP`); it does not tunnel through the parent SSH connection.
- Both forms share the same IP-restriction core: loopback/RFC1918/link-local targets are blocked unless `allowPrivateNetworks: true`, resolved via the same DNS-rebinding-safe dial-then-validate path.

**Auth method contributions**

```json
"authMethods": [
  {
    "id": "otp",
    "label": "One-time password",
    "kind": "keyboard-interactive"
  }
]
```

**Tunnel provider contributions**

```json
"tunnelProviders": [
  { "id": "socks5", "label": "SOCKS5 proxy" }
]
```

Connection `forwardRules` with `"kind": "dynamic"` reference `pluginId` + `providerId` from these contributions.

Each forward rule `id` must be globally unique across all connections in the vault (auto-generated on create). Duplicate IDs are rejected at save time.

Rules:

- FS patterns must start with `${pluginData}` and resolve under the plugin install directory.
- **Network outbound:** each `outbound` entry must be an explicit `tcp:hostname:port` pattern (no wildcards). Alternatively, set `allowArbitraryOutbound: true` to permit TCP dials to any public host/port — install shows a warning and requires explicit user consent. Set `allowPrivateNetworks: true` (requires `allowArbitraryOutbound`) to also allow loopback, RFC1918, and link-local targets. Allowlist and arbitrary modes may coexist; a dial succeeds if either mode permits the target.
- Every `contributions.connectionProtocols[].id` must be listed in `capabilities.session.connectProtocols`.
- Event subscribe allowlist: `core.session.*` or explicit `core.session.opened|closed|stateChanged`. Broad `core.*` is rejected.
- Event publish must use namespace `plugin.<yourPluginId>.*`. Publishing to `core.*` is rejected.
- User-disabled plugins are stored in app settings (`plugins.disabled`).
- **`terminal: true` requires `isolation: per-session`** unless `allowMultiSession: true` is set (install shows a warning and is audit-logged).
- **`embed: true` requires `isolation: per-session`**. Mutually exclusive with `terminal: true`. Requires `connectProtocols`. **`allowMultiSession` is rejected** with embed.
- **`localEmbedServer: true`** requires `embed: true`. Opt-in loopback HTTP server in the plugin (Mode B); install shows a separate consent line. Default path is Mode A (core embed broker).
- **`remoteFs: true`** requires `terminal: true` or `embed: true` (adjunct file panel only).
- **`maxTunnelBandwidthKbps`:** optional per-session tunnel rate cap (0 = host default 32 MiB/s).
- **`channel.purposes`:** each must be one of the closed enum `exec` / `embed-stream` / `tcp-relay` / `udp-relay`; unknown purposes are rejected at manifest load. `channel.execCommands` requires `exec` to also be declared in `purposes`; every `{placeholder}` in an `argv` template requires a matching, safely-compilable regex in that template's `params`. **`exec` requires install-time user consent**, same as `auth.provider` / `allowArbitraryOutbound` — see [security-model.md](./security-model.md#capability-gate).
- **`allowMultiSession`:** when `false` (default) and `isolation: per-plugin`, only one bound session per plugin process is allowed; a second bind is rejected.
- **`remoteFs` (display):** when `true`, the session UI shows the remote file panel (SFTP-style). Terminal-only plugins (e.g. telnet) should leave this `false`.
- View `entry` paths must live under `ui/` (default `ui/index.html`). Embed `embedEntry` paths follow the same rule.

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
  {
    "id": "vnc",
    "label": "VNC",
    "defaultPort": 5900,
    "icon": "monitor",
    "embedEntry": "ui/vnc.html",
    "fields": []
  }
]
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Protocol id (must match `capabilities.session.connectProtocols`) |
| `label` | string | yes | Label in the connection editor |
| `defaultPort` | int | no | Used when connection `port` is 0 (ping and `session.connect`) |
| `icon` | string | no | UI icon hint (e.g. `monitor` for VNC/RDP) |
| `embedEntry` | string | no | HTML entry under `ui/` for embed sessions (default `ui/embed.html`) |
| `fields` | array | no | Field groups (see below) |

#### Connection protocol fields

Fields are grouped for display in Connection Details. The host validates manifest at load time and validates **values** on save.

**Field group**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Group id |
| `label` | string | yes | Group heading |
| `order` | int | no | Sort order (lower first) |
| `fields` | array | yes | Field definitions |

**Field definition**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique within the protocol |
| `label` | string | yes | Label in the form |
| `type` | string | yes | `text`, `password`, `number`, `select`, `checkbox`, `textarea` |
| `required` | bool | no | Enforced on save **only while the field is visible** (see `dependsOn`). Fields hidden by `dependsOn` are never validated and are actively cleared from storage on save, even if a value was previously entered — do not rely on stale storage. |
| `default` | any | no | Default for non-secret fields only |
| `placeholder` | string | no | Input hint |
| `description` | string | no | Help text |
| `width` | string | no | `full`, `half`, `third` |
| `order` | int | no | Sort order within group |
| `validation` | object | no | See below |
| `options` | array | yes for `select` | `{ "value", "label" }` |
| `dependsOn` | string | no | Field id; UI hides this field until dependency is truthy |
| `secret` | bool | yes | `true` for secrets; required for `password` type |

**Validation object**

| Field | Type | Applies to |
|-------|------|------------|
| `minLength` / `maxLength` | int | string types |
| `min` / `max` | number | `number` |
| `pattern` | string | string types (regex; ReDoS-safe subset enforced at load) |
| `maxSizeBytes` | int | `textarea` (default cap 1 MiB if unset) |

**Example**

```json
"connectionProtocols": [
  {
    "id": "telnet",
    "label": "Telnet",
    "defaultPort": 23,
    "fields": [
      {
        "id": "auth",
        "label": "Authentication",
        "order": 1,
        "fields": [
          {
            "id": "username",
            "label": "Username",
            "type": "text",
            "required": true,
            "width": "half",
            "order": 1,
            "secret": false,
            "validation": {
              "minLength": 3,
              "maxLength": 50,
              "pattern": "^[a-zA-Z0-9_]+$"
            }
          },
          {
            "id": "password",
            "label": "Password",
            "type": "password",
            "width": "half",
            "order": 2,
            "secret": true
          }
        ]
      }
    ]
  }
]
```

**Manifest validation rules (fields)**

- Field ids must be unique within a protocol.
- `password` fields must have `secret: true`.
- Secret fields cannot have a `default`.
- `select` fields must have `options`; default must match an option value.
- `dependsOn` must reference an existing field id; cycles are rejected.
- `dependsOn` cannot reference a `secret` field (host cannot evaluate visibility from encrypted values).
- Unsafe regex patterns (nested quantifiers, deep nesting) are rejected at load time.
- Protocol ids must not collide across installed plugins.

**Runtime storage**

- Non-secret values: `Connection.pluginFields[id]`.
- Secret values: encrypted in the vault file (age + scrypt at rest); connection stores `secret:<connectionId>.<fieldId>`.
- UI never receives secret plaintext on load (empty string for secret fields until the user re-enters a value).

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

When present, `signature` is base64 Ed25519 over a canonical JSON **envelope** that binds the manifest to the bundle checksum file:

```json
{
  "manifest": { "...": "plugin.json fields without signature" },
  "checksumsSha256": "<hex SHA-256 of normalized SHA256SUMS bytes>"
}
```

- `manifest` is the manifest JSON with the `signature` field omitted.
- `checksumsSha256` is the lowercase hex SHA-256 of the `SHA256SUMS` file bytes after normalizing CRLF line endings to LF.
- JSON map keys are sorted lexicographically (canonical JSON) so signatures remain stable across reloads.

Trusted publisher public keys are configured in application settings. Unsigned plugins can still be installed with explicit user confirmation unless **Require signed plugins** is enabled.

**Signing order for authors:** write `SHA256SUMS` first (hashes of all plugin files except the checksums file), then sign the manifest envelope `{manifest, checksumsSha256}`.

## Bundle format

`.xqsp` files are ZIP archives. Include `SHA256SUMS` with SHA-256 hashes of all files except the checksums file itself.

Bundled and user-installed plugins **should** ship with a valid `SHA256SUMS` file. Discovery validates checksums when the file is present; signed plugins **require** `SHA256SUMS` for signature verification. Unsigned dev plugins may omit checksums.

Refresh `SHA256SUMS` after changing plugin files (before signing or packing).

## Validation rules

- `id`, `name`, `version`, `engine.entry` required
- `engine.type` must be `go-binary`
- Capability patterns validated at install
- Connection protocol field declarations validated at manifest load (`ValidateManifestFields`)
- Binary must exist and match host GOOS at discovery/install

See also: [plugin-api.md](./plugin-api.md)
