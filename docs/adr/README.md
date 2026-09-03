# Architecture Decision Records

One record per decision that constrains later work: what was decided, what it rules out, and what
was rejected on the way. Ask before diverging from one — a rule whose reason has been lost tends to
be treated as arbitrary and worked around.

| ADR | Decision |
|-----|----------|
| [007](007-host-filesystem-trust.md) | Host Filesystem Trust — three filesystem zones, each behind its own port |
| [008](008-session-embed-surfaces.md) | Session Embed Surfaces — plugin UI in a session tab over a tunnelled WebSocket |
| [009](009-session-manager-decomposition.md) | SessionManager Decomposition — a facade over per-concern files |
| [010](010-composition-root.md) | Composition Root Policy — DI lives in `main*.go`; `app.go` is a Wails facade |
| [011](011-binary-channel-bus.md) | Binary Duplex Channel Bus for Plugins |
| [012](012-plugin-api-versioning.md) | Plugin API Versioning — the core is the authority on compatibility |
| [013](013-native-drag-out.md) | Native Drag-Out of Files to the OS |
| [014](014-discovery-subtrees.md) | Discovery Subtrees |
| [015](015-plugin-ui-surfaces.md) | Plugin UI Surfaces |
| [016](016-ui-plugins-ship-bundles.md) | A Plugin With a UI Is Published as a Bundle |
| [017](017-release-and-compatibility-policy.md) | Release and Compatibility Policy — six version axes, four frozen contracts, latest release only |
| [018](018-plugin-process-isolation.md) | Plugin Process Isolation — AppContainer on Windows, Landlock on Linux, refuse rather than fall back |
| [019](019-interface-language.md) | Interface Language — packs on disk, security warnings that a pack cannot reword, plugins told which language to write in |
| [020](020-local-terminal.md) | The Local Terminal — an RPC with no arguments, a shell named by id and never by path, unreachable from a plugin |
| [021](021-vault-recovery-key.md) | A Second Credential Opens the Vault — a wrapped vault key, one error for both credentials, a key shown once and never stored |

## Numbers below 007

The sequence here starts at 007. Four earlier numbers are cited in prose elsewhere in these docs,
but no ADR document for them has ever existed in this repository — the decisions were made before
the directory was, and were written up as prose instead. Follow the citation to the section that
records it rather than looking for a file that is not here:

| Cited as | Where the decision is actually written down |
|----------|---------------------------------------------|
| ADR-001 — SSH fast path vs plugin sessions | [architecture.md § Plugin seam](../architecture.md#plugin-seam-sessionconnector) |
| ADR-002 — Secrets | [security-model.md § Secrets](../security-model.md#secrets-adr-002) |
| ADR-003 — Process isolation | [security-model.md § Process isolation](../security-model.md#process-isolation-adr-003) |
| ADR-006 — Portable data layout | [security-model.md § Portable data layout](../security-model.md#portable-data-layout-adr-006) |

New records continue from the highest number present in this directory.
