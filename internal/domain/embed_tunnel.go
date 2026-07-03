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
	MaxTunnelsPerSession = 4
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

// EmbedTunnelRegistry manages embed token lifecycle and tunnel frame routing.
type EmbedTunnelRegistry interface {
	Register(ctx context.Context, reg EmbedRegistration) (SessionEmbedDescriptor, error)
	RevokeBySession(sessionID string) error
	Lookup(token string) (EmbedRegistration, error)
	RouteTunnelFrameFromPlugin(ctx context.Context, sessionID, tunnelID string, data []byte) error
	SetSessionActive(sessionID string, active bool)
}
