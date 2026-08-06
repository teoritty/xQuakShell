package plugin

import (
	"maps"
	"slices"
)

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
	CapDiscovery  CapabilityID = "discovery"
	CapUI         CapabilityID = "ui"
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

	// FeatDiscoveryPublish is the plugin->host discovery.publish REQUEST (ADR-014). It is a request
	// rather than a notification despite having no answer worth having: it names a sessionId, so it
	// is one of the plugin->host calls the host can refuse — by the gate with -32001, or by the IDOR
	// check — and a notification has no channel to carry a refusal back.
	FeatDiscoveryPublish FeatureID = "publish"
	// FeatDiscoveryInvoke is the host->plugin discovery.invokeAction request (ADR-014).
	FeatDiscoveryInvoke FeatureID = "invoke"

	// UI features (ADR-015). The two surface kinds are separate features rather than one
	// "surfaces" flag because they are separate things a plugin can depend on: a log viewer with
	// search and export is not a terminal with a narrower API, and a plugin that requires one and
	// gets the other has nowhere to put its output.
	FeatUISurfaceTerminal FeatureID = "surfaceTerminal"
	FeatUISurfaceLog      FeatureID = "surfaceLog"
	FeatUIDialogs         FeatureID = "dialogs"
	FeatUINodeDetails     FeatureID = "nodeDetails"
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

// Registry is the immutable source of truth for the capabilities a host provides. It is an
// opaque value: its capability map is private and there are no mutators, so once built it cannot
// be changed and can be shared freely without defensive copying. Build one with NewRegistry;
// read it through the methods below.
type Registry struct {
	caps map[CapabilityID]CapabilityDescriptor
}

// NewRegistry builds an immutable Registry from a capability map, copying the input so later
// mutation of the caller's map cannot affect the registry. Used to build the host contract and
// synthetic registries in tests.
func NewRegistry(caps map[CapabilityID]CapabilityDescriptor) Registry {
	out := make(map[CapabilityID]CapabilityDescriptor, len(caps))
	for id, d := range caps {
		out[id] = copyDescriptor(d)
	}
	return Registry{caps: out}
}

// hostRegistry is the authoritative host contract at this build, built once and never mutated.
// Every capability starts at 1.0.0 for the release freeze with an explicit feature list mirroring
// the gate's method map.
var hostRegistry = NewRegistry(map[CapabilityID]CapabilityDescriptor{
	CapNetwork:    {Version: "1.0.0", Features: []FeatureID{FeatNetworkDial}},
	CapFilesystem: {Version: "1.0.0", Features: []FeatureID{FeatFilesystemRead, FeatFilesystemWrite}},
	CapEvents:     {Version: "1.0.0", Features: []FeatureID{FeatEventsPublish, FeatEventsSubscribe}},
	CapVault:      {Version: "1.0.0", Features: []FeatureID{FeatVaultGetConnection, FeatVaultGetSecret}},
	CapSession:    {Version: "1.0.0", Features: []FeatureID{FeatSessionEmbed, FeatSessionLocalEmbedServer, FeatSessionTerminal, FeatSessionTunnel, FeatSessionUpdateState}},
	CapAuth:       {Version: "1.0.0", Features: []FeatureID{FeatAuthProvider}},
	CapTunnel:     {Version: "1.0.0", Features: []FeatureID{FeatTunnelBind, FeatTunnelDial}},
	CapChannel:    {Version: "1.0.0", Features: []FeatureID{FeatChannelOpen}},
	CapDiscovery:  {Version: "1.0.0", Features: []FeatureID{FeatDiscoveryPublish, FeatDiscoveryInvoke}},
	CapUI:         {Version: "1.0.0", Features: []FeatureID{FeatUISurfaceTerminal, FeatUISurfaceLog, FeatUIDialogs, FeatUINodeDetails}},
})

// HostRegistry returns the shared immutable host contract. No copy is made — the type has no
// mutators, so sharing is safe.
func HostRegistry() Registry {
	return hostRegistry
}

func copyDescriptor(d CapabilityDescriptor) CapabilityDescriptor {
	features := append([]FeatureID(nil), d.Features...)
	var dep map[FeatureID]DeprecationInfo
	if len(d.Deprecated) > 0 {
		dep = maps.Clone(d.Deprecated)
	}
	return CapabilityDescriptor{Version: d.Version, Features: features, Deprecated: dep}
}

// Has reports whether the registry provides the given capability.
func (r Registry) Has(capability CapabilityID) bool {
	_, ok := r.caps[capability]
	return ok
}

// HasFeature reports whether the given capability offers the given feature flag.
func (r Registry) HasFeature(capability CapabilityID, feature FeatureID) bool {
	d, ok := r.caps[capability]
	if !ok {
		return false
	}
	return slices.Contains(d.Features, feature)
}

// CapabilityVersion returns the parsed version of the given capability.
func (r Registry) CapabilityVersion(capability CapabilityID) (Semver, bool) {
	d, ok := r.caps[capability]
	if !ok {
		return Semver{}, false
	}
	v, err := ParseSemver(d.Version)
	if err != nil {
		return Semver{}, false
	}
	return v, true
}

// Names returns every capability id in the registry, sorted for deterministic iteration.
func (r Registry) Names() []CapabilityID {
	names := make([]CapabilityID, 0, len(r.caps))
	for id := range r.caps {
		names = append(names, id)
	}
	slices.Sort(names)
	return names
}

// Descriptor returns a copy of the descriptor for a capability, so the immutable registry cannot
// be mutated through the returned value.
func (r Registry) Descriptor(capability CapabilityID) (CapabilityDescriptor, bool) {
	d, ok := r.caps[capability]
	if !ok {
		return CapabilityDescriptor{}, false
	}
	return copyDescriptor(d), true
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
	caps := make(map[CapabilityID]CapabilityInfo, len(hostRegistry.caps))
	for id, d := range hostRegistry.caps {
		caps[id] = CapabilityInfo{
			Version:  d.Version,
			Features: append([]FeatureID(nil), d.Features...),
		}
	}
	return APIDescriptor{PluginAPI: PluginAPIVersion, Capabilities: caps}
}
