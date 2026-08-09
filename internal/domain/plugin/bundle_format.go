package plugin

import (
	"fmt"
	"strings"
)

// CurrentBundleFormat is the packaging format this build can read: the archive layout of a
// .xqsp bundle (ADR-016) — which entries exist, what SHA256SUMS covers, and what the manifest
// signature binds to. It is a version axis of its own, independent of PluginAPIVersion, because
// packaging and protocol change for unrelated reasons: adding an entry to the archive says
// nothing about the wire contract, and vice versa.
//
// It moves strictly by semver, additive-only within a major, and only alongside an ADR update.
const CurrentBundleFormat = "1.0.0"

// baselineBundleFormat is what a manifest that declares no bundleFormat is taken to mean.
//
// It is a separate constant from CurrentBundleFormat and must never be redefined as "whatever
// the host currently is". Every bundle published before the field existed was built against the
// first format; if the baseline tracked the host, bumping the host to 1.1.0 would retroactively
// reinterpret those old manifests as claiming 1.1.0 and the compatibility check would wave
// through a bundle nobody ever built against that layout.
const baselineBundleFormat = "1.0.0"

// ValidateBundleFormat checks the AUTHOR-WRITTEN bundleFormat field for well-formedness only,
// independent of any particular host — the same split Validate/CheckHostCompatibility keeps for
// the requires{} block (ADR-012 §4), so a manifest this build cannot run can still be parsed and
// displayed. An absent field is valid and resolves to the baseline.
//
// A pre-release suffix is refused for the same reason requirements refuse one: a published
// bundle may not claim to be packaged against an unstable format.
func ValidateBundleFormat(declared string) error {
	if strings.TrimSpace(declared) == "" {
		return nil
	}
	v, err := ParseSemver(declared)
	if err != nil {
		return fmt.Errorf("%w: bundleFormat %q is not valid semver", ErrInvalidManifest, declared)
	}
	if v.HasPre() {
		return fmt.Errorf("%w: bundleFormat %q must not carry a pre-release suffix", ErrInvalidManifest, declared)
	}
	return nil
}

// EffectiveBundleFormat resolves the declared field to the format the bundle actually claims,
// substituting the baseline when the field is absent.
func EffectiveBundleFormat(declared string) string {
	if strings.TrimSpace(declared) == "" {
		return baselineBundleFormat
	}
	return strings.TrimSpace(declared)
}

// CheckBundleFormat folds the packaging-format check into an existing incompatibility report,
// returning the report to act on: nil in and compatible yields nil out, so the caller keeps the
// single "nil means runnable" convention CheckAgainstHost established.
//
// The rule is the one every other axis uses (Satisfies): the major must match, and the host
// minor must reach the declared one. A bundle packaged by a newer minor may carry entries this
// build would silently ignore, which for a signed archive means verifying less than the
// publisher signed — so it is refused rather than opened optimistically.
func CheckBundleFormat(declared string, report *IncompatibilityReport) *IncompatibilityReport {
	want, err := ParseSemver(EffectiveBundleFormat(declared))
	if err != nil {
		return addBundleFormatItem(report, fmt.Sprintf("invalid bundleFormat %q", declared))
	}
	have, err := ParseSemver(CurrentBundleFormat)
	if err != nil {
		return addBundleFormatItem(report, "host bundleFormat version is malformed")
	}
	if !Satisfies(have, want) {
		return addBundleFormatItem(report, fmt.Sprintf("packaged as bundleFormat %s, host reads %s", want, have))
	}
	return report
}

func addBundleFormatItem(report *IncompatibilityReport, detail string) *IncompatibilityReport {
	if report == nil {
		report = &IncompatibilityReport{}
	}
	report.add("bundleFormat", IncompatVersion, detail)
	return report
}
