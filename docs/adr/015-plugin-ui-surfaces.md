# ADR-015: Plugin UI Surfaces

## Status

Proposed

## Context

ADR-014 gave plugins a subtree inside a connection's row and a menu of actions on it. It
deliberately stopped there: an action is opaque to the core, which draws the label, relays the
click, and knows nothing about what happens next. That is the right boundary for *invoking* work,
and it is the whole boundary that exists today.

The gap shows up the moment an action has a **result the user must look at or answer**. A plugin
enumerating remote resources can start and stop them, but it cannot:

- **show a stream** — a log follow, an interactive shell attached to a discovered resource. The
  only plugin→host verbs that reach a terminal are `session.writeTerminal` / `session.updateState`,
  and both are confined to the session the plugin itself provides as a connection protocol
  (`capabilities.session.connectProtocols`). A plugin that owns no protocol has no terminal
  anywhere, for anything.
- **ask a question** — "create a volume with these options", "remove, and also remove its
  anonymous volumes?". `Action.Confirm` carries one string and returns yes/no; there is no way to
  collect a structured answer. The only UI surface a plugin can draw at all is a
  `sidebar.bottom` WebView panel, which the frontend renders at `max-height: 220px`
  (`frontend/src/lib/PluginWebViewPanel.svelte`) and which no other location value reaches
  (`ContributionHost.svelte`).
- **show or edit a node's own settings** — the tree row has a label, an icon, a status dot and a
  menu, and nothing else. There is no equivalent of Connection Details for a discovered node.

Each of these is a general shape, not a Docker shape: "a plugin has a byte stream the user should
watch", "a plugin needs a structured answer before it can act", "a discovered node has properties".
The core has good primitives for all three *for its own features* — the terminal, the connection
editor's declarative field schema, the Connection Details panel — and no way to lend any of them to
a plugin.

## Decision

Three primitives, one new capability, no technology-specific concept anywhere in the core.

```json
"capabilities": {
  "ui": {
    "surfaces": ["terminal", "log"],
    "dialogs": true,
    "nodeDetails": true,
    "maxSurfaces": 8
  }
}
```

`ui` is granted like every other capability: declared in the manifest, enforced by the same `Gate`
that answers `-32001` and writes the denial to the audit log. It carries **no new privilege over
the remote machine** — every byte a surface displays was already obtainable by the plugin through
`channel`/`exec`, which carries its own install-time consent. `ui` governs where those bytes may
be *drawn*, which is why it needs no separate consent prompt, exactly as ADR-014 argued for
`discovery`. `PermissionSummary` gets one line: "Show its own tabs, dialogs and node details".

### 1. Surfaces — a tab the plugin owns

A **surface** is a tab in the tile grid, drawn by the core, whose content is a byte stream the
plugin produces and (for `terminal`) consumes. It is bound to a `parentSessionId` and lives no
longer than that session.

| Method | Direction | Params | Response |
|---|---|---|---|
| `surface.open` | plugin → host (request) | `parentSessionId`, `kind` (`terminal`\|`log`), `title`, `iconId?` | `{surfaceId}` |
| `surface.write` | plugin → host (request) | `surfaceId`, `dataBase64`, `stream?` (`stdout`\|`stderr`) | `{ok:true}` |
| `surface.updateState` | plugin → host (request) | `surfaceId`, `state` (`connecting`\|`ready`\|`error`), `error?` | `{ok:true}` |
| `surface.setTitle` | plugin → host (request) | `surfaceId`, `title` | `{ok:true}` |
| `surface.close` | plugin → host (request) | `surfaceId` | `{ok:true}` |
| `surface.input` | host → plugin (notification) | `surfaceId`, `dataBase64` | — |
| `surface.resize` | host → plugin (notification) | `surfaceId`, `cols`, `rows` | — |
| `surface.closed` | host → plugin (notification) | `surfaceId`, `reason` | — |

