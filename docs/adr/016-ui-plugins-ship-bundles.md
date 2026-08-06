# ADR-016: A Plugin With a UI Is Published as a Bundle

## Status

Accepted

## Context

A plugin can be installed two ways. **Install bundle…** takes an `.xqsp` — a ZIP of `plugin.json`,
the binary, `ui/` and the author's `SHA256SUMS` — and unpacks it whole. **Add GitHub repository**
reads `xqsp.json` from the repository, picks the release asset matching the host platform, and
installs that.

The GitHub route was built around one assumption: a release asset is an executable. It staged the
downloaded binary, wrote a `plugin.json` regenerated from `xqsp.json` next to it, and computed
`SHA256SUMS` over the two. Everything else an author packages — in practice, the whole `ui/` tree —
never existed on that path.

Nothing caught it. `views[].entry`, `embedEntry` and `discoveryIcons` are validated lexically
(they must resolve under `ui/`) but never checked against the disk, so a plugin whose interface
was entirely absent installed cleanly, started, connected, and answered the first request for its
own page with a 404 — a symptom several layers away from its cause.

Two further consequences of regenerating the tree, both silent:

- the checksums the host later validated were the host's own, so validation proved nothing about
  what the author shipped;
- a manifest signature is bound to the digest of `SHA256SUMS` (ADR: bundle signing), so rewriting
  that file made every signed plugin unverifiable through this route.

## Decision

**1. A release asset may be a bundle, and a bundle is installed verbatim.**
`.xqsp` is recognised as a platform asset (`<name>-<os>-<arch>.xqsp`). It is extracted into the
staging directory unchanged: the author's `plugin.json` and `SHA256SUMS` are what land on disk, so
the existing checksum validation and signature verification become real statements about the
author's package rather than about ours.

**2. Where both shapes exist for one platform, the bundle wins.**
A publisher adding bundles keeps the bare binaries for hosts that predate them. The install
resolves a platform to exactly one asset, and it must not depend on the order the GitHub API
happens to list them in.

**3. A plugin that declares `ui/` assets cannot be installed from a bare binary.**
The refusal happens before the download, and names the asset the publisher has to add. The
alternative — installing with a warning — produces exactly the failure this ADR exists to remove.
"Declares `ui/` assets" means view entries, an embed entry under `capabilities.session.embed`, or
discovery icons. It deliberately does **not** include `capabilities.ui`: surfaces, dialogs and node
details are drawn by the host from IPC data, so a plugin can declare them and ship no files.

**4. A bundle's manifest id must match the repository's.**
The id decides the install directory, so a bundle that disagrees would install itself over — or
masquerade as — a different plugin. Versions are logged, not enforced: the repository manifest
legitimately comes from the default branch when a tag carries none, so an older tag would fail for
a naming reason rather than a real one. Capabilities need no comparison, because consent is
already computed from the staged manifest.

**5. Declared `ui/` files must exist, checked at install and only at install.**
The check lives in `loadSource`, which both install routes pass through and which startup
discovery does not. A load-time refusal would drop an already-installed plugin from the registry,
and uninstall resolves through the registry — users of the broken installs this decision addresses
would be left unable to remove them. At install, the refusal keeps a bad tree out and reaches the
user during preview, before any consent is given.

## Consequences

- Authors of plugins with a UI must publish per-platform `.xqsp` assets. Headless plugins may keep
  publishing bare binaries; nothing about their install changes.
- A host older than this change cannot install a bundle-only release: it reports the platform as
  unsupported. That is the intended trade for a publisher who chooses bundles only — an honest
  refusal instead of an installation that silently lacks its interface.
- Signature verification becomes meaningful on the GitHub route for the first time.
- Existing installs, including bare-binary ones made before this change, keep loading and can be
  uninstalled normally.
