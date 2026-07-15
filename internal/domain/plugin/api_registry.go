package plugin

// PluginAPIVersion is the frozen version of the plugin protocol ENVELOPE (ADR-012): the
// wire framing, the JSON-RPC envelope, the initialize handshake shape, the lifecycle
// methods, and the error-code space. It is the one contract every plugin depends on
// regardless of which capabilities it uses, and it moves rarely and strictly by semver
// (major = breaking, minor = additive-only). It replaces the former, unenforced
// coreAPIVersion. Do NOT bump this without an ADR update and a golden-surface review.
const PluginAPIVersion = "1.0.0"

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
	Features []string
	// Deprecated maps a feature name (or "" for the whole capability) to its deprecation
	// record. Absent when nothing is deprecated.
	Deprecated map[string]DeprecationInfo `json:",omitempty"`
}

// Registry maps a capability name (the CapabilitySet JSON key, e.g. "vault") to its
// descriptor. It is the single source of truth for what the host provides.
type Registry map[string]CapabilityDescriptor

// hostRegistry is the authoritative host contract at this build. Every capability starts at
// 1.0.0 for the release freeze with an explicit feature list mirroring the gate's method
// map. Kept unexported; callers get an independent copy via HostRegistry so it cannot be
// mutated in place.
var hostRegistry = Registry{
	"network":    {Version: "1.0.0", Features: []string{"dial"}},
	"filesystem": {Version: "1.0.0", Features: []string{"read", "write"}},
	"events":     {Version: "1.0.0", Features: []string{"publish", "subscribe"}},
	"vault":      {Version: "1.0.0", Features: []string{"getConnection", "getSecret"}},
	"session":    {Version: "1.0.0", Features: []string{"embed", "localEmbedServer", "terminal", "tunnel", "updateState"}},
	"auth":       {Version: "1.0.0", Features: []string{"provider"}},
	"tunnel":     {Version: "1.0.0", Features: []string{"bind", "dial"}},
	"channel":    {Version: "1.0.0", Features: []string{"open"}},
}

// HostRegistry returns an independent copy of the host's capability contract, safe for the
// caller to read or retain without affecting the source of truth.
func HostRegistry() Registry {
	out := make(Registry, len(hostRegistry))
	for name, d := range hostRegistry {
		out[name] = copyDescriptor(d)
	}
	return out
}

func copyDescriptor(d CapabilityDescriptor) CapabilityDescriptor {
	features := append([]string(nil), d.Features...)
	var dep map[string]DeprecationInfo
	if len(d.Deprecated) > 0 {
		dep = make(map[string]DeprecationInfo, len(d.Deprecated))
		for k, v := range d.Deprecated {
			dep[k] = v
		}
	}
	return CapabilityDescriptor{Version: d.Version, Features: features, Deprecated: dep}
}

// Has reports whether the registry provides the named capability.
func (r Registry) Has(capability string) bool {
	_, ok := r[capability]
	return ok
}

// HasFeature reports whether the named capability offers the named feature flag.
func (r Registry) HasFeature(capability, feature string) bool {
	d, ok := r[capability]
	if !ok {
		return false
	}
	for _, f := range d.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// CapabilityVersion returns the parsed version of the named capability.
func (r Registry) CapabilityVersion(capability string) (Semver, bool) {
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
	Version  string   `json:"version"`
	Features []string `json:"features"`
}

// APIDescriptor is the versioning payload the host advertises to a plugin at initialize:
// the frozen envelope version plus, for every capability, its version and feature set. The
// plugin negotiates against this; the host — never the plugin's echo — is the authority.
type APIDescriptor struct {
	PluginAPI    string                    `json:"pluginApi"`
	Capabilities map[string]CapabilityInfo `json:"capabilities"`
}

// HostDescriptor builds the APIDescriptor advertised at initialize from the host registry.
func HostDescriptor() APIDescriptor {
	caps := make(map[string]CapabilityInfo, len(hostRegistry))
	for name, d := range hostRegistry {
		caps[name] = CapabilityInfo{
			Version:  d.Version,
			Features: append([]string(nil), d.Features...),
		}
	}
	return APIDescriptor{PluginAPI: PluginAPIVersion, Capabilities: caps}
}
