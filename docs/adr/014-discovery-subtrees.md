# ADR-014: Discovery Subtrees

## Status

Accepted

## Context

- ADR-011 §"Application to discovery" described discovery as a deferred direction: the `channel` capability (`exec`/`embed-stream`/`tcp-relay`/`udp-relay`) is implemented, `discovery` is not.
- The connection tree (`frontend/src/lib/remoteTree/types.ts`) knows exactly two node kinds: `folder` and `connection`. There is no way to draw a subtree *inside* a connection.
- Multiple discovery plugins on the same machine, with no grouping, would produce one interleaved flat list — unusable once more than one plugin enumerates resources under the same SSH connection.

## Decision — node model

The node is technology-neutral. The core contains no mention of Docker, Kubernetes, or Postgres — those are examples of what a plugin might draw, never part of the contract.

```
Node{ID, ParentID, Kind, Label, IconID, Order, Status, Actions, DefaultActionID}
Kind   ∈ {group, instance}          // group has children, instance is a leaf
Status {Tone, Color, Tooltip}       // filled ONLY by the plugin
Tone   ∈ {ok, warn, error, busy, neutral, unknown}
Color  — optional, strictly ^#[0-9a-fA-F]{6}$
Action {ID, Label, IconID, Danger, Confirm, Multi}
```

**Branch** state is a separate entity, filled ONLY by the host:

```
Branch{State, Error, Truncated{Shown, Total}}
State ∈ {loading, ready, error, stale}
```

The split is normative: the host never writes to `Status`, and the plugin never sends `Branch.State = stale` or `Truncated`. If a plugin's `discovery.publish` payload contains them anyway, the host **silently drops those fields and processes the rest of the snapshot** — this is not a protocol violation or a gate denial. Sending `Branch` fields is ordinary plugin-author carelessness, not an attack, and rejecting an otherwise-valid snapshot over one stray host-only field would be disproportionate: the children the plugin reported are still real and still shown.

`instance` is a leaf, normatively and not just descriptively: a `publish` naming an instance as its parent is **refused** with an error the plugin author sees. Nothing legitimate is blocked — a node that should be expandable but is currently empty is `kind: group` with `children: []`, which draws a chevron and an empty branch. Accepting the publish instead would produce a row that is a leaf and a branch at the same time.

**Icons**: `IconID` may be set on a node at any depth. Inheritance from the nearest ancestor is a fallback only, used when a node did not set its own icon. So a `docker` group with its own icon, its `containers`/`images`/`volumes`/`networks` subgroups each with their own, and instances inside `volumes` inherit the `volumes` icon — not `docker`'s.

**Node order**: `Order` (set by the plugin), then `Label`, then `pluginID`.

## Decision — data flow (level, not edge)

Three atomic verbs, no others:

| Direction | Method | Payload |
|---|---|---|
| host → plugin (notification) | `discovery.observe` | `{sessionId, nodeIds: []}` — the FULL set of currently expanded nodes; `""` = the connection root |
| plugin → host (request) | `discovery.publish` | `{sessionId, nodeId, state: "loading"\|"ready"\|"error", error?, children: []}` — a snapshot that fully replaces `nodeId`'s children → `{ok:true}`/error |
| host → plugin (request) | `discovery.invokeAction` | `{sessionId, nodeIds: [], actionId}` → ack/error |

**Why level, not edge.** "A node was expanded" is a front-edge event; a lost one (plugin crash, a race at connect time) leaves the node empty forever, with nothing to repair it. `observe` is a level: an idempotent, full set, resent on every change AND on every plugin (re)start. This also removes load at the source — a plugin stops polling branches the user has collapsed.

**Why `publish` is a request.** It carries no answer worth having — the host has nothing to tell a plugin about a snapshot the plugin composed — but it names a `sessionId`, and that makes it the one plugin→host verb here that can be *refused*: by the capability gate with `-32001`, or by the IDOR check when the session is not one the plugin holds a binding for. A notification has no channel for a refusal, so a plugin following the wire contract literally would be denied in silence and could not distinguish that from a message the host never received. The `{ok:true}` result therefore means only "accepted for processing"; a snapshot for a collapsed branch or a session that has stopped leading is accepted and then dropped, by design.

`nodeIds` in `invokeAction` is always a list, even for a single node. There is deliberately no separate bulk verb: a single action is a mass action over a list of one.

Deltas are deliberately absent: a per-node snapshot removes a whole class of desync bugs, and the 500-children cap keeps a snapshot cheap.

`sessionId` is the transport address for the plugin. It never reaches the frontend — everything there is addressed by `connectionId`.

## Decision — manifest

```json
"capabilities": {
  "discovery": { "parentProtocols": ["ssh"] }
},
"contributions": {
  "discoveryIcons": [
    { "id": "docker", "asset": "ui/icons/docker.svg" }
  ]
}
```

