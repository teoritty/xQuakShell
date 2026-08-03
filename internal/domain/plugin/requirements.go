package plugin

import (
	"fmt"
	"slices"
	"strings"
)

// RequirementSet is a plugin's declared dependency on the versioned API surface (ADR-012).
// It is the manifest "requires" block: the protocol envelope version the plugin targets and,
// per capability, the minimum version and named feature flags it actually uses. A plugin
// declares only what it depends on, so evolving an unrelated capability never rejects it.
type RequirementSet struct {
	// PluginAPI is the required protocol envelope version (MAJOR.MINOR.PATCH, no suffix).
	PluginAPI string `json:"pluginApi"`
	// Capabilities maps a capability id to its version/feature requirement. Optional; a
	// granted capability with no entry here gets an implicit baseline requirement.
	Capabilities map[CapabilityID]CapabilityRequirement `json:"capabilities,omitempty"`
}

// CapabilityRequirement is a plugin's dependency on one capability: the minimum version it
// needs and the specific feature flags it calls.
type CapabilityRequirement struct {
	// Min is the minimum acceptable capability version (MAJOR.MINOR.PATCH, no suffix).
	Min string `json:"min"`
	// Features are the named feature flags the plugin relies on within this capability.
	Features []FeatureID `json:"features,omitempty"`
}

// IncompatKind classifies why a requirement could not be satisfied by the host.
type IncompatKind string

const (
	// IncompatUnsupported: the host does not provide the capability at all (unknown name,
	// or a capability added in a newer host than this one). ADR-012 edge #4.
	IncompatUnsupported IncompatKind = "unsupported"
	// IncompatVersion: the host provides the axis but at an incompatible version (wrong
	// major, or minor too low). ADR-012 edges #6.
	IncompatVersion IncompatKind = "version"
	// IncompatMissingFeature: the host provides the capability but not a required feature.
	IncompatMissingFeature IncompatKind = "missing-feature"
)

// Incompatibility is a single unmet requirement.
type Incompatibility struct {
	// Axis is "pluginApi" or a capability name.
	Axis string
	// Kind classifies the mismatch.
	Kind IncompatKind
	// Detail is a human-readable explanation (what was required vs what the host offers).
	Detail string
}

// IncompatibilityReport aggregates every unmet requirement for one plugin. It doubles as an
// error so it can be returned directly from validation; errors.Is reports ErrIncompatibleAPI
// and/or ErrMissingFeature depending on which kinds are present.
type IncompatibilityReport struct {
	Items []Incompatibility
}

// Empty reports whether the report has no incompatibilities.
func (r *IncompatibilityReport) Empty() bool { return r == nil || len(r.Items) == 0 }

// Error renders all incompatibilities in a stable, human-readable order.
func (r *IncompatibilityReport) Error() string {
	if r.Empty() {
		return "plugin API compatible"
	}
	parts := make([]string, 0, len(r.Items))
	for _, it := range r.Items {
		parts = append(parts, fmt.Sprintf("%s: %s", it.Axis, it.Detail))
	}
	return "incompatible plugin API: " + strings.Join(parts, "; ")
}

// Is lets errors.Is match the underlying sentinels. A report carrying any version/unsupported
// item matches ErrIncompatibleAPI; any missing-feature item matches ErrMissingFeature.
func (r *IncompatibilityReport) Is(target error) bool {
	if r.Empty() {
		return false
	}
	for _, it := range r.Items {
		switch {
		case target == ErrMissingFeature && it.Kind == IncompatMissingFeature:
			return true
		case target == ErrIncompatibleAPI && (it.Kind == IncompatVersion || it.Kind == IncompatUnsupported):
			return true
		}
	}
	return false
}

func (r *IncompatibilityReport) add(axis string, kind IncompatKind, detail string) {
	r.Items = append(r.Items, Incompatibility{Axis: axis, Kind: kind, Detail: detail})
}

// GrantedCapabilityNames returns the ids of every capability the plugin declared in its
// CapabilitySet (a non-nil grant), sorted for determinism. This is the set a plugin is permitted
// to require versions/features of, and the single mapping from grant fields to capability ids.
func (c CapabilitySet) GrantedCapabilityNames() []CapabilityID {
	var names []CapabilityID
	if c.Network != nil {
		names = append(names, CapNetwork)
	}
	if c.FS != nil {
		names = append(names, CapFilesystem)
	}
	if c.Events != nil {
		names = append(names, CapEvents)
	}
	if c.Vault != nil {
		names = append(names, CapVault)
	}
	if c.Session != nil {
		names = append(names, CapSession)
	}
	if c.Auth != nil {
		names = append(names, CapAuth)
	}
	if c.Tunnel != nil {
		names = append(names, CapTunnel)
	}
	if c.Channel != nil {
		names = append(names, CapChannel)
	}
	if c.Discovery != nil {
		names = append(names, CapDiscovery)
	}
	slices.Sort(names)
	return names
}

// grantsCapability reports whether the plugin declared the given capability.
func (c CapabilitySet) grantsCapability(name CapabilityID) bool {
	return slices.Contains(c.GrantedCapabilityNames(), name)
}
