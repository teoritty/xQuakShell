package plugin

import "fmt"

// NegotiatedDescriptor is the single runtime version contract for one plugin process, produced by
// Negotiate at handshake time. Everything version-related — the capability gate's feature check,
// the initializer's logging — reads ONLY this, never the manifest, the registry, or a
// RequirementSet. That keeps exactly one source of truth for "what did this plugin negotiate?".
// Authorization (capability grants) is a separate axis and deliberately stays with the manifest.
type NegotiatedDescriptor struct {
	// PluginAPI is the negotiated protocol envelope version the plugin targets.
	PluginAPI Semver
	// Capabilities is the resolved per-capability contract: the version the plugin negotiated and
	// the feature flags it declared it uses.
	Capabilities map[CapabilityID]NegotiatedCapability
}

// NegotiatedCapability is one capability's slice of the runtime contract.
type NegotiatedCapability struct {
	Version  Semver
	Features []FeatureID
}

// Negotiate resolves a plugin against a host registry into the single runtime contract, or returns
// the structured incompatibility (it folds EffectiveRequirements + CheckAgainstHost). The returned
// warnings are migration/advisory notices for the caller to log. It fails closed: any
// unsatisfiable requirement, pre-1.0 legacy plugin, or malformed version yields an error and no
// descriptor, so a caller can refuse the plugin before doing any further work.
func Negotiate(m *Manifest, reg Registry) (NegotiatedDescriptor, []string, error) {
	eff, warnings, err := EffectiveRequirements(m)
	if err != nil {
		return NegotiatedDescriptor{}, warnings, err
	}
	if report := eff.CheckAgainstHost(reg); report != nil {
		return NegotiatedDescriptor{}, warnings, report
	}

	api, err := ParseSemver(eff.PluginAPI)
	if err != nil {
		return NegotiatedDescriptor{}, warnings, fmt.Errorf("%w: negotiated pluginApi %q is malformed", ErrIncompatibleAPI, eff.PluginAPI)
	}

	caps := make(map[CapabilityID]NegotiatedCapability, len(eff.Capabilities))
	for id, req := range eff.Capabilities {
		v, err := ParseSemver(req.Min)
		if err != nil {
			return NegotiatedDescriptor{}, warnings, fmt.Errorf("%w: negotiated %s version %q is malformed", ErrIncompatibleAPI, id, req.Min)
		}
		caps[id] = NegotiatedCapability{Version: v, Features: append([]FeatureID(nil), req.Features...)}
	}

	return NegotiatedDescriptor{PluginAPI: api, Capabilities: caps}, warnings, nil
}

// CapabilityVersion returns the negotiated version of a capability, and whether it was negotiated
// at all. Used by the gate to decide whether an above-baseline feature method is in reach.
func (nd NegotiatedDescriptor) CapabilityVersion(id CapabilityID) (Semver, bool) {
	c, ok := nd.Capabilities[id]
	return c.Version, ok
}
