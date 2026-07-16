package usecase

import (
	"context"
	"strings"
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/pkg/safego"
)

// EmbedFrameSink is the minimal ADR-008 embed-tunnel surface a channel embed-stream backend
// needs (usecase.EmbedTunnelService in production): raw byte delivery in both directions plus
// session ownership lookup, with no JSON/base64 envelope — unlike the ordinary plugin RPC path
// (HandlePluginTunnelFrame / RouteTunnelFrameToPlugin's session.tunnelData notification), which
// base64-encodes only because plain JSON-RPC has no binary transport. The channel bus IS that
// binary transport, so this backend talks straight to raw []byte methods already used
// internally by EmbedTunnelService for the plugin->browser direction, and reuses the same
// per-session/tunnel addressing for the browser->plugin direction.
type EmbedFrameSink interface {
	// RouteTunnelFrameFromPlugin delivers plugin-originated bytes (e.g. decoded video) to the
	// session's browser-facing embed surface.
	RouteTunnelFrameFromPlugin(ctx context.Context, sessionID, tunnelID string, data []byte) error
	// PluginIDForSession reports which plugin owns sessionID's embed registration, so Authorize
	// can refuse a channel for a session the requesting plugin never registered for embed via
	// session.registerEmbed — a channel is never itself how embed ownership is established.
	PluginIDForSession(sessionID string) (string, bool)
	// SubscribeOutbound streams the embed surface's host->plugin bytes (e.g. browser control
	// input) for tunnelID until the returned unsubscribe func is called. A nil channel means
	// this sink has nothing to deliver in that direction.
	SubscribeOutbound(sessionID, tunnelID string) (<-chan []byte, func())
}

const embedDefaultTunnelID = "main"

// ChannelEmbedBackend implements the embed-stream purpose (ADR-011 Stage 8): it authorizes
// against manifest.Capabilities.Session.Embed plus the parentSessionId already being the
// plugin's own registered embed session, then wires the channel straight through to that
// session's embed surface in both directions with no JSON/base64 wrapping. Overflow handling
// is entirely Stage 5's, applied inside ch.Data().Send — this backend only ever calls
// Send/Recv/Ack, it never buffers or evicts frames itself.
type ChannelEmbedBackend struct {
	pluginID    string
	hasEmbedCap bool
	sink        EmbedFrameSink
	audit       domainplugin.ChannelAuditRecorder

	mu              sync.Mutex
	parentSessionID string
	tunnelID        string
	unsubscribe     func()
	closed          bool
}

// NewChannelEmbedBackend creates an embed-stream backend for one channel.open request.
// hasEmbedCap reflects manifest.Capabilities.Session != nil && manifest.Capabilities.Session.Embed,
// resolved once per plugin process by the composition root — the same place every other purpose
// backend's capability flag is resolved (see ChannelExecBackend's consentGranted).
func NewChannelEmbedBackend(pluginID string, hasEmbedCap bool, sink EmbedFrameSink, audit domainplugin.ChannelAuditRecorder) *ChannelEmbedBackend {
	return &ChannelEmbedBackend{
		pluginID:    pluginID,
		hasEmbedCap: hasEmbedCap,
		sink:        sink,
		audit:       audit,
		tunnelID:    embedDefaultTunnelID,
	}
}

// Authorize requires the manifest's session.embed capability and that parentSessionID is
// already this plugin's own registered embed session.
func (b *ChannelEmbedBackend) Authorize(purpose, parentSessionID, hint string) error {
	if purpose != domainplugin.PurposeEmbedStream {
		return domainplugin.ErrCapabilityDenied
	}
	if !b.hasEmbedCap {
		return domainplugin.ErrCapabilityDenied
	}
	parentSessionID = strings.TrimSpace(parentSessionID)
	if parentSessionID == "" || b.sink == nil {
		return domainplugin.ErrCapabilityDenied
	}
	owner, ok := b.sink.PluginIDForSession(parentSessionID)
	if !ok || owner != b.pluginID {
		return domainplugin.ErrCapabilityDenied
	}

	tunnelID := strings.TrimSpace(hint)
	if tunnelID == "" {
		tunnelID = embedDefaultTunnelID
	}

	b.mu.Lock()
	b.parentSessionID = parentSessionID
	b.tunnelID = tunnelID
	b.mu.Unlock()
	return nil
}

// Wire pumps bytes in both directions with no framing beyond the channel bus's own kind=0x02
// frames: plugin -> data.Recv() -> sink.RouteTunnelFrameFromPlugin (e.g. video/output), and the
// embed surface's outbound subscription -> data.Send() (e.g. control/input). Stage 5's
// exhaustion policy applies inside data.Send exactly as it does for every other purpose; this
// method never inspects or overrides it.
func (b *ChannelEmbedBackend) Wire(ctx context.Context, ch *domainplugin.ChannelHandle) error {
	b.mu.Lock()
	parentSessionID := b.parentSessionID
	tunnelID := b.tunnelID
	closed := b.closed
	b.mu.Unlock()
	if parentSessionID == "" || closed {
		return domainplugin.ErrCapabilityDenied
	}

	if b.audit != nil {
		b.audit(domainplugin.ChannelAuditEntry{
			Timestamp:       time.Now(),
			PluginID:        b.pluginID,
			Action:          "channel.open",
			ChannelID:       ch.ChannelID(),
			Purpose:         ch.Purpose(),
			ParentSessionID: ch.ParentSessionID(),
			Target:          tunnelID,
			Success:         true,
		})
	}

	data := ch.Data()

	safego.GoNamed("plugin.channelEmbedInbound", func() {
		for {
			payload, ok := data.Recv()
			if !ok {
				_ = b.CloseRemote()
				return
			}
			if err := b.sink.RouteTunnelFrameFromPlugin(ctx, parentSessionID, tunnelID, payload); err != nil {
				_ = b.CloseRemote()
				return
			}
			// The frame reached the browser-facing surface: the plugin may send one more.
			// Acking here rather than after Recv is what makes the plugin's window track the
			// browser's real consumption instead of the host's buffering.
			if err := data.Ack(ctx); err != nil {
				_ = b.CloseRemote()
				return
			}
		}
	})

	if b.sink != nil {
		outbound, unsubscribe := b.sink.SubscribeOutbound(parentSessionID, tunnelID)
		b.mu.Lock()
		b.unsubscribe = unsubscribe
		b.mu.Unlock()

		if outbound != nil {
			safego.GoNamed("plugin.channelEmbedOutbound", func() {
				for {
					select {
					case payload, ok := <-outbound:
						if !ok {
							return
						}
						if err := data.Send(ctx, payload); err != nil {
							return
						}
					case <-ctx.Done():
						return
					}
				}
			})
		}
	}

	return nil
}

// CloseRemote detaches from the embed surface: it never revokes the session's whole embed
// registration, only this channel's own outbound subscription, so other embed traffic for the
// session (UI, other tunnels) is unaffected. Idempotent: safe to call after an already
// closed/never-wired backend.
func (b *ChannelEmbedBackend) CloseRemote() error {
	b.mu.Lock()
	unsubscribe := b.unsubscribe
	b.unsubscribe = nil
	alreadyClosed := b.closed
	b.closed = true
	b.mu.Unlock()
	if alreadyClosed {
		return nil
	}
	if unsubscribe != nil {
		unsubscribe()
	}
	return nil
}

var _ domainplugin.ChannelPurposeBackend = (*ChannelEmbedBackend)(nil)
