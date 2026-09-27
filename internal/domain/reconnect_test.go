package domain_test

import (
	"encoding/json"
	"testing"

	"xquakshell/internal/domain"
)

// Both settings arrived after vaults existed. A vault written before them has no "reconnect" key at
// all, and that has to read as on - the defaults the feature ships with - not as two opt-outs
// nobody made.
func TestReconnectSettingsDefaultToOnInAnOlderVault(t *testing.T) {
	var settings domain.AppSettings
	if err := json.Unmarshal([]byte(`{"theme":"dark"}`), &settings); err != nil {
		t.Fatal(err)
	}
	if !settings.Reconnect.AutoReconnectEnabled() {
		t.Error("auto-reconnect must resolve to on when the vault never stored it")
	}
	if !settings.Reconnect.PreserveContextEnabled() {
		t.Error("context preservation must resolve to on when the vault never stored it")
	}
}

func TestReconnectSettingsHonourAnExplicitChoice(t *testing.T) {
	off, on := false, true
	cases := []struct {
		name     string
		settings domain.ReconnectSettings
		auto     bool
		preserve bool
	}{
		{"both off", domain.ReconnectSettings{AutoReconnect: &off, PreserveContext: &off}, false, false},
		{"both on", domain.ReconnectSettings{AutoReconnect: &on, PreserveContext: &on}, true, true},
		// The two are independent: keeping the terminal on a manual reconnect is useful on its own.
		{"only preserve", domain.ReconnectSettings{AutoReconnect: &off, PreserveContext: &on}, false, true},
		{"only auto", domain.ReconnectSettings{AutoReconnect: &on, PreserveContext: &off}, true, false},
	}
	for _, tc := range cases {
		if got := tc.settings.AutoReconnectEnabled(); got != tc.auto {
			t.Errorf("%s: AutoReconnectEnabled = %v, want %v", tc.name, got, tc.auto)
		}
		if got := tc.settings.PreserveContextEnabled(); got != tc.preserve {
			t.Errorf("%s: PreserveContextEnabled = %v, want %v", tc.name, got, tc.preserve)
		}
	}
}

// An explicit false has to survive being stored, or the opt-out would silently undo itself on the
// next unlock. omitempty on a *bool omits only nil, which is the property this pins.
func TestReconnectOptOutSurvivesTheVaultRoundTrip(t *testing.T) {
	off := false
	raw, err := json.Marshal(domain.AppSettings{Reconnect: domain.ReconnectSettings{AutoReconnect: &off, PreserveContext: &off}})
	if err != nil {
		t.Fatal(err)
	}
	var back domain.AppSettings
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Reconnect.AutoReconnectEnabled() || back.Reconnect.PreserveContextEnabled() {
		t.Fatalf("an explicit opt-out came back as on after a round trip: %s", raw)
	}
}
