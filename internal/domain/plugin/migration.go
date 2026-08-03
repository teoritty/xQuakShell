package plugin

import (
	"fmt"
	"maps"
	"slices"
)

// EffectiveRequirements resolves a manifest into the concrete RequirementSet the host checks
// against, filling implicit capability baselines. It returns any advisory warnings (for the
// caller to log) and an error for a hard rejection.
//
// Resolution rules (ADR-012 edge #1):
//   - requires{} present → authoritative.
//   - absent             → default to the current envelope baseline (advisory warn).
//
// Regardless of source, every capability the plugin grants but does not explicitly require
// gets an implicit requirement of the envelope-major baseline (e.g. 1.0.0), because declaring
// the grant means accepting that capability's baseline contract (edge #3).
func EffectiveRequirements(m *Manifest) (RequirementSet, []string, error) {
	var warnings []string

	pluginAPI, warns, err := resolvePluginAPI(m)
	warnings = append(warnings, warns...)
	if err != nil {
		return RequirementSet{}, warnings, err
	}

	baseline, err := majorBaseline(pluginAPI)
	if err != nil {
		return RequirementSet{}, warnings, err
	}

	caps := map[CapabilityID]CapabilityRequirement{}
	if m.Requires != nil {
		maps.Copy(caps, m.Requires.Capabilities)
	}
	// Fill implicit baselines for granted-but-unrequired capabilities (edge #3).
	for _, name := range m.Capabilities.GrantedCapabilityNames() {
		if _, ok := caps[name]; !ok {
			caps[name] = CapabilityRequirement{Min: baseline}
		}
	}

	return RequirementSet{PluginAPI: pluginAPI, Capabilities: caps}, warnings, nil
}

// resolvePluginAPI determines the effective pluginApi version string and any warnings/errors
// from the manifest's requires declaration.
func resolvePluginAPI(m *Manifest) (string, []string, error) {
	var warnings []string

	if m.Requires != nil {
		return m.Requires.PluginAPI, warnings, nil
	}

	if m.MinCoreVersion != "" {
		return "", warnings, fmt.Errorf("%w: minCoreVersion is not supported; declare a requires{} block", ErrInvalidManifest)
	}

	// Nothing declared: default to the current envelope baseline (edge #1).
	warnings = append(warnings, fmt.Sprintf("no requires{} declared; defaulting to pluginApi %s — declare requires{} explicitly", PluginAPIVersion))
	return PluginAPIVersion, warnings, nil
}

// majorBaseline returns the "<major>.0.0" baseline string for a version.
func majorBaseline(version string) (string, error) {
	v, err := ParseSemver(version)
	if err != nil {
		return "", fmt.Errorf("%w: invalid pluginApi %q", ErrInvalidManifest, version)
	}
	return fmt.Sprintf("%d.0.0", v.Major), nil
}

// sortedCapabilityNames returns the requirement's capability names in stable order (for
// deterministic reporting/serialisation).
func (rs RequirementSet) sortedCapabilityNames() []CapabilityID {
	names := make([]CapabilityID, 0, len(rs.Capabilities))
	for name := range rs.Capabilities {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
