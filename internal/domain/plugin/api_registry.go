package plugin

import "slices"

// PluginAPIVersion is the frozen version of the plugin protocol ENVELOPE (ADR-012): the
// wire framing, the JSON-RPC envelope, the initialize handshake shape, the lifecycle
// methods, and the error-code space. It is the one contract every plugin depends on
// regardless of which capabilities it uses, and it moves rarely and strictly by semver
// (major = breaking, minor = additive-only). It replaces the former, unenforced
// coreAPIVersion. Do NOT bump this without an ADR update and a golden-surface review.
const PluginAPIVersion = "1.0.0"

// CapabilityID is the stable identifier of a plugin capability — the CapabilitySet JSON key
// (e.g. "vault"). It is a distinct type so the capability vocabulary is defined once, in the
// Cap* constants below, and typos surface at compile time rather than as silent gate drift.
type CapabilityID string

// FeatureID is the stable identifier of a named point-feature within a capability (e.g.
// "getSecret"). Feature ids are scoped to their capability, so the same value may appear under
// two capabilities (e.g. "dial" under both network and tunnel); always pair a FeatureID with its
// CapabilityID. Defined once in the Feat* constants.
type FeatureID string

// Capability identifiers — the complete host vocabulary. Every capability the host provides has
// exactly one constant here, and it must match a CapabilitySet grant field (see
// GrantedCapabilityNames) and a hostRegistry entry.
const (
	CapNetwork    CapabilityID = "network"
	CapFilesystem CapabilityID = "filesystem"
	CapEvents     CapabilityID = "events"
	CapVault      CapabilityID = "vault"
	CapSession    CapabilityID = "session"
	CapAuth       CapabilityID = "auth"
	CapTunnel     CapabilityID = "tunnel"
	CapChannel    CapabilityID = "channel"
)

// Feature identifiers, grouped by capability. Named Feat<Capability><Feature> because feature ids
// are only unique within a capability (e.g. FeatNetworkDial and FeatTunnelDial share the value
// "dial").
const (
	FeatNetworkDial FeatureID = "dial"

	FeatFilesystemRead  FeatureID = "read"
	FeatFilesystemWrite FeatureID = "write"

	FeatEventsPublish   FeatureID = "publish"
	FeatEventsSubscribe FeatureID = "subscribe"

	FeatVaultGetConnection FeatureID = "getConnection"
	FeatVaultGetSecret     FeatureID = "getSecret"

	FeatSessionEmbed            FeatureID = "embed"
	FeatSessionLocalEmbedServer FeatureID = "localEmbedServer"
	FeatSessionTerminal         FeatureID = "terminal"
	FeatSessionTunnel           FeatureID = "tunnel"
	FeatSessionUpdateState      FeatureID = "updateState"

	FeatAuthProvider FeatureID = "provider"

	FeatTunnelBind FeatureID = "bind"
	FeatTunnelDial FeatureID = "dial"

	FeatChannelOpen FeatureID = "open"
)

// DeprecationInfo records that a capability or one of its features is on its way out.
// A deprecated item keeps working (it still validates and negotiates) until it is removed
// in the RemoveIn major, after the deprecation window; usage emits a warning so authors
// migrate ahead of time.
type DeprecationInfo struct {
	// Since is the version in which the item was first marked deprecated.
	Since string
	// RemoveIn is the version in which the item is scheduled to be removed (always a major).
	RemoveIn string
	// Replacement names the feature/capability to migrate to, if any.
	Replacement string
}

// CapabilityDescriptor is the host's declared contract for one capability: its independent
// semver version and the set of named feature flags it currently offers. Each capability
// evolves on its own timeline, so changing one never forces a version bump on another.
type CapabilityDescriptor struct {
	// Version is this capability's own semver. Bumped (minor) when features are added.
	Version string
	// Features are the named point-features offered inside this capability. This list is a
	// deliberate, hand-authored contract — it is NOT derived from the gate, so that adding
	// an RPC method never silently changes the advertised surface.
	Features []FeatureID
	// Deprecated maps a feature id (or "" for the whole capability) to its deprecation record.
	// Absent when nothing is deprecated.
	Deprecated map[FeatureID]DeprecationInfo `json:",omitempty"`
}

