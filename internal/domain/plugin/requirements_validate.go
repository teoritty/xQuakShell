package plugin

import (
	"fmt"
	"strings"
)

// ValidateRequirements checks the AUTHOR-WRITTEN requires{} block for well-formedness,
// independent of any particular host: strict semver with no pre-release suffix (a plugin may
// not depend on an unstable API), and that every required capability is one the plugin
// actually granted in its CapabilitySet (edge #2 — you cannot require a version of a surface
// you have no permission to call). Host-relative checks live in CheckAgainstHost.
//
// A nil requires{} block is valid here; the legacy migration in EffectiveRequirements handles
// the absent case.
func (m *Manifest) ValidateRequirements() error {
	if m.Requires == nil {
		return nil
	}

	if err := validateRequirementVersion("requires.pluginApi", m.Requires.PluginAPI); err != nil {
		return err
	}

	for name, req := range m.Requires.Capabilities {
		field := "requires.capabilities." + name
		if err := validateRequirementVersion(field+".min", req.Min); err != nil {
			return err
		}
		if !m.Capabilities.grantsCapability(name) {
			return fmt.Errorf("%w: %s requires capability %q that is not granted in capabilities{}", ErrInvalidManifest, field, name)
		}
		for _, f := range req.Features {
			if strings.TrimSpace(f) == "" {
				return fmt.Errorf("%w: %s.features contains an empty feature name", ErrInvalidManifest, field)
			}
		}
	}
	return nil
}

// validateRequirementVersion enforces the strict requirement grammar: a parseable
// MAJOR.MINOR.PATCH with no pre-release suffix.
func validateRequirementVersion(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalidManifest, field)
	}
	v, err := ParseSemver(value)
	if err != nil {
		return fmt.Errorf("%w: %s %q is not valid semver", ErrInvalidManifest, field, value)
	}
	if v.HasPre() {
		return fmt.Errorf("%w: %s %q must not carry a pre-release suffix (requirements may not depend on unstable versions)", ErrInvalidManifest, field, value)
	}
	return nil
}

// CheckAgainstHost verifies the (already effective) requirement set against a host registry
// and returns a report of every unmet requirement, or nil when fully compatible. This is the
// single compatibility oracle used both at install time (against HostRegistry) and at the
// runtime handshake (against the live registry) — the host descriptor is always the authority.
func (rs RequirementSet) CheckAgainstHost(reg Registry) *IncompatibilityReport {
	report := &IncompatibilityReport{}

	rs.checkEnvelope(report)
	for _, name := range rs.sortedCapabilityNames() {
		rs.checkCapability(name, rs.Capabilities[name], reg, report)
	}

	if report.Empty() {
		return nil
	}
	return report
}

func (rs RequirementSet) checkEnvelope(report *IncompatibilityReport) {
	want, err := ParseSemver(rs.PluginAPI)
	if err != nil {
		report.add("pluginApi", IncompatVersion, fmt.Sprintf("invalid required version %q", rs.PluginAPI))
		return
	}
	have, err := ParseSemver(PluginAPIVersion)
	if err != nil {
		report.add("pluginApi", IncompatVersion, "host pluginApi version is malformed")
		return
	}
	if !Satisfies(have, want) {
		report.add("pluginApi", IncompatVersion, fmt.Sprintf("requires pluginApi %s, host provides %s", want, have))
	}
}

func (rs RequirementSet) checkCapability(name string, req CapabilityRequirement, reg Registry, report *IncompatibilityReport) {
	haveVer, ok := reg.CapabilityVersion(name)
	if !ok {
		// Unknown or unsupported capability (typo, or one added in a newer host). Edge #4.
		report.add(name, IncompatUnsupported, fmt.Sprintf("capability %q is not provided by this host", name))
		return
	}
	want, err := ParseSemver(req.Min)
	if err != nil {
		report.add(name, IncompatVersion, fmt.Sprintf("invalid required version %q", req.Min))
		return
	}
	if !Satisfies(haveVer, want) {
		report.add(name, IncompatVersion, fmt.Sprintf("requires %s %s, host provides %s", name, want, haveVer))
	}
	for _, f := range req.Features {
		if !reg.HasFeature(name, f) {
			report.add(name, IncompatMissingFeature, fmt.Sprintf("requires feature %q not offered by %s", f, name))
		}
	}
}
