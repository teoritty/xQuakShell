package usecase

import (
	"context"
	"fmt"
	"log/slog"
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

// embedAckCeiling bounds how long the inbound pump waits for ONE frame to be accepted by the
// browser-facing surface before it gives up and closes the channel with a reason.
//
// Why 120 s, precisely. ipc's ackStallTimeout is 30 s and is explicitly "a diagnostic threshold,
// not a deadline: nothing is torn down". 120 s is 4x that, so the ack-stall watchdog names the
// stall in the log three full periods before this ceiling ever kills the channel: the diagnosis
// always arrives before the symptom. (Never mute that watchdog to hide a wait — if waits are
// routinely this long, the ceiling is wrong, not the log.)
//
// The upper bound is justified by what the ceiling can no longer be measuring. A backgrounded tab
// PAUSES it rather than spending it (see deliverFrame), so the only thing left it can time out is
// an ACTIVE, FOREGROUND tab whose 256-frame WebSocket send buffer has not drained for two minutes.
// That browser is not slow, it is gone.
const embedAckCeiling = 120 * time.Second

// embedDeliverRetryInterval is the bounded pause between delivery attempts for a transient
// refusal. The pump waits on a timer and on ctx.Done(); it never busy-waits.
const embedDeliverRetryInterval = 25 * time.Millisecond

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
	notifyClose ChannelCloseNotifier

	// ackCeiling / retryInterval are the D4 wait budget. Set to their production constants by the
	// constructor — the ONLY constructor, which production and tests both go through — and lowered
	// by tests afterwards so CI does not sleep for two minutes. There is no test-only branch or
	// alternate constructor here: the same field feeds the same code path either way. This mirrors
	// ChannelExecBackend.sessionOpener, the existing pattern for exactly this.
	ackCeiling    time.Duration
	retryInterval time.Duration

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
//
// notifyClose reports the ceiling/terminal close as channel.close {reason, message}; the plugin
// has no other way to learn WHY its channel died, so a nil notifier degrades to a silent close.
func NewChannelEmbedBackend(pluginID string, hasEmbedCap bool, sink EmbedFrameSink, audit domainplugin.ChannelAuditRecorder, notifyClose ChannelCloseNotifier) *ChannelEmbedBackend {
	return &ChannelEmbedBackend{
		pluginID:      pluginID,
		hasEmbedCap:   hasEmbedCap,
		sink:          sink,
		audit:         audit,
		notifyClose:   notifyClose,
		tunnelID:      embedDefaultTunnelID,
		ackCeiling:    embedAckCeiling,
		retryInterval: embedDeliverRetryInterval,
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
				slog.Debug("embed: channel inbound pump ended (Recv closed)", "pluginId", "com.xquakshell.vnc", "sessionId", parentSessionID)
				_ = b.CloseRemote()
				return
			}
			slog.Debug("embed: channel inbound frame from plugin", "pluginId", "com.xquakshell.vnc", "sessionId", parentSessionID, "tunnelId", tunnelID, "bytes", len(payload))
			if !b.deliverFrame(ctx, ch, parentSessionID, tunnelID, payload) {
				slog.Debug("embed: deliverFrame gave up (channel torn down)", "pluginId", "com.xquakshell.vnc", "sessionId", parentSessionID)
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

// deliverFrame hands one frame to the embed surface, retrying until it is accepted, and reports
// whether the pump may continue. It NEVER drops the frame in hand: waiting is correct on this
// purpose, dropping never is (7.3). The frame is not Acked by the caller until this returns true,
// so an undelivered frame closes the plugin's credit window and the backpressure reaches the
// producer — which is the whole point of B4.
//
// The wait has a ceiling, but a backgrounded tab PAUSES it rather than spending it (D5): that
// refusal shares a sentinel with a momentarily full buffer, yet it is the user's own doing via
// SetSessionActive and legitimately lasts minutes. Charging it would kill the session of every
// user who looks at another tab — the exact defect this function exists to remove. The pause is
// bounded and is not a leak: credit is counted in frames (window 8, frames capped at 64 KiB), so a
// paused channel parks at most 512 KiB with no host-side queue behind it, and it is not the only
// exit — session revoke, plugin crash and the session-close cascade all still close the channel.
// An indefinitely paused channel ends when its session does.
func (b *ChannelEmbedBackend) deliverFrame(ctx context.Context, ch *domainplugin.ChannelHandle, sessionID, tunnelID string, payload []byte) bool {
	var spent time.Duration // ceiling consumed so far; a paused (tab-inactive) wait adds nothing
	for {
		err := b.sink.RouteTunnelFrameFromPlugin(ctx, sessionID, tunnelID, payload)
		if err == nil {
			return true
		}

		cause, classified := EmbedRefusalCauseOf(err)
		slog.Debug("embed: deliverFrame refused", "pluginId", "com.xquakshell.vnc", "cause", string(cause), "classified", classified, "err", err.Error())
		if !classified {
			// An unclassified error from the sink is not something this pump can reason about, and
			// guessing is how "wait" and "tear down" get confused again. Fail where the cause is.
			b.closeWithReason(ch, "embed-error", err.Error())
			return false
		}

		charged := false
		switch cause {
		case EmbedRefusedTabInactive:
			// D5: wait with the ceiling STOPPED.
		case EmbedRefusedWSBufferFull, EmbedRefusedRateLimited, EmbedRefusedTunnelNotAttached:
			// D7: a tunnel that is open but not yet attached is the iframe still loading — a wait,
			// with the ceiling RUNNING. Unlike a backgrounded tab this is nobody's deliberate act,
			// so an iframe that never attaches must not park the channel forever.
			charged = true
		case EmbedRefusedSessionGone, EmbedRefusedTunnelClosed:
			b.closeWithReason(ch, string(cause), err.Error())
			return false
		case EmbedRefusedTunnelUnknown:
			b.closeWithReason(ch, string(cause),
				"plugin bug: this tunnel id was never registered for this session via session.registerEmbed")
			return false
		case EmbedRefusedFrameTooLarge:
			// Unreachable: the channel layer rejects an oversize embed-stream frame on ingress, so
			// it never reaches the sink. Observing it here means that ingress check has a hole —
			// a broken host invariant, which is worth being loud about rather than papering over.
			// Deliberately NOT a second size check: one rule, one place.
			b.closeWithReason(ch, string(cause),
				"host invariant broken: an oversize frame reached the embed sink past the channel ingress check")
			return false
		default:
			// A cause this pump does not know how to wait on is terminal by default: a new cause
			// must be handled deliberately, not silently treated as "retry forever".
			b.closeWithReason(ch, string(cause), err.Error())
			return false
		}

		if charged {
			if spent >= b.ackCeiling {
				b.closeWithReason(ch, string(cause), fmt.Sprintf(
					"embed frame not accepted by the browser for %s: the consumer is not draining", b.ackCeiling))
				return false
			}
		}

		// Bounded wait, never a busy loop — and it selects on ctx.Done() because this goroutine
		// must never be the thing a host teardown waits on: closeResources calls conn.Close()
		// before the reaper's kill, so a pump that can only be unblocked by the kill wedges
		// teardown forever.
		waitStart := time.Now()
		timer := time.NewTimer(b.retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = b.CloseRemote()
			return false
		case <-timer.C:
		}
		if charged {
			spent += time.Since(waitStart)
		}
	}
}

// closeWithReason reports the channel's terminal reason to the plugin (there is no binary error
// frame — ADR-011 puts application-level errors on channel.close) and then detaches.
func (b *ChannelEmbedBackend) closeWithReason(ch *domainplugin.ChannelHandle, reason, message string) {
	if b.notifyClose != nil && !b.isClosed() {
		b.notifyClose(ch.ChannelID(), reason, message)
	}
	_ = b.CloseRemote()
}

// isClosed reports whether this backend has already detached from the embed surface.
func (b *ChannelEmbedBackend) isClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
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
