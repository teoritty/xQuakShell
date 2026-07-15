package plugin

import (
	"fmt"
	"strings"
)

// HostCoreVersion is the core (backend engine) version. Post-freeze it is purely
// informational — reported to plugins at initialize and rendered in the About panel — and
// only the deprecated minCoreVersion legacy path still gates on it. Plugins gate on the
// pluginApi envelope + capability versions instead (ADR-012), never on this value.
const HostCoreVersion = "1.0.0"

// CompatibleWithCore reports whether the host satisfies manifest minCoreVersion.
func (m *Manifest) CompatibleWithCore(coreVersion string) error {
	min := strings.TrimSpace(m.MinCoreVersion)
	if min == "" {
		return nil
	}
	if !CoreVersionAtLeast(coreVersion, min) {
		return fmt.Errorf("%w: requires core %s, running %s", ErrIncompatibleCore, min, coreVersion)
	}
	return nil
}

// ValidateMinCoreVersion rejects malformed minCoreVersion declarations.
func ValidateMinCoreVersion(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	if _, err := ParseSemver(v); err != nil {
		return fmt.Errorf("%w: invalid minCoreVersion %q", ErrInvalidManifest, v)
	}
	return nil
}

// CoreVersionAtLeast reports whether actual >= minimum by full major.minor.patch order.
//
// This is the LEGACY minCoreVersion comparison and, unlike Satisfies, it orders across
// majors (a higher major is "at least" a lower one) because that is how minCoreVersion
// historically behaved. New code uses Satisfies; this remains only for the deprecated
// minCoreVersion shim. Pre-release suffixes are ignored.
func CoreVersionAtLeast(actual, minimum string) bool {
	a, aerr := ParseSemver(actual)
	m, merr := ParseSemver(minimum)
	if aerr != nil || merr != nil {
		return false
	}
	if a.Major != m.Major {
		return a.Major > m.Major
	}
	if a.Minor != m.Minor {
		return a.Minor > m.Minor
	}
	return a.Patch >= m.Patch
}
