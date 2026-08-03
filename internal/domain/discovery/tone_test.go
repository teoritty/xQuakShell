package discovery_test

import (
	"testing"

	"xquakshell/internal/domain/discovery"
)

func TestValidToneAcceptsKnownValues(t *testing.T) {
	for _, tone := range []discovery.Tone{
		discovery.ToneOK, discovery.ToneWarn, discovery.ToneError,
		discovery.ToneBusy, discovery.ToneNeutral, discovery.ToneUnknown,
	} {
		if !discovery.ValidTone(tone) {
			t.Fatalf("expected %q to be a valid tone", tone)
		}
	}
}

func TestValidToneRejectsUnknownValue(t *testing.T) {
	if discovery.ValidTone("critical") {
		t.Fatal("expected unknown tone to be rejected")
	}
}

func TestValidColorAcceptsEmpty(t *testing.T) {
	if !discovery.ValidColor("") {
		t.Fatal("expected empty color (unset) to be valid")
	}
}

func TestValidColorAcceptsStrictHex(t *testing.T) {
	for _, c := range []string{"#000000", "#ffffff", "#AaBbCc", "#123456"} {
		if !discovery.ValidColor(c) {
			t.Fatalf("expected %q to be a valid color", c)
		}
	}
}

func TestValidColorRejectsMalformedValues(t *testing.T) {
	cases := []string{
		"000000",    // missing #
		"#abc",      // 3-digit shorthand
		"#1a2b3c4d", // 8-digit (with alpha)
		"#gggggg",   // non-hex characters
		"#12345",    // 5 digits
	}
	for _, c := range cases {
		if discovery.ValidColor(c) {
			t.Fatalf("expected %q to be rejected", c)
		}
	}
}
