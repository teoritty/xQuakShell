package domain

import (
	"context"
	"time"
)

const (
	// DefaultEmbedTokenTTL is how long an embed session token remains valid.
	DefaultEmbedTokenTTL = 8 * time.Hour
	// MaxTunnelFrameSize is the maximum opaque tunnel frame size (64 KiB).
	MaxTunnelFrameSize = 64 * 1024
	// DefaultTunnelBandwidthBytesPerSec is the default aggregate tunnel rate limit (32 MiB/s).
	DefaultTunnelBandwidthBytesPerSec = 32 * 1024 * 1024
	// MaxTunnelsPerSession is the maximum concurrent tunnels per embed session.
	//
	// 8, and the number is only defensible as a pair: the channel counts of the protocols this API
	// exists for, AND the per-tunnel memory cost. Neither half justifies it alone, which is why
	// both are written down here.
	//
	// WHY NOT 4. 4 does not fit the protocols. SPICE is natively six channels (main, display,
	// inputs, cursor, playback, record). A realistic RDP set is six as well: display, clipboard,
	// audio-out, audio-in, drive, printer. VNC needs one. 4 fits VNC and nothing else -- it is a
	// single-stream number chosen before multi-channel protocols were on the table, and leaving it
	// is what would make this API VNC-only.
	//
	// WHY 8. It covers SPICE's six and RDP's six with headroom for one or two optional channels
	// (smartcard, USB redirection). A ceiling should bind abuse, not design.
	//
	// WHY NOT 16 OR UNBOUNDED. Cost is linear per tunnel and lands in HOST memory, which the
	// plugin's Job Object does not bound: one WebSocket, one send queue and one credit window each.
	// Since embedWSSendQueueDepth was pinned to the embed-stream credit window (D10) that is
	// ~8 x 64 KiB = 512 KiB per tunnel, so 8 tunnels cost ~4 MiB per session. Before D10 the queue
	// alone was 256 frames -- ~16.5 MiB per tunnel, 132 MiB per session at this ceiling -- which is
	// why raising it was only affordable after that landed, and not before.
	//
	// WHY NOT CONFIGURABLE. YAGNI (AI_GUIDELINES 4.5): one honest constant with a reason beats a
	// knob nobody sets correctly. It is deliberately not a manifest field either, unlike
	// channel.maxConcurrent/maxThroughputKbps: a consent line reading "may open up to 6 embed
	// tunnels" is not actionable -- nobody can weigh 6 against 8 -- whereas allowArbitraryOutbound
	// is a decision a human can actually make. If the per-tunnel cost ever stops being cheap,
	// revisit; that, not protocol count, is what would justify a manifest field.
	//
	// THE LINE THIS DOES NOT CROSS. A protocol-INFORMED value is correct here; protocol-AWARE
	// behaviour is not. The core never interprets a tunnel id: TunnelIDs is []string, hint is
	// opaque, frames are opaque []byte, and the id is only ever a map key. `if tunnelID ==
	// "clipboard"` anywhere outside a plugin is the violation this comment guards against. Today
	// there is none. Keep it that way.
	MaxTunnelsPerSession = 8
)

// SessionEmbedDescriptor is delivered to the frontend when an embed session is ready.
type SessionEmbedDescriptor struct {
	SessionID string
	PluginID  string
	UIUrl     string
	TunnelUrl string
	Sandbox   []string
	ExpiresAt time.Time
}

// EmbedRegistration binds a minted token to a session-scoped embed surface.
type EmbedRegistration struct {
	Token     string
	SessionID string
	PluginID  string
	UIEntry   string
	TunnelIDs []string
	ExpiresAt time.Time
	Revoked   bool
}

// EmbedTunnelStream is the browser-side tunnel handle returned by AttachWebSocket.
type EmbedTunnelStream interface {
	Send() <-chan []byte
	Done() <-chan struct{}
}

// EmbedTunnelPort is the infra-facing port for the embed HTTP broker (/embed/s/…).
type EmbedTunnelPort interface {
	Lookup(token string) (EmbedRegistration, error)
	AttachWebSocket(token, tunnelID string) (EmbedTunnelStream, EmbedRegistration, error)
	RouteTunnelFrameToPlugin(ctx context.Context, sessionID, tunnelID string, data []byte) error
}

// EmbedTunnelRegistry manages embed token lifecycle and tunnel frame routing.
type EmbedTunnelRegistry interface {
	Register(ctx context.Context, reg EmbedRegistration) (SessionEmbedDescriptor, error)
	RevokeBySession(sessionID string) error
	Lookup(token string) (EmbedRegistration, error)
	RouteTunnelFrameFromPlugin(ctx context.Context, sessionID, tunnelID string, data []byte) error
	SetSessionActive(sessionID string, active bool)
}
