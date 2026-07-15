package plugin

import (
	"fmt"
	"sort"
)

// EffectiveRequirements resolves a manifest into the concrete RequirementSet the host checks
// against, applying the legacy minCoreVersion migration and filling implicit capability
// baselines. It returns any deprecation/advisory warnings (for the caller to log) and an
// error only for a hard rejection (a plugin built against the pre-1.0 API — ADR-012 edge #8).
//
// Resolution rules (ADR-012 edges #1, #8, #9, #10):
//   - requires{} present        → authoritative; minCoreVersion, if also set, is ignored (warn).
//   - only minCoreVersion, <1.0 → reject: built against the unstable 0.x API.
//   - only minCoreVersion, ≥1.0 → migrate to requires{pluginApi: minCoreVersion} (deprecation warn).
//   - neither                   → default to the current envelope baseline (advisory warn).
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

	caps := map[string]CapabilityRequirement{}
	if m.Requires != nil {
		for name, req := range m.Requires.Capabilities {
			caps[name] = req
		}
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
// from the manifest's requires/minCoreVersion combination.
func resolvePluginAPI(m *Manifest) (string, []string, error) {
	var warnings []string

	if m.Requires != nil {
		// requires{} is authoritative (edge #10).
		if m.MinCoreVersion != "" {
			warnings = append(warnings, "minCoreVersion is ignored because requires{} is declared; remove minCoreVersion")
		}
		return m.Requires.PluginAPI, warnings, nil
	}

	if m.MinCoreVersion == "" {
		// Nothing declared: default to the current envelope baseline (edge #1).
		warnings = append(warnings, fmt.Sprintf("no requires{} declared; defaulting to pluginApi %s — declare requires{} explicitly", PluginAPIVersion))
		return PluginAPIVersion, warnings, nil
	}

	// Legacy minCoreVersion only.
	v, err := ParseSemver(m.MinCoreVersion)
	if err != nil {
		return "", warnings, fmt.Errorf("%w: invalid minCoreVersion %q", ErrInvalidManifest, m.MinCoreVersion)
	}
	if v.Major < 1 {
		// Built against the unstable pre-1.0 API — reject even though 1.0.0 >= 0.x numerically
		// passes the legacy comparison. We will not load a 0.x plugin into a frozen 1.0 host
		// (edge #8).
		return "", warnings, fmt.Errorf("%w: plugin targets pre-1.0 API (minCoreVersion %q); rebuild against pluginApi %s", ErrIncompatibleAPI, m.MinCoreVersion, PluginAPIVersion)
	}
	warnings = append(warnings, "minCoreVersion is deprecated; migrate to a requires{} block")
	return m.MinCoreVersion, warnings, nil
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
func (rs RequirementSet) sortedCapabilityNames() []string {
	names := make([]string, 0, len(rs.Capabilities))
	for name := range rs.Capabilities {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
