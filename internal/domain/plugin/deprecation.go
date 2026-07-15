package plugin

import "fmt"

// DeprecationNotices reports every deprecated capability or feature the requirement set depends on,
// as human-readable warnings. Deprecated items still work — they are removed only after the
// deprecation window in a future major (ADR-012) — so these are advisories the host logs once per
// plugin load to nudge authors to migrate, never a reason to reject.
func (rs RequirementSet) DeprecationNotices(reg Registry) []string {
	var notices []string
	for _, name := range rs.sortedCapabilityNames() {
		desc, ok := reg[name]
		if !ok || len(desc.Deprecated) == 0 {
			continue
		}
		// Whole-capability deprecation (keyed by "").
		if info, ok := desc.Deprecated[""]; ok {
			notices = append(notices, formatDeprecation(name, "", info))
		}
		// Per-feature deprecation for features this plugin actually requires.
		for _, f := range rs.Capabilities[name].Features {
			if info, ok := desc.Deprecated[f]; ok {
				notices = append(notices, formatDeprecation(name, f, info))
			}
		}
	}
	return notices
}

func formatDeprecation(capability, feature string, info DeprecationInfo) string {
	target := capability
	if feature != "" {
		target = capability + "." + feature
	}
	msg := fmt.Sprintf("%s is deprecated since %s and will be removed in %s", target, info.Since, info.RemoveIn)
	if info.Replacement != "" {
		msg += "; use " + info.Replacement + " instead"
	}
	return msg
}
