package plugin_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func bundleFormatManifest(declared string) domainplugin.Manifest {
	return domainplugin.Manifest{
		ID:           "com.example.echo",
		Name:         "Echo",
		Version:      "1.0.0",
		BundleFormat: declared,
		Engine: domainplugin.EngineConfig{
			Type:  domainplugin.EngineGoBinary,
			Entry: "echo.exe",
		},
	}
}

func TestBundleFormatAbsentResolvesToBaseline(t *testing.T) {
	m := bundleFormatManifest("")
	if err := m.Validate(); err != nil {
		t.Fatalf("a manifest without bundleFormat must stay valid: %v", err)
	}
	if got := domainplugin.EffectiveBundleFormat(""); got != "1.0.0" {
		t.Errorf("EffectiveBundleFormat(\"\") = %q, want 1.0.0; every bundle published before the field existed was built against the first format", got)
	}
	if report := domainplugin.CheckBundleFormat("", nil); report != nil {
		t.Errorf("an undeclared bundleFormat must be compatible, got %v", report)
	}
}

func TestBundleFormatRejectsMalformedAndPrerelease(t *testing.T) {
	for _, declared := range []string{"1.0", "v1.0.0", "1.0.0.0", "one.oh.oh", "1.1.0-rc.1"} {
		m := bundleFormatManifest(declared)
		err := m.Validate()
		if err == nil {
			t.Errorf("bundleFormat %q was accepted; the packaging axis must be strict semver with no pre-release", declared)
			continue
		}
		if !errors.Is(err, domainplugin.ErrInvalidManifest) {
			t.Errorf("bundleFormat %q: got %v, want ErrInvalidManifest", declared, err)
		}
	}
}

func TestBundleFormatRefusesAFormatThisBuildCannotRead(t *testing.T) {
	for _, declared := range []string{"2.0.0", "1.1.0"} {
		report := domainplugin.CheckBundleFormat(declared, nil)
		if report == nil {
			t.Fatalf("bundleFormat %q must be refused: host reads %s, and opening a newer layout means verifying less than the publisher signed", declared, domainplugin.CurrentBundleFormat)
		}
		if !errors.Is(report, domainplugin.ErrIncompatibleAPI) {
			t.Errorf("bundleFormat %q: report does not match ErrIncompatibleAPI", declared)
		}
		if len(report.Items) != 1 || report.Items[0].Axis != "bundleFormat" {
			t.Errorf("bundleFormat %q: want one item on axis bundleFormat, got %+v", declared, report.Items)
		}
	}
}

func TestBundleFormatKeepsExistingReportItems(t *testing.T) {
	report := &domainplugin.IncompatibilityReport{}
	report.Items = append(report.Items, domainplugin.Incompatibility{Axis: "vault", Kind: domainplugin.IncompatVersion, Detail: "seeded"})

	got := domainplugin.CheckBundleFormat("2.0.0", report)
	if got == nil || len(got.Items) != 2 {
		t.Fatalf("packaging must fold into the existing report so one install preview lists every reason, got %+v", got)
	}
	if got.Items[0].Axis != "vault" {
		t.Errorf("existing items must survive, first item is %+v", got.Items[0])
	}
}

// A manifest without the field must serialise byte-identically to how it did before the field
// existed. If it does not, every signature issued against a published bundle stops verifying —
// the signing payload canonicalises the whole manifest, so one extra key breaks all of them.
func TestBundleFormatAbsentDoesNotChangeTheSignedBytes(t *testing.T) {
	digest := strings.Repeat("a", 64)

	payload, err := domainplugin.ManifestSigningPayload(bundleFormatManifest(""), digest)
	if err != nil {
		t.Fatalf("signing payload: %v", err)
	}
	if bytes.Contains(payload, []byte("bundleFormat")) {
		t.Fatalf("an absent bundleFormat leaked into the signed bytes; the json tag has lost its omitempty:\n%s", payload)
	}

	declared, err := domainplugin.ManifestSigningPayload(bundleFormatManifest("1.0.0"), digest)
	if err != nil {
		t.Fatalf("signing payload: %v", err)
	}
	if !bytes.Contains(declared, []byte("bundleFormat")) {
		t.Fatal("a declared bundleFormat must be covered by the signature, otherwise it can be edited after signing")
	}
}