- `parentProtocols` is not decorative: the host addresses `observe` only to plugins whose list contains the protocol of that connection.
- Asset paths are validated by the existing `ValidateViewAssetEntry` **once, at install time**; there are no paths at all on the hot path — `iconId` refers to an already-validated asset. Reusing that validator means icons live under the bundle's `ui/` tree like every other declared asset: a path outside it fails manifest validation, so the plugin does not install at all.
- Extensions `.svg`/`.png`/`.ico`; ≤ 64 assets per plugin, ≤ 64 KiB each, ≤ 1 MiB total.
- Icon bytes are read **once, when the plugin enters the registry**, encoded as base64 data URIs and cached there; they reach the frontend on the existing `ListPlugins` call as `discoveryIcons: {iconId: dataUri}`, so there is no icon endpoint taking a plugin ID and an asset name from the frontend. This deliberately differs from view assets (`internal/infra/plugin/assets/handler.go`), which are read from disk per request: a discovery icon is fetched up to 64 times per plugin on a path that repaints, and the cache is also what keeps "an unreadable asset is logged once per plugin" true. `Register`/`Unregister` re-read, so install, update and removal are all reflected; only editing a file inside an already-installed bundle goes unnoticed, which is not a supported scenario.
- No separate install-time consent: discovery by itself is metadata only — the actual work runs through `channel`/`exec`, which already carries consent. `PermissionSummary` gets one line: "Show discovered resources under your connections".

## Limits (v1, not overridable by the plugin)

| Parameter | Value |
|---|---|
| Tree depth | 8 |
| Children per publish | 500 |
| Nodes per (plugin, connection) | 2000 |
| ID length | 256 |
| `Label` | 128 |
| `Tooltip` | 256 |
| Actions per node | 16 |
| Nodes in one `invokeAction` | 200 |
| Publish rate | 20/s per (plugin, connection) |
| Frontend emit coalescing | 100 ms per node |
| `invokeAction` ack timeout | 5 s |

Both budgets are keyed by **connection**, not by session: the host stores one tree per connection
whatever session currently carries the traffic, so that is the side that owns the memory and the
only side that can enforce a ceiling on it. A plugin sees only sessions, which is why an earlier
draft of this table said "session"; the code and `internal/domain/discovery/limits.go` have always
said connection.

Exceeding the children limit is truncation with `Truncated{Shown, Total}`, not a refusal: the user should see something and understand the list is incomplete, rather than see nothing.

**The tree is not virtualised**, and that is what the 500-children cap is for. Rendering every visible row is what keeps the connection tree one flat list that keyboard navigation, selection and drag-and-drop all walk the same way; a windowed renderer would have to answer what "the next row" means when it has not been rendered yet, for four node kinds at once. The cap bounds the worst case instead: one branch contributes at most 500 rows, and only while it is expanded. Virtualisation is out of scope for v1 and would be worth revisiting only if the cap itself were raised.

## Leading session

A plugin that stops answering — a crash, or an idle suspension — puts its branches in `stale` and keeps its nodes: the process is expected back, the observed set is replayed to it, and the branches refill, so deleting them would turn a recoverable absence into a subtree that visibly vanishes and returns.

A plugin drawing a subtree is **in use** for as long as it holds a binding, even though it serves
no session of its own and owns no view panel. Without that rule every "is anyone using this?" check
in the host answered no: the idle sweeper reclaimed the plugin after five quiet minutes — quiet
being the normal state of a tree the user has finished expanding — and the crash supervisor declined
to restart it, so the two transitions below could never happen and a subtree went stale and stayed
there until the connection was re-established.

`stale` therefore means "expected back". Once that stops being true — after **3 failed restart attempts**, the host's fixed limit — the branches become `error` carrying a reason the row shows. The observable difference is what the user is told: `stale` says the tree is being re-confirmed, `error` says nobody is coming. Both refuse actions inside the subtree, and neither is terminal in the store: an `error` branch is repaired by an ordinary `publish`, so a plugin that is started again later — by the user, or by the next activation — refills it with no separate recovery path. The host never deletes the nodes on either transition; only the loss of the last `ready` session does that.

**Known limitation (v1): recovery from a crash is not observable, and neither transition above
happens today.** The lifetime of a plugin's child process is tied to the context of the call that
started it — `PluginSupervisor` passes its own per-attempt context to `EnsureRunningForSession`,
which reaches `exec.CommandContext`, so the `cancel()` on the success path kills the process it has
just brought up. The consequence for this ADR is concrete: the observed set is replayed to a process
that is already dying, the branches do not refill, and because the supervisor correctly counts that
restart as a success, its attempts never run out and the branches never reach `error` either. They
stay `stale` — "expected back" — indefinitely, until the connection is re-established.

That is stated here, beside the promise, rather than in a footnote, because this document makes a
branch state that says something the user can check and find false the one thing it forbids, and a
promise the system does not keep is the same failure one level up. The rule above is the intended
behaviour and the code implementing it is in place; what is missing is underneath it. The context
defect is not specific to discovery — the same pattern appears in the plugin start, command, view,
connector and install paths, and killing a process on return from the call that started it affects
every plugin — so it is separate work, and the `error` transition becomes reachable once it lands.

