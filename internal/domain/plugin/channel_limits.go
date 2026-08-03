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

// MaxEmbedStreamFrameBytes caps one embed-stream binary frame at 64 KiB, below the 1 MiB
// MaxBinaryFrameBytes every other purpose gets.
//
// It mirrors domain.MaxTunnelFrameSize, which the embed leg has always enforced, and is declared
// here rather than imported because domain/plugin does not depend on domain. Both halves are
// pinned together by a test; if one moves, that test says so.
//
// WHY this purpose is special: credit is counted in frames, not bytes, so bytes parked in the host
// are window x frame size. embed-stream's window is 8 frames, so a 1 MiB cap would let 8 MiB per
// channel sit in host memory that the plugin's Job Object does not bound -- the unbounded
// host-side queue the credit window exists to close, reopened through another door. A plugin with
// more than 64 KiB to say chunks it, where it knows what a frame means (D2).
const MaxEmbedStreamFrameBytes = 64 * 1024

// MaxFrameBytesForPurpose returns the largest kind=0x02 payload a channel of this purpose may
// carry. It is the single authority on per-purpose frame size: the ingress check in the channel
// layer reads it, and nothing downstream re-checks, so an oversize embed-stream frame can never
// reach the embed sink at all.
func MaxFrameBytesForPurpose(purpose string) int {
	if purpose == PurposeEmbedStream {
		return MaxEmbedStreamFrameBytes
	}
	return MaxBinaryFrameBytes
}

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
