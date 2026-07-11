package plugin

import "time"

const (
	// MaxBinaryFrameBytes is the maximum payload size for kind=0x02 binary channel frames.
	MaxBinaryFrameBytes = 1 << 20
	// FrameHeaderLen is the fixed frame header size: 4B length + 1B kind + 4B channelId.
	FrameHeaderLen = 9

	// FrameKindJSONRPC marks a frame carrying a JSON-RPC message on the control plane.
	FrameKindJSONRPC = 0x01
	// FrameKindBinary marks a frame carrying raw binary channel data.
	FrameKindBinary = 0x02
	// FrameKindCredit marks a frame carrying a credit/window update.
	FrameKindCredit = 0x03

	// DefaultMaxConcurrentChannels is the host default when channel.maxConcurrent is 0/absent.
	DefaultMaxConcurrentChannels = 4
	// DefaultChannelThroughputKbps is the host default when channel.maxThroughputKbps is 0,
	// matching the existing maxTunnelBandwidthKbps default of 32 MiB/s.
	DefaultChannelThroughputKbps = 32 * 1024

	// ChannelOpenTimeout is the plugin->host channel.open RPC timeout (matches initialize).
	ChannelOpenTimeout = 10 * time.Second
)

// InitialCredit returns the initial per-channel credit (in frames) granted for a purpose.
// Unknown purposes return 0 (no channel of that purpose can validly exist).
func InitialCredit(purpose string) int {
	switch purpose {
	case PurposeExec, PurposeTCPRelay, PurposeUDPRelay:
		return 4
	case PurposeEmbedStream:
		return 8
	default:
		return 0
	}
}