**Two kinds, because they are two different questions.** `terminal` is a duplex byte stream
rendered by xterm: the user types, the plugin receives `surface.input`, and a resize is a real
`cols`/`rows` event the far end can act on. `log` is a one-way stream the core renders in a viewer
it owns, with search, stdout/stderr distinction and export to a file — none of which a terminal
emulator can offer, because in a terminal those bytes are already screen cells. A plugin that sent
a log to a `terminal` surface would get a tab where searching means asking xterm about its
scrollback; one that sent a shell to a `log` surface would get a tab that cannot type. The kinds
are not a style choice.

**Why not "the plugin gets a session".** A session in this core owns a connection, an SSH client,
a vault binding and a host-key decision. A surface owns none of those and must never appear to: it
is a view onto work the plugin is already authorized to do on an existing session. Minting a real
session for it would put a second, weaker thing behind every `sessionId` in the codebase, and every
IDOR check that keys on session ownership would have to learn the difference. A surface has its own
id space (`srf-…`) and its own registry, and `SessionRegistry` never sees one.

**Lifetime is one-directional, like a channel's.** The parent session owns the surface: when it
closes — tab closed, SSH dropped, plugin crash-recovery — every surface bound to it is closed
synchronously, in the same step of the session close sequence that already closes channels
(ADR-011 §Session lifecycle coupling), and the plugin is told with `surface.closed`. When the
plugin process exits, its surfaces close unconditionally. Closing a surface never affects the
parent session or its siblings. Close is idempotent from either side.

**Backpressure, not buffering.** Each surface has a bounded output queue — 1 MiB, drained by a pump
that batches on a 50 ms tick, per stream and never across them — and `surface.write` returns
`-32003` once that queue has stayed full for 2 s, the same allowance `session.writeTerminal` gives.
The queue is what makes the verdict possible at all: a frontend event is fire and forget, so
emitting straight from the write could never observe a consumer falling behind. It is also what
keeps a chatty producer from becoming one repaint per chunk. `surface.write` decodes its payload
before queueing it, so a malformed one is refused rather than displayed.

A `log` surface holds a bounded ring buffer and states plainly in the UI when it has dropped the
oldest lines; it does not grow without limit, and it does not silently lose the fact that it lost
something. The viewer renders only the rows its viewport can show — 200 000 lines is a buffer
bound, not a DOM bound — and searches incrementally, scanning only what arrived since the last
pass.

### 2. Dialogs — a structured question

| Method | Direction | Params | Response |
|---|---|---|---|
| `dialog.open` | plugin → host (request) | `kind` (`form`\|`detail`), `title`, `submitLabel?`, `sections[]`, `values?` | `{dialogId}` |
| `dialog.setError` | plugin → host (request) | `dialogId`, `message`, `fieldErrors?` | `{ok:true}` |
| `dialog.close` | plugin → host (request) | `dialogId` | `{ok:true}` |
| `dialog.submit` | host → plugin (notification) | `dialogId`, `values` | — |
| `dialog.cancel` | host → plugin (notification) | `dialogId` | — |

**`dialog.open` returns immediately and the answer arrives later.** A dialog is open for as long as
a person is reading it; an RPC that stayed open for that would blow the 5 s timeout on every dialog
a user thinks about. The id comes back at once, and `dialog.submit` / `dialog.cancel` are the
answer. Exactly one of the two arrives for any dialog the host opened, including when the host
closes it during teardown (`cancel`).

**`sections[]` is the connection-field schema, unchanged.** The core already has a declarative
form language with manifest-load validation, per-type value validation and a renderer: the one
connection protocols use (`internal/domain/plugin/fields.go`, `ValidateManifestFields`,
`internal/usecase/plugin_fields.go`, `frontend/src/lib/fields/`). A dialog reuses it whole,
including the parts a schema off the wire does not get for free: `ValidateWireFields` compiles a
declared `validation.pattern` through the same safety screen a manifest pattern goes through, and
resolves `dependsOn` against the rest of the panel. A field whose dependency is off is not part of
the answer — its value is dropped and its `required` does not apply, the rule the renderer and
`SavePluginFields` already follow. Two field types are added, both to the shared schema rather than to a dialog-only dialect:

