package plugin_test

import (
	"errors"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestCoreVersionAtLeast(t *testing.T) {
	tests := []struct {
		actual, minimum string
		want            bool
	}{
		{"0.2.0-dev", "0.2.0", true},
		{"0.2.1", "0.2.0", true},
		{"0.1.9", "0.2.0", false},
		{"1.0.0", "0.9.9", true},
	}
	for _, tc := range tests {
		if got := domainplugin.CoreVersionAtLeast(tc.actual, tc.minimum); got != tc.want {
			t.Fatalf("CoreVersionAtLeast(%q, %q) = %v, want %v", tc.actual, tc.minimum, got, tc.want)
		}
	}
}

func TestManifestCompatibleWithCore(t *testing.T) {
	m := domainplugin.Manifest{MinCoreVersion: "0.3.0"}
	err := m.CompatibleWithCore("0.2.0-dev")
	if err == nil {
		t.Fatal("expected incompatible core error")
	}
	if !errors.Is(err, domainplugin.ErrIncompatibleCore) {
		t.Fatalf("expected ErrIncompatibleCore, got %v", err)
	}
	if err := m.CompatibleWithCore("0.3.0"); err != nil {
		t.Fatalf("expected compatible: %v", err)
	}
}

func TestValidateMinCoreVersionRejectsGarbage(t *testing.T) {
	if err := domainplugin.ValidateMinCoreVersion("not-a-version"); err == nil {
		t.Fatal("expected invalid minCoreVersion")
	}
}
