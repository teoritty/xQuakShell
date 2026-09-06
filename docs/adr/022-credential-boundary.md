# ADR-022: Credentials Cross the Plugin Boundary Only Inside a User-Declared Scope

## Status

Proposed. Extends the trust model of [ADR-018](018-plugin-process-isolation.md) and the capability
contract of [ADR-012](012-plugin-api-versioning.md). Adds no on-disk axis by itself; the vault schema
changes it implies are governed by [ADR-017](017-release-and-compatibility-policy.md).

## Context

Two demands arrive together and pull in opposite directions.

A user with several machines wants the same connections on all of them. A user inside a company wants
connections and secrets to come from a central store that will never hand its data to a desktop
application permanently. Both are "credential sync" in conversation, and building either one into the
core picks a winner: whichever vendor is implemented first defines the shape every other integration
must imitate.

The existing model has no room for either. Its authorization anchor for credential access is
**an active session the plugin owns** ([security-model.md § Ownership](../security-model.md#ownership-idor)):
`vault.getConnection` and `vault.getSecret` are allowed only when `SessionManager` holds a live
session whose `pluginId` and `connectionId` both match. That anchor is exactly right for what it was
built for and it evaporates for credentials:

- replication runs with **no session at all** — it is a background job, there is nothing to own;
- provisioning runs **before** any session exists — its whole purpose is to supply what a session
  will be made of.

A second gap sharpens the first. `capabilities.network` offers a manifest allowlist or
`allowArbitraryOutbound`. The address of a self-hosted sync server is unknowable when the bundle is
built, so a sync plugin must request arbitrary outbound: **the plugin that handles every credential
is forced to ask for the widest grant in the system.**

Nothing here can lean on the sandbox. On macOS there is none and there will not be one, and on Linux
below ABI 4 the network is uncovered. Every rule below therefore holds at the IPC gate, where the
host decides what it will do *for* a plugin, and is stated as an honesty control wherever it does
not.

## Decision

### 1. Ownership and exposure are two axes, not one

Conflating them is the modelling error that makes the rest impossible.

| Axis | Meaning | Who may change it |
|---|---|---|
| `owner` | who created the object and owns its lifecycle: `core` or `plugin:<id>` | nobody; it is set at creation and has no setter |
| `exposure` | whether the object may be seen by, or travel through, a given plugin | the user only, by an explicit gesture, never a plugin |

A user putting their own connection where a sync plugin can see it is an **exposure** change. It is
not a transfer of ownership, and no RPC that reassigns `owner` exists in the protocol.

### 2. Exposure is a core-owned folder, and inside it the tree is ordinary

A plugin declares in its manifest that it needs a scope. **The core creates, names, renders and owns
exactly one folder per such plugin**, badged with its provenance. The plugin cannot create a second
one, rename it, delete it, or draw anything resembling it.

**Inside that folder the user builds whatever tree they like** — arbitrary nesting, any depth — and
that tree is part of what replicates: it arrives on the other device in the same shape. The scope
root is a core-owned fixture; its descendants are ordinary user objects that happen to be in scope.
`ConnectionFolder.ParentID` already nests, so the shape is available today.

Membership is positional and therefore visible: what is inside leaves the machine, what is outside
does not, and default-deny is the whole point. The known cost, accepted deliberately: an object lives
in one place in a tree, so a user who wants both synced and local hosts under one label keeps two
labels. Visibility was chosen over convenience, and this is what that costs.

The folder is how the user *edits* the set. It is not the enforcement: that lives in the usecase gate
beside the existing ownership check, because the frontend is not a trust boundary.

Moving anything into a scope discloses its **transitive closure** — the identities, passwords and
jump hops that travel with it — and requires confirmation. Moving a subtree means the closure of
everything beneath it, so the disclosure summarises by count and enumerates only what carries
elevated risk. An object referring to a `NonExportable` identity can never enter a scope, and this
one is a refusal rather than a warning: a promise that a key cannot leave the vault outranks any
gesture, including a deliberate one. An object inside a scope may not refer to an object outside it.

### 3. Outside the scope is an invariant; inside it is an informed decision

**Nothing outside a scope leaves the vault.** Inside a scope, secrets leave the machine — that is
what the user asked for by putting them there. The guarantee is not weakened by an exception, it is
*bounded*: a model with a drawn edge can be checked, one with an exception can only be hoped for.

Removing an object from a scope removes it locally and says plainly that the copy already sent is
outside the application's control. There is deliberately **no "delete from server" button**: it
would mean trusting a plugin to carry out a deletion, and would lie in the one moment the user needs
the truth.

### 4. Ask for an operation, not for a secret

Where a protocol permits it, the host asks the plugin to *do* something rather than to *hand over*
something. `PluginAuthProvider.Sign` is the existing model: the key never leaves the plugin and the
host sees a signature. A secret crosses the boundary only where the protocol itself requires
plaintext.

### 5. A secret that must not be persisted is a type that cannot be serialized

A plugin-supplied secret is carried in a type whose `MarshalJSON` fails and which is unreachable from
`VaultData`, so "it ended up in the vault or a backup" becomes a build or test failure rather than an
audit finding. This covers the persistence path and only that path: Go cannot stop a value being
copied, formatted into a log line, or read out of process memory. Those stay the business of the
logging rules in CLAUDE.md §5.3 and of the same-user boundary.

### 6. While the vault is locked, the core does nothing for a plugin

Locking denies every credential-bearing RPC. It does **not** kill plugin processes: the gate already
stops all new data flowing from the core, and a plugin that survives a lock has gained nothing it
could not have taken before it. Bouncing every process to clear memory a plugin could have written
to its own instance directory anyway buys little and costs restarts in the most fix-heavy code in
the repository.

**The scope anchor of decision 1 is void while locked.** This is the load-bearing half. Today a
plugin is powerless after `lock` incidentally — sessions are closed and `GetData` refuses — and a
scope is a property on an object, which survives a lock trivially. Introducing the scope anchor
without this clause would *grant* plugins post-lock access they do not have today, by way of the
mechanism added to constrain them.

The accepted consequence is that **background synchronisation while locked is impossible.**
Credentials move at unlock, while the user is present, which is also when the confirmations of
decision 2 and decision 7 can honestly be shown. The residual risk — a plugin that keeps its own
memory and its own network after a lock — is recorded rather than claimed away.

Uninstall is different and does stop the process: see decision 7.

### 7. The core is authoritative and deletion never arrives over the wire

Neither a plugin nor a server can cause the loss of a core-owned object. A plugin may mark its own
objects as gone; a synchronisation that would delete anything — including a folder, and therefore
everything beneath it — is shown and confirmed. Uninstalling a plugin stops its processes, revokes
its grants, kills its ephemeral secrets and **freezes** rather than deletes its objects: metadata
stays, marked orphaned, secrets no longer resolve, and connecting through them is refused. Removing
a plugin must never be a data-loss event, or users will hesitate to do it at exactly the moment
hesitation is most expensive.

### 8. Host trust is never provisioned

`KnownHosts` and `PeerTrust` are not accepted from a plugin in any form, and the fields do not exist
in the provisioning schema — they are absent, not ignored. A plugin may prompt; the user decides, as
with TOFU today. A transport that can inject a host key does not need to break the vault to read
every session.

### 9. Security-relevant configuration is declared by the manifest and owned by the core

A manifest declares **slots**, not values: named fields the user must fill, with a label, permitted
schemes, and whether they are required. Values never exist in a bundle, are stored by the core, are
edited in core-drawn UI, and are not writable by the plugin. The effective outbound allowlist is the
manifest's patterns plus the filled slots, and a sync plugin therefore never needs
`allowArbitraryOutbound`.

Slots inherit the existing dial behaviour without restating it: the host resolves the target and
dials the resolved address, so a name whose resolution drifts between check and connect is already
covered ([security-model.md § Network outbound](../security-model.md#network-outbound-ssrf)).
Private, loopback and link-local targets keep requiring the same explicit consent they require now.

A server address is configuration and is visible. A token for a corporate store is a secret and
belongs in `VaultData.PluginSecrets`. They are different things and are not stored together.

**Any field whose value becomes a security decision is drawn by the core.** The master password
prompt, the endpoint slot, and the closure confirmation of decision 2 are all instances of one rule;
a plugin UI surface cannot host them, because a field drawn by untrusted code cannot be told apart
from a field imitating it.

### 10. A grant is bound to what was asked for, not to a version

Consent is keyed by `(pluginID, granted permission set)` and carries a scope, replacing
`SecretAccessGranted map[string]bool`. The bundle fingerprint is recorded in the audit log on every
start, so which binary held a grant is always answerable.

Re-consent is required when the **effective requested permissions widen**, not on every update. A
widening includes a new capability, a new feature within one, a new value inside an existing list —
`getSecret: ["password"]` becoming `["password","privateKey"]` — and a new configuration slot.
Prompting on every version instead would show the dialog so often that users stop reading it, and
they would then click through the one release where the permissions really did grow. A plugin id
whose bundle changes without widening keeps its consent; a bundle that widens gets a dialog naming
exactly what was added.

## Alternatives rejected

- **One `sync` capability.** Every integration would have to present itself as a synchronisation
  server. A corporate store does not persist secrets on a desktop and issues short-lived leases; it
  would have to lie about its own model to fit. This is the vendor lock the ADR exists to avoid, and
  it arrives disguised as generality.
- **Telling users to replicate `vault.age` with an off-the-shelf file sync.** Genuinely covers much
  of the single-user, several-machines case at zero cost, and is not rejected because the ciphertext
  would be at risk — it would not be; the file is encrypted at rest and the tool never sees a key.
  It is rejected as *the* answer for two reasons it cannot address: the vault is rewritten whole on
  every flush, so concurrent edits resolve as last-writer-wins or as two files a human must pick
  between; and a file-level tool has no notion of version order, so an old vault reappearing is
  indistinguishable from a new one and silently resurrects deleted connections and revoked trust.
  Users who accept those two properties may of course still do it.
- **Letting the plugin create the scope folder.** It would let a plugin create a *second* folder
  resembling the first, and personal servers would be dropped into it by the user's own hand — the
  exact outcome the scope exists to prevent. An undeletable UI object authored by untrusted code is
  also a standing surface for a prompt that imitates the host.
- **Marking exposure with a per-object flag instead of a folder.** It would preserve a user's
  existing tree, but it makes the most consequential property in the product invisible until
  something is opened. Position in a tree is legible at a glance; a checkbox is not.
- **Making the folder purely a UI convention.** The frontend is not a trust boundary. A scope that is
  not enforced at the gate is decoration.
- **Changing `owner` when the user moves an object into a scope.** Indistinguishable at the gate from
  a plugin claiming an object it did not create, which is the escalation the ownership axis exists to
  prevent.
- **Killing plugin processes on lock.** Rejected as decision 6: the gate denial already stops every
  new flow of data, so the marginal gain is a plugin's own memory, which it could have persisted
  before the lock in any case. The cost is restart churn in
  `internal/infra/plugin/process_host_start.go`, the file with the heaviest fix history here.
- **Keeping background sync by holding the vault key across a lock.** It would deliver the feature by
  destroying the guarantee the lock exists to provide.
- **A "delete from server" action.** It reports success based on the word of the component that is
  assumed hostile.
- **Deleting plugin-owned objects on uninstall.** It makes removing a suspicious plugin a
  destructive act, so users delay it.
- **Relying on the OS sandbox for the outbound allowlist.** True on Windows, partial on Linux, and
  absent on macOS. Stated as what it is: a control over an honest plugin that has been compromised,
  and not a boundary against a hostile one.

## Consequences

- **The gate gains a second authorization anchor.** Alongside "an active session this plugin owns"
  there is now "the user placed this object in this plugin's scope, and the vault is open". Both are
  explicit; neither is "the plugin asked".
- **Synchronisation happens at unlock and is visible.** Fewer moving parts, less magic, and a wiped
  or hostile server cannot quietly propagate its emptiness — the user sees a confirmation and can
  uninstall the plugin instead.
- **Folder structure becomes replicated data.** A transport must therefore reconcile a *tree* —
  moves, renames, subtree deletions — not a flat list of connections. This is a real increase in the
  difficulty of whatever implements replication, and it follows directly from decision 2.
- **`PluginSettings` grants change shape**, from `map[pluginID]bool` to permission-set-keyed scoped
  grants, and that is a vault schema change under ADR-017.
- **`ConnectionFolder` and `Connection` gain provenance and scope**, and connection validation gains
  the composition rule of decision 2. This is security-critical code under CLAUDE.md §2.3.
- **`lockNow` gains no new responsibility**; what changes is that the scope anchor must consult lock
  state, which is a gate concern rather than a process-lifecycle one.
- **What is not solved, and is recorded rather than implied:** the window between a revocation at an
  external store and the next unlock; device revocation, which is not real without re-keying the
  vault payload; the observability of blob size and update timing to a transport; a plugin process
  that outlives a lock holding its own memory and network; and every rule here on a platform that
  cannot confine a plugin.

The full adversary model, the vector catalogue with per-vector defences and verification, and the
constraints this decision places on any future transport are in
[the credential security model spec](../superpowers/specs/2026-09-05-plugin-credential-security-model-design.md).
The types and ports that carry these decisions, and the order they would be built in, are in
[the abstractions design](../superpowers/specs/2026-09-06-plugin-credential-abstractions-design.md).