// Registry maps a capability id to its descriptor. It is the single source of truth for what the
// host provides.
type Registry map[CapabilityID]CapabilityDescriptor

// hostRegistry is the authoritative host contract at this build. Every capability starts at
// 1.0.0 for the release freeze with an explicit feature list mirroring the gate's method
// map. Kept unexported; callers get an independent copy via HostRegistry so it cannot be
// mutated in place.
var hostRegistry = Registry{
	CapNetwork:    {Version: "1.0.0", Features: []FeatureID{FeatNetworkDial}},
	CapFilesystem: {Version: "1.0.0", Features: []FeatureID{FeatFilesystemRead, FeatFilesystemWrite}},
	CapEvents:     {Version: "1.0.0", Features: []FeatureID{FeatEventsPublish, FeatEventsSubscribe}},
	CapVault:      {Version: "1.0.0", Features: []FeatureID{FeatVaultGetConnection, FeatVaultGetSecret}},
	CapSession:    {Version: "1.0.0", Features: []FeatureID{FeatSessionEmbed, FeatSessionLocalEmbedServer, FeatSessionTerminal, FeatSessionTunnel, FeatSessionUpdateState}},
	CapAuth:       {Version: "1.0.0", Features: []FeatureID{FeatAuthProvider}},
	CapTunnel:     {Version: "1.0.0", Features: []FeatureID{FeatTunnelBind, FeatTunnelDial}},
	CapChannel:    {Version: "1.0.0", Features: []FeatureID{FeatChannelOpen}},
}

// HostRegistry returns an independent copy of the host's capability contract, safe for the
// caller to read or retain without affecting the source of truth.
func HostRegistry() Registry {
	out := make(Registry, len(hostRegistry))
	for id, d := range hostRegistry {
		out[id] = copyDescriptor(d)
	}
	return out
}

func copyDescriptor(d CapabilityDescriptor) CapabilityDescriptor {
	features := append([]FeatureID(nil), d.Features...)
	var dep map[FeatureID]DeprecationInfo
	if len(d.Deprecated) > 0 {
		dep = make(map[FeatureID]DeprecationInfo, len(d.Deprecated))
		for k, v := range d.Deprecated {
			dep[k] = v
		}
	}
	return CapabilityDescriptor{Version: d.Version, Features: features, Deprecated: dep}
}

// Has reports whether the registry provides the given capability.
func (r Registry) Has(capability CapabilityID) bool {
	_, ok := r[capability]
	return ok
}

// HasFeature reports whether the given capability offers the given feature flag.
func (r Registry) HasFeature(capability CapabilityID, feature FeatureID) bool {
	d, ok := r[capability]
	if !ok {
		return false
	}
	return slices.Contains(d.Features, feature)
}

// CapabilityVersion returns the parsed version of the given capability.
func (r Registry) CapabilityVersion(capability CapabilityID) (Semver, bool) {
	d, ok := r[capability]
	if !ok {
		return Semver{}, false
	}
	v, err := ParseSemver(d.Version)
	if err != nil {
		return Semver{}, false
	}
	return v, true
}

// CapabilityInfo is the per-capability slice of the initialize descriptor sent to plugins.
type CapabilityInfo struct {
	Version  string      `json:"version"`
	Features []FeatureID `json:"features"`
}

// APIDescriptor is the versioning payload the host advertises to a plugin at initialize:
// the frozen envelope version plus, for every capability, its version and feature set. The
// plugin negotiates against this; the host — never the plugin's echo — is the authority.
type APIDescriptor struct {
	PluginAPI    string                          `json:"pluginApi"`
	Capabilities map[CapabilityID]CapabilityInfo `json:"capabilities"`
}

// HostDescriptor builds the APIDescriptor advertised at initialize from the host registry.
func HostDescriptor() APIDescriptor {
	caps := make(map[CapabilityID]CapabilityInfo, len(hostRegistry))
	for id, d := range hostRegistry {
		caps[id] = CapabilityInfo{
			Version:  d.Version,
			Features: append([]FeatureID(nil), d.Features...),
		}
	}
	return APIDescriptor{PluginAPI: PluginAPIVersion, Capabilities: caps}
}
