package plugin

import (
	"fmt"
	"strconv"
	"strings"
)

// Semver is a parsed MAJOR.MINOR.PATCH version with an optional pre-release suffix.
//
// It is the single parsing/comparison primitive for the plugin API versioning scheme
// (ADR-012). Both the protocol envelope (pluginApi) and every capability version are
// expressed and compared as Semver values.
type Semver struct {
	Major int
	Minor int
	Patch int
	// Pre is the pre-release suffix without the leading '-', e.g. "dev" or "rc1".
	// Empty for a released version. Pre-release ordering is deliberately NOT modelled:
	// for compatibility a host pre-release is treated as ">= the same release" (a dev
	// build of 1.1.0 satisfies a requirement of 1.0.0). See Satisfies and ADR-012 edge #7.
	Pre string
}

// String renders the version back to canonical form (with the pre-release suffix if any).
func (v Semver) String() string {
	base := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		return base + "-" + v.Pre
	}
	return base
}

// HasPre reports whether the version carries a pre-release suffix.
func (v Semver) HasPre() bool { return v.Pre != "" }

// ParseSemver parses a strict MAJOR.MINOR.PATCH[-pre] version.
//
// The grammar is intentionally strict to keep the plugin contract unambiguous: all three
// numeric components are required, no "v" prefix is accepted, and no leading zeros are
// allowed beyond a bare "0". A pre-release suffix is permitted here (host versions may be
// dev builds); callers that forbid it — e.g. requirement declarations — reject Pre
// themselves. Anything malformed returns an error rather than being silently coerced.
func ParseSemver(s string) (Semver, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return Semver{}, fmt.Errorf("empty version")
	}

	core := raw
	pre := ""
	if i := strings.IndexByte(raw, '-'); i >= 0 {
		core, pre = raw[:i], raw[i+1:]
		if pre == "" {
			return Semver{}, fmt.Errorf("invalid version %q: empty pre-release", raw)
		}
	}

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Semver{}, fmt.Errorf("invalid version %q: want MAJOR.MINOR.PATCH", raw)
	}

	nums := make([]int, 3)
	for i, p := range parts {
		n, err := parseVersionComponent(p)
		if err != nil {
			return Semver{}, fmt.Errorf("invalid version %q: %w", raw, err)
		}
		nums[i] = n
	}

	return Semver{Major: nums[0], Minor: nums[1], Patch: nums[2], Pre: pre}, nil
}

// parseVersionComponent parses one numeric component, rejecting empty strings, signs,
// non-digits, and leading zeros (which mask typos like "01").
func parseVersionComponent(p string) (int, error) {
	if p == "" {
		return 0, fmt.Errorf("empty numeric component")
	}
	if len(p) > 1 && p[0] == '0' {
		return 0, fmt.Errorf("leading zero in %q", p)
	}
	for _, r := range p {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("non-numeric component %q", p)
		}
	}
	return strconv.Atoi(p)
}

// Satisfies reports whether a host providing `have` satisfies a requirement of `want`.
//
// The rule is caret-like and identical for the envelope and every capability: the major
// versions must match (a different major is a breaking change) and the host minor must be
// at least the required minor (minor bumps are additive-only, ADR-012). Patch never gates —
// a patch is a non-contractual fix. Pre-release suffixes are ignored, so a host dev build
// satisfies a released requirement (edge #7).
func Satisfies(have, want Semver) bool {
	if have.Major != want.Major {
		return false
	}
	return have.Minor >= want.Minor
}