- `keyValue` — a repeatable list of string pairs. Labels, driver options, environment: the shape
  every "arbitrary map" field in every system has, which today has no representation and would
  otherwise be encoded as a textarea the plugin parses by hand.
- `code` — a read-only monospace block with copy. What a detail view is mostly made of.

`secret: true` is **rejected** on a dialog field. A secret's whole storage story is the vault, keyed
by connection and field id; a dialog has no connection and no persistence, so a "secret" one would
be a plaintext string with a lock icon on it. Plugins that need a secret still go through
`vault.getSecret` under its existing consent.

**`kind`** distinguishes a question from a presentation: `form` has submit/cancel and returns
values; `detail` has only a close button and never sends `dialog.submit`. One dialog per plugin at
a time — a second `dialog.open` is refused with `-32003` rather than stacking modals over each
other.

### 3. Node details — the panel a discovered node has

An extension of discovery, not a fourth verb family: it is addressed by `(sessionId, nodeId)` and
gated by the discovery capability plus `ui.nodeDetails`.

| Method | Direction | Params | Response |
|---|---|---|---|
| `discovery.describeNode` | host → plugin (request) | `sessionId`, `nodeId` | `{sections[], values{}, editable}` |
| `discovery.publishDetails` | plugin → host (request) | `sessionId`, `nodeId`, `sections[]`, `values{}` | `{ok:true}` |
| `discovery.applyDetails` | host → plugin (request) | `sessionId`, `nodeId`, `values{}` | ack |

The host asks when the user selects a node, and the plugin may push a newer snapshot at any time
with `publishDetails` — the same level-triggered, full-snapshot discipline `discovery.publish`
already uses, for the same reason: a delta protocol here would desync against a tree that is itself
snapshot-based.

**The host stores nothing.** `applyDetails` hands the values to the plugin, which persists them
where it already has permission to write (`${pluginData}`) under whatever key makes sense for the
resource it is describing. The core has no schema for "settings of a discovered node" and should
not acquire one: it does not know what a node is, and a node's identity across restarts is a
question only the plugin can answer. `applyDetails` acks receipt within 5 s and reports the outcome
by republishing, exactly as `invokeAction` does.

`editable: false` renders the panel read-only, which is the normal case for a node whose properties
are facts about a remote resource rather than local preferences.

## Limits

| Parameter | Value |
|---|---|
| Surfaces per plugin | `ui.maxSurfaces`, default 8, hard ceiling 16 |
| Surface `title` | 128 chars, sanitized like `Node.Label` |
| `surface.write` payload | 256 KiB (the frame cap) |
| `log` surface ring buffer | 8 MiB or 200 000 lines, whichever first |
| Open dialogs per plugin | 1 |
| Fields per dialog / details panel | 100 |
| `keyValue` pairs per field | 64 |
| `code` field content | 256 KiB |
| `describeNode` / `applyDetails` ack timeout | 5 s |
| `publishDetails` rate | 20/s per (plugin, connection) — the `discovery.publish` budget, shared |

## Security model

- Every plugin→host verb here passes the existing `Gate`: no `ui` capability, or no matching
  sub-grant (`surfaces` list, `dialogs`, `nodeDetails`), is `-32001` plus an audit record.
- **IDOR**: `surface.open` names a `parentSessionId` and is accepted only for a session the plugin
  holds an active binding for — the rule that already protects `vault.getSecret`, `channel.open`
  and `discovery.publish`. Every later `surface.*` call names a `surfaceId`, which the host resolves
  to its owning plugin; a surface id belonging to another plugin is `-32001`, not "not found", and
  is indistinguishable from an id that never existed.
- `discovery.publishDetails` reuses the `discovery.publish` ownership check unchanged.
- **Audit**: `surface.open`, dialog submit and `applyDetails` are audit-logged with the plugin id
  and the node/session they name — the same events ADR-014 logs for `invokeAction`, for the same
  reason: these are the points where a plugin acts on the user's screen or on remote state. A
  dialog's entry records which fields were answered and never their values: a form field holds
  whatever the user typed into it, and a refused submit is logged as denied so an attempt that the
  host rejected is not the one event missing from the log.
