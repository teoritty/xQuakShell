package plugin

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// HostCoreVersion is the semver reported to plugins at initialize and used for minCoreVersion checks.
const HostCoreVersion = "0.2.0-dev"

var coreVersionPrefix = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)

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
	if _, _, _, ok := parseCoreVersion(v); !ok {
		return fmt.Errorf("%w: invalid minCoreVersion %q", ErrInvalidManifest, v)
	}
	return nil
}

// CoreVersionAtLeast reports whether actual >= minimum (semver prefix compare).
func CoreVersionAtLeast(actual, minimum string) bool {
	am, ai, ap, aok := parseCoreVersion(actual)
	mm, mi, mp, mok := parseCoreVersion(minimum)
	if !aok || !mok {
		return false
	}
	if am != mm {
		return am > mm
	}
	if ai != mi {
		return ai > mi
	}
	return ap >= mp
}

func parseCoreVersion(v string) (major, minor, patch int, ok bool) {
	v = strings.TrimSpace(v)
	m := coreVersionPrefix.FindStringSubmatch(v)
	if len(m) != 4 {
		return 0, 0, 0, false
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, 0, 0, false
	}
	minor, err = strconv.Atoi(m[2])
	if err != nil {
		return 0, 0, 0, false
	}
	patch, err = strconv.Atoi(m[3])
	if err != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}