In the tree, there is exactly one subtree per connection, no matter how many tabs are open. But a plugin can only enumerate resources through an authenticated transport, so the host designates one **leading** session — the earliest one in `ready` state — and passes only that one as `sessionId`. If the leading session closes while others are still alive, the role passes to the next `ready` session, and branches get `stale` for the duration of the handover. If no `ready` session remains, the tree state is deleted: nothing is cached or persisted.

## Manifest surface versioning

Adding `discovery` to `hostRegistry` is purely additive — a new capability entry, own `Version` starting at `1.0.0` — and does not bump `PluginAPIVersion`: per ADR-012 the envelope version covers wire framing and the lifecycle/handshake, which are unchanged, and `TestAPISurfaceAdditiveOnly` already permits new capabilities within a major (it only fails a *removed or downgraded* one). `TestFrozenAPISurface`'s golden file was regenerated to include it after review.

## Security model

- New manifest capability `discovery`; `discovery.publish` is gated by the same `Gate` that denies undeclared methods with `-32001` and writes to the audit log.
- `observe`/`invokeAction` go host→plugin and do not pass through the gate — the host simply never addresses them to a plugin without the capability.
- IDOR: `publish` is accepted only for a session the plugin holds an active binding for — the same rule that protects `vault.getSecret` and the tunnel proxies.
- `invokeAction` (including mass actions) is audit-logged with the full list of nodes.
- An unknown `iconId` does not fail the publish — the node is published without an icon, with a log entry.
- **XSS**: icons render exclusively as `<img src="data:image/svg+xml;base64,…">`. Inline SVG insertion would execute scripts from the plugin's bundle inside the main window.
- `Label`/`Tooltip` are stripped of control characters and Unicode bidirectional overrides (U+202A–U+202E, U+2066–U+2069): without this, a resource name could visually spoof the neighboring tree row.
- No new privilege for the plugin process: discovery carries metadata only.

## Search (v1 limitation)

The connection tree's search box filters folders and connections. It does **not**
filter discovery rows, and it does not auto-expand a discovery node.

Auto-expansion is the part that must not happen: expanding is what publishes an
`observe`, so a single keystroke in the search box would fan `observe`/`publish`
out across every connection at once — the exact load the level-based protocol
exists to avoid.

Filtering the rows already on screen would have been safe, but it was left out
too: a search that silently reaches into some subtrees (expanded ones) and not
others (collapsed ones) reports an absence it never checked, which is worse than
not searching at all. The tree shows an explicit hint saying discovered resources
are not searched, so the user is told rather than misled.

Revisiting this means deciding what "not found" means for a branch nobody is
observing — the honest answers are either "search only what is loaded, and say
so" (today's behaviour) or a plugin-side search verb, which is a new wire verb
and out of scope for v1.

## Actions

Fully opaque to the core. A node carries `actions[]` and `defaultActionId`; the core draws the menu and calls `invokeAction`, and what the action does is known only to the plugin. A group carries its own `actions[]`: "stop all" is an ordinary action on the group, NOT an automatic core expansion into a list of children's actions. The core does not invent an "apply to all descendants" semantic — a collapsed group has no children in memory at all.

Mass actions: selection is limited to children of the same parent; an action is shown when it has `multi: true` and is present on every selected node (matched by `actionId`). The result arrives as an ordinary `publish` — the plugin itself moves nodes to `busy` and publishes the outcome; partial success is expressed by some nodes ending up `ok` and others `error`. There is no separate reporting protocol.

`invokeAction` ack must arrive within 5 s. Long-running work does not hold the RPC open: the plugin acknowledges receipt and reports back via `publish`.

## Consequences

- Discovery gets no new transport — only a new capability layered on the existing `channel`/`exec`/`tcp-relay`.
- Existing plugins are unaffected: everything is additive.
- The core stays technology-neutral; adding a Docker or k8s plugin requires no core changes.

## Alternatives considered

1. **`nodeExpanded` event on the existing bus instead of `observe`.** Rejected: an edge instead of a level, a lost event is unrecoverable, and it would require a separate resync mechanism.
2. **Deltas instead of snapshots.** Rejected: state desync is a whole class of bug, and the 500-children cap makes a snapshot cheap.
3. **Separate `invokeBulkAction` verb.** Rejected: two paths for one meaning, against the atomic-API principle.
4. **Persisting the tree across restarts.** Rejected: discovery reflects remote reality, not local config; a stale cache is worse than emptiness.

## References

- [plugin-api.md](../plugin-api.md)
- [plugin-manifest.md](../plugin-manifest.md)
- [security-model.md](../security-model.md)
- ADR-008 — Session embed surfaces
- ADR-011 — Binary duplex channel bus (§Application to discovery)
- ADR-012 — Plugin API versioning