- **No markup from a plugin, ever.** Titles, labels, field values and `code` contents are text
  nodes; icons remain `<img src="data:…">` per ADR-014. Nothing a plugin sends is interpolated as
  HTML.
- All plugin-supplied display strings are stripped of control characters and Unicode bidirectional
  overrides (U+202A–U+202E, U+2066–U+2069) by the existing discovery sanitizer: surface and dialog
  titles, field labels, descriptions, placeholders, select option labels, a surface's error message
  and a dialog's per-field errors. A title cannot spoof the tab next to it, and a label cannot
  reorder the sentence above the button the user is about to press. Two values are deliberately
  exempt: a `select` option's **value**, which is data the plugin matches a submit against rather
  than something drawn, and a `code` field's content, whose control characters are the point of it
  — bidirectional overrides stay refused there by the field's own validator.
- Dialog fields may not be `secret`, so no new path exists by which vault material would be
  rendered or round-tripped through a plugin.

## Manifest surface versioning

`ui` is a new capability entry in `hostRegistry` with its own version starting at `1.0.0`. Per
ADR-012 this does **not** bump `PluginAPIVersion`: the envelope, framing and handshake are
unchanged, and `TestAPISurfaceAdditiveOnly` permits new capabilities within a major.
`TestFrozenAPISurface`'s golden file is regenerated after review. The two new field types
(`keyValue`, `code`) are additive to the field schema; existing manifests keep validating.

## Consequences

- Discovery plugins become able to complete an action end to end without the core learning what
  the action was.
- The tile grid gains a second kind of tab. Tabs stay addressed by a single id, and the surface's
  id space is disjoint from session ids, so tile layout, drag-and-drop and keyboard navigation are
  unchanged in shape.
- `Terminal.svelte` is split three ways: the xterm host, the I/O it is wired to (`TerminalIO`, one
  implementation per producer) and the grid sizing it had grown around it. The file was over the
  350-line limit before any of this, so it is a debt payment rather than a new cost.
- The connection-field renderer moves out of `connectionDetails/` into a shared `fields/` module
  used by three callers (connection editor, dialogs, node details). `keyValue` and `code` are part
  of that shared schema, so a connection protocol may declare them and the editor draws them.
- Closing a tab, cycling to the next one and the hotkeys that do either move to a tab layer that
  resolves what an id names. A tab id stopped meaning "a session" the moment surfaces existed.
- The composition root splits by subject (`main_plugin_*.go`) rather than growing a sixth screen of
  wiring in one function.

## Alternatives considered

1. **More view locations instead of surfaces** (`main.tab`, `main.panel`). Rejected: it hands a
   plugin an iframe where a terminal and a log viewer belong, so every plugin reimplements xterm,
   search and export in its own bundle, and each one is a separate XSS surface inside the main
   window. Surfaces give the plugin a stream and keep the rendering in the core.
2. **A generic `ui.render` verb taking a component tree.** Rejected: that is a UI framework across
   an IPC boundary, versioned forever. The three shapes above are the ones the product actually
   needs, and each maps onto something the core already draws.
3. **Letting a plugin mint a real session.** Rejected: see §1. A weaker second thing behind
   `sessionId` would undermine every ownership check keyed on it.
4. **Storing node settings in the core.** Rejected: the core cannot name a discovered resource
   stably across restarts, and persisting a plugin's opinion about a remote object is not core
   state. The plugin already has `${pluginData}`.
5. **A synchronous `dialog.open` returning the answer.** Rejected: a person is slower than any RPC
   timeout worth having.

## References

- ADR-011 — Binary duplex channel bus (lifetime coupling, backpressure)
- ADR-012 — Plugin API versioning
- ADR-014 — Discovery subtrees (node model, sanitization, audit, IDOR rule)
- [plugin-api.md](../plugin-api.md), [plugin-manifest.md](../plugin-manifest.md),
  [security-model.md](../security-model.md)
