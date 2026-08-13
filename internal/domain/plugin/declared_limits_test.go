package plugin

import "testing"

// The manifest is the plugin's own file. Every one of these numbers was used as the host's limit
// whenever it was greater than zero, so a plugin asking for a million concurrent channels got a
// million - a declaration of intent read as an authorisation.
func TestDeclaredLimitsAreClampedToTheHostCeiling(t *testing.T) {
	tests := []struct {
		name     string
		resolve  func(int) int
		declared int
		want     int
	}{
		{"channel concurrency above the ceiling", EffectiveMaxConcurrentChannels, 1_000_000, MaxAllowedConcurrentChannels},
		{"channel concurrency just above", EffectiveMaxConcurrentChannels, MaxAllowedConcurrentChannels + 1, MaxAllowedConcurrentChannels},
		{"tunnel channels above the ceiling", EffectiveMaxTunnelChannels, 1_000_000, MaxAllowedTunnelChannels},
		{"throughput above the ceiling", EffectiveChannelThroughputKbps, 1 << 30, DefaultChannelThroughputKbps},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.resolve(tt.declared); got != tt.want {
				t.Errorf("resolve(%d) = %d, want %d; a manifest cannot vote itself more than the host allows", tt.declared, got, tt.want)
			}
		})
	}
}

// A manifest asking for less is the author saying what they need, and holding them to it costs
// nothing. Ignoring it would make the ceiling the only limit that ever applies.
func TestDeclaredLimitsBelowTheDefaultAreHonoured(t *testing.T) {
	tests := []struct {
		name     string
		resolve  func(int) int
		declared int
	}{
		{"one channel", EffectiveMaxConcurrentChannels, 1},
		{"two tunnel channels", EffectiveMaxTunnelChannels, 2},
		{"a slow channel", EffectiveChannelThroughputKbps, 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.resolve(tt.declared); got != tt.declared {
				t.Errorf("resolve(%d) = %d, want %d; a self-imposed smaller limit must be honoured", tt.declared, got, tt.declared)
			}
		})
	}
}

// Absent, zero and negative all mean "the manifest said nothing". Negative especially: it must not
// reach a counter as a limit, or the first comparison against it lets everything through.
func TestDeclaredLimitsFallBackToTheHostDefault(t *testing.T) {
	tests := []struct {
		name    string
		resolve func(int) int
		want    int
	}{
		{"channel concurrency", EffectiveMaxConcurrentChannels, DefaultMaxConcurrentChannels},
		{"tunnel channels", EffectiveMaxTunnelChannels, DefaultMaxTunnelChannels},
		{"throughput", EffectiveChannelThroughputKbps, DefaultChannelThroughputKbps},
	}
	for _, tt := range tests {
		for _, declared := range []int{0, -1, -1_000_000} {
			t.Run(tt.name, func(t *testing.T) {
				if got := tt.resolve(declared); got != tt.want {
					t.Errorf("resolve(%d) = %d, want the host default %d", declared, got, tt.want)
				}
			})
		}
	}
}

// A ceiling below the default would mean the host default is itself unreachable, which would make
// the default a lie. This is the invariant that keeps the two constants in a sane relationship if
// either is ever retuned.
func TestHostDefaultsAreWithinTheirCeilings(t *testing.T) {
	if DefaultMaxConcurrentChannels > MaxAllowedConcurrentChannels {
		t.Errorf("DefaultMaxConcurrentChannels (%d) exceeds its ceiling (%d)", DefaultMaxConcurrentChannels, MaxAllowedConcurrentChannels)
	}
	if DefaultMaxTunnelChannels > MaxAllowedTunnelChannels {
		t.Errorf("DefaultMaxTunnelChannels (%d) exceeds its ceiling (%d)", DefaultMaxTunnelChannels, MaxAllowedTunnelChannels)
	}
}
