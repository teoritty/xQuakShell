package plugin

// Ceilings on the resource limits a plugin declares for itself in its manifest.
//
// Three places took a number out of plugin.json and used it as the host's limit whenever it was
// greater than zero: channel.maxConcurrent, channel.maxThroughputKbps and tunnel
// maxConcurrentChannels. The manifest is the plugin's own file, so "the plugin's limit" is
// whatever the plugin wrote there - a declaration of intent read as an authorisation. A plugin
// asking for a million concurrent channels got a million.
//
// The rule below is the same asymmetry the plugin trust settings use, and for the same reason: a
// declaration is honoured when it makes the plugin's own footprint SMALLER, and clamped when it
// tries to make it larger than the host is willing to give. A manifest that asks for less is
// useful information - it is the author saying what they need, and holding them to it costs
// nothing. A manifest that asks for more is not information, it is a request, and the host answers
// it.
//
// The values are clamped rather than rejected at validation. Tightening a validation branch is a
// breaking manifest change under ADR-017 - a plugin that installs today would stop installing -
// and the security benefit is identical either way, because an over-large declaration is capped
// before it reaches anything that allocates.
const (
	// MaxAllowedConcurrentChannels caps channel.maxConcurrent. Each channel holds host-side state
	// and a credit window whose parked bytes the plugin's own process limits do not bound, so this
	// is the number that decides how much of the host one plugin can occupy.
	MaxAllowedConcurrentChannels = 64

	// MaxAllowedTunnelChannels caps a tunnel provider's maxConcurrentChannels. Each one is a live
	// SSH channel on the user's connection, so the ceiling is lower than the channel bus: these
	// consume a resource shared with the user's own session.
	MaxAllowedTunnelChannels = 64
)

// EffectiveMaxConcurrentChannels resolves the channel-bus concurrency limit for a manifest.
func EffectiveMaxConcurrentChannels(declared int) int {
	return clampDeclaredLimit(declared, DefaultMaxConcurrentChannels, MaxAllowedConcurrentChannels)
}

// EffectiveMaxTunnelChannels resolves the tunnel.dial concurrency limit for a manifest.
func EffectiveMaxTunnelChannels(declared int) int {
	return clampDeclaredLimit(declared, DefaultMaxTunnelChannels, MaxAllowedTunnelChannels)
}

// EffectiveChannelThroughputKbps resolves the per-channel bandwidth cap for a manifest.
//
// Here the ceiling IS the default: the host default is already 32 MiB/s over local IPC, so there is
// no honest reason for a plugin to ask for more, and asking for more only means switching its own
// rate limiter off.
func EffectiveChannelThroughputKbps(declared int) int {
	return clampDeclaredLimit(declared, DefaultChannelThroughputKbps, DefaultChannelThroughputKbps)
}

// clampDeclaredLimit applies the rule: absent or nonsensical means the host default, a smaller
// number is honoured, and a larger one is cut down to the ceiling.
func clampDeclaredLimit(declared, hostDefault, ceiling int) int {
	if declared <= 0 {
		return hostDefault
	}
	if declared > ceiling {
		return ceiling
	}
	return declared
}
