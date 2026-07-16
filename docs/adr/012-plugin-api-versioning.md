# ADR-012: Plugin API Versioning

## Status

Accepted

Supersedes the `0.3.0-dev` core-version bump anticipated in ADR-008: plugins no longer gate on the core version at all (see Decision §1).

## Context

Ahead of the release we must **freeze the plugin API** so third parties can write plugins against a stable contract. The pre-freeze scheme was a hole:

- `HostCoreVersion` (`0.2.0-dev`) was the only enforced gate — a coarse `major.minor.patch >=` compare against an optional `minCoreVersion`, with pre-release suffixes silently stripped.
- `coreAPIVersion = "1.0.0"` was sent to plugins at `initialize` but **nothing validated it** — advisory dead weight.
- There was **no per-feature versioning**, **no negotiation**, and **no handling of a plugin built against a newer API than the host**.

The core problem is that a single monotonic version is a *lie*: bumping the surface for capability A wrongly rejects a plugin that only uses capability B, because one number conflates independent contracts ("formally incompatible, factually compatible").

## Decision

### 1. Two version axes, split by contract direction

- **`pluginApi` — the protocol envelope.** ONE semver (`PluginAPIVersion`, frozen at `1.0.0`). Covers the wire framing (ADR-011), the JSON-RPC envelope, the `initialize` handshake shape, lifecycle, and the error-code space. Replaces the dead `coreAPIVersion`.
- **Capabilities — the provided surface.** Each capability (`network`, `filesystem`, `events`, `vault`, `session`, `auth`, `tunnel`, `channel`) carries its **own** semver plus named **feature flags**, defined in the capability registry (`internal/domain/plugin/api_registry.go`).

**Plugins gate only on `pluginApi` + capability versions/features — never on the core or app version.** `HostCoreVersion` and the app/product version become purely informational (shown in About). `minCoreVersion` survives only as a deprecated legacy shim. This removes the core-vs-API conflation entirely.

### 2. Compatibility rule (per axis)

A host satisfies a requirement iff `host.major == req.major && host.minor >= req.minor` (patch never gates — patch is a non-contractual fix). Applied identically to `pluginApi` and each required capability. A required feature flag absent from the host's set is rejected by name. Pre-release suffixes are ignored, so a host dev build satisfies a released requirement.

### 3. Manifest `requires{}` block

```jsonc
"requires": {
  "pluginApi": "1.0.0",
  "capabilities": {
    "vault": { "min": "1.0.0", "features": ["getSecret"] }
  }
}
```

Values are strict `MAJOR.MINOR.PATCH` with **no** pre-release suffix (a plugin may not depend on an unstable API). A plugin may only require a capability it also grants in `capabilities{}`; a granted capability with no explicit requirement gets an implicit baseline (`<major>.0.0`).

### 4. Enforcement — install gate + runtime handshake

- **Install/discovery gate** — compatibility is checked separately from well-formedness. `Manifest.Validate` only verifies the manifest is structurally sound and references granted capabilities, so parsing and listing stay tolerant of plugins this build cannot run (e.g. the GitHub listing can still display them). `Manifest.CheckHostCompatibility` (backed by `Negotiate`) resolves the effective requirement, checks it against the host registry, and returns a structured `IncompatibilityReport`; discovery/install gate on it so incompatible plugins stay out of the active set, and the GitHub install preview surfaces the exact missing items and blocks Install.
- **Runtime handshake** — at `initialize` the host re-checks the effective requirement against the **live** registry (catching skew the static manifest can't) and advertises its full `APIDescriptor`. The host is the sole authority; a plugin's echoed set is never trusted for enforcement. Fail closed on any skew.
- **Capability gate** — beyond the capability grant (the authorization boundary, unchanged), a method mapped to an above-baseline feature is allowed only if the plugin negotiated a capability version that reaches it (`featureVersions` in `gate.go`).

### 5. Stability contract — additive-only + deprecation window

Within a major: only additions (minor bumps / new feature flags). Removal or a breaking change requires a major bump, and only after the item is marked `deprecated` and kept working for **≥ 2 minor releases and until the next major**. Two CI guard tests protect this: a golden snapshot of the advertised surface (`TestFrozenAPISurface`) and an additive-only check (`TestAPISurfaceAdditiveOnly`).

## Runbook (for plugin authors and maintainers)

- **Add a feature flag** to capability X: append it to X's `Features` in `api_registry.go`, bump X's minor version, regenerate the golden (`go test ./internal/domain/plugin -run TestFrozenAPISurface -update-golden`), review the diff. Plugins that require the new feature declare `"features":["…"]` and a `min` at the new minor.
- **Add a method** gated by a new feature: add it to the gate switch and add a `featureVersions` entry pinning it to the introducing version.
- **Deprecate** an item: add a `DeprecationInfo` entry (Since/RemoveIn/Replacement) to the capability's `Deprecated` map. It keeps working and logs a notice per load.
- **Remove** a deprecated item: only in the `RemoveIn` major, after the window. Bump the capability's major and regenerate the golden.
- **Migrate a legacy plugin**: replace `minCoreVersion` with a `requires{}` block. `minCoreVersion < 1.0.0` is rejected (built against the pre-1.0 API); `>= 1.0.0` is migrated with a deprecation warning.

## Rejected alternatives

- **Single global API semver.** The original problem — an unrelated bump rejects compatible plugins.
- **Per-capability versions without an envelope.** Leaves the wire/protocol contract unversioned; nothing to freeze for the release.
- **Free-form semver range strings (`^`, `~`, `>=`).** Ambiguous and a footgun; we use a structured `{min, features}` object instead.

## Consequences

- Plugin authors get a frozen `pluginApi 1.0` plus independently versioned capabilities they can depend on precisely.
- The host can evolve one capability without touching others, and can add methods behind feature versions without exposing them to older plugins.
- At the 1.0 freeze every capability is `1.0.0` and every method is baseline, so the feature-version gate has no current denials — it is the forward-looking mechanism that makes additive evolution safe.
