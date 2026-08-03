package plugin

import (
	"fmt"
	"slices"
)

// DeprecationNotices reports every deprecated capability or feature the negotiated contract depends
// on, as human-readable warnings. Deprecated items still work — they are removed only after the
// deprecation window in a future major (ADR-012) — so these are advisories the host logs once per
// plugin load to nudge authors to migrate, never a reason to reject. It reads only the negotiated
// descriptor, keeping the runtime single-source (see NegotiatedDescriptor).
func (nd NegotiatedDescriptor) DeprecationNotices(reg Registry) []string {
	var notices []string
	names := make([]CapabilityID, 0, len(nd.Capabilities))
	for name := range nd.Capabilities {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		desc, ok := reg.Descriptor(name)
		if !ok || len(desc.Deprecated) == 0 {
			continue
		}
		// Whole-capability deprecation (keyed by the empty feature id).
		if info, ok := desc.Deprecated[FeatureID("")]; ok {
			notices = append(notices, formatDeprecation(name, "", info))
		}
		// Per-feature deprecation for features this plugin actually negotiated.
		for _, f := range nd.Capabilities[name].Features {
			if info, ok := desc.Deprecated[f]; ok {
				notices = append(notices, formatDeprecation(name, f, info))
			}
		}
	}
	return notices
}

func formatDeprecation(capability CapabilityID, feature FeatureID, info DeprecationInfo) string {
	target := string(capability)
	if feature != "" {
		target = string(capability) + "." + string(feature)
	}
	msg := fmt.Sprintf("%s is deprecated since %s and will be removed in %s", target, info.Since, info.RemoveIn)
	if info.Replacement != "" {
		msg += "; use " + info.Replacement + " instead"
	}
	return msg
}
