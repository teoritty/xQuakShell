package capability

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/pkg/safego"
)

// ChannelUDPRelayBackend implements the udp-relay purpose (ADR-011 Stage 6b): it wires a
// channel to a host-dialed UDP target chosen by the plugin's hint, validated through the same
// dial-policy chain tcp-relay uses (matchingPatternHost, shouldAllowResolvedIP), matched
// against the plugin's udp: allowlist entries instead of tcp:. UDP has no connection close, so
// the channel is additionally reaped on an idle timeout, reusing the PreBindTunnelTimeout
// time.AfterFunc pattern already established by dynamic_forward_coordinator.go.
type ChannelUDPRelayBackend struct {
	pluginID string
	caps     *domainplugin.NetworkCaps
	resolver ipResolver
	audit    domainplugin.ChannelAuditRecorder

	idleTimeout time.Duration
	onIdleReap  func() // test hook, invoked in addition to the real close path

	mu      sync.Mutex
	target  string // canonical resolved host:port, set by Authorize, dialed by Wire
	conn    *net.UDPConn
	closed  bool
	idleTmr *time.Timer
}

// NewChannelUDPRelayBackend creates a udp-relay backend for one channel.open request. caps is
// the plugin's manifest network capability (the same NetworkCaps tcp-relay enforces) — udp-relay
// reuses its allowlist and allowArbitrary/allowPrivateNetworks flags, matched against udp:
// pattern entries only.
func NewChannelUDPRelayBackend(pluginID string, caps *domainplugin.NetworkCaps, audit domainplugin.ChannelAuditRecorder) *ChannelUDPRelayBackend {
	return &ChannelUDPRelayBackend{
		pluginID:    pluginID,
		caps:        caps,
		resolver:    net.DefaultResolver,
		audit:       audit,
		idleTimeout: domainplugin.PreBindTunnelTimeout,
	}
}

// Authorize validates hint ("host:port") through the existing dial-policy chain against the
// plugin's udp: allowlist entries only — no parallel validator.
func (b *ChannelUDPRelayBackend) Authorize(purpose, _ string, hint string) error {
	if purpose != domainplugin.PurposeUDPRelay {
		return domainplugin.ErrCapabilityDenied
	}

	host, portStr, err := net.SplitHostPort(strings.TrimSpace(hint))
	if err != nil {
		return domainplugin.ErrCapabilityDenied
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return domainplugin.ErrCapabilityDenied
	}

	var allowArbitrary, allowPrivate bool
	var patterns []string
	if b.caps != nil {
		allowArbitrary = b.caps.AllowArbitraryOutbound
		allowPrivate = b.caps.AllowPrivateNetworks
		patterns = udpOnlyPatterns(b.caps.Outbound)
	}

	patternHost := host
	allowlistAllowsHost := false
	if len(patterns) > 0 {
		var ok bool
		patternHost, ok = matchingPatternHost(patterns, host, port)
		allowlistAllowsHost = ok
	}
	if !allowArbitrary && !allowlistAllowsHost {
		return domainplugin.ErrCapabilityDenied
	}

	resolver := b.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(context.Background(), domainplugin.NetDialTimeout)
	defer cancel()
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("%w: lookup failed", domainplugin.ErrNetworkDialFailed)
	}

	for _, addr := range addrs {
		if shouldAllowResolvedIP(allowArbitrary, allowPrivate, patternHost, addr.IP) {
			b.mu.Lock()
			b.target = net.JoinHostPort(addr.IP.String(), portStr)
			b.mu.Unlock()
			return nil
		}
	}
	return domainplugin.ErrCapabilityDenied
}

// udpOnlyPatterns filters a plugin's manifest outbound patterns to the udp: entries Authorize
// matches against, via the same ParseNetworkPattern every other dial-policy caller uses.
func udpOnlyPatterns(patterns []string) []string {
	out := make([]string, 0, len(patterns))
	for _, p := range patterns {
		parsed, err := domainplugin.ParseNetworkPattern(p)
		if err != nil || parsed.Proto != "udp" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// Wire dials the target Authorize validated and pumps datagrams between the UDP socket and the
// channel's data path: each inbound plugin frame is sent as exactly one datagram, and each
// datagram received is emitted as exactly one outbound frame (1:1, no batching). The socket
// read is gated on ch.Data.WaitForCapacity so at credit 0 the backend stops reading — excess
// datagrams are then dropped by the OS receive buffer, which is the correct bounded behavior
// for UDP (ADR-011 §2b), not a growing in-process queue. An idle-reap timer, armed here and
// reset on every datagram in either direction, tears the channel down when no traffic flows —
// UDP has no connection-close signal to detect a dead peer.
func (b *ChannelUDPRelayBackend) Wire(ctx context.Context, ch *domainplugin.ChannelHandle) error {
	b.mu.Lock()
	target := b.target
	b.mu.Unlock()
	if target == "" {
		return domainplugin.ErrCapabilityDenied
	}

	udpAddr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		return fmt.Errorf("%w: %v", domainplugin.ErrNetworkDialFailed, err)
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return fmt.Errorf("%w: %v", domainplugin.ErrNetworkDialFailed, err)
	}

	b.mu.Lock()
	b.conn = conn
	b.mu.Unlock()

	if b.audit != nil {
		b.audit(domainplugin.ChannelAuditEntry{
			Timestamp:       time.Now(),
			PluginID:        b.pluginID,
			Action:          "channel.open",
			ChannelID:       ch.ChannelID,
			Purpose:         ch.Purpose,
			ParentSessionID: ch.ParentSessionID,
			Target:          "udp:" + target,
			Success:         true,
		})
	}

	b.armIdleTimer()

	if ch.Data == nil {
		return nil
	}
	data := ch.Data

	safego.GoNamed("plugin.channelUDPRelayInbound", func() {
		for {
			payload, ok := data.Recv()
			if !ok {
				_ = b.CloseRemote()
				return
			}
			if _, err := conn.Write(payload); err != nil {
				_ = b.CloseRemote()
				return
			}
			b.resetIdleTimer()
		}
	})

	safego.GoNamed("plugin.channelUDPRelayOutbound", func() {
		buf := make([]byte, 64*1024)
		for {
			if err := data.WaitForCapacity(ctx); err != nil {
				return
			}
			n, err := conn.Read(buf)
			if n > 0 {
				if sendErr := data.Send(ctx, append([]byte(nil), buf[:n]...)); sendErr != nil {
					return
				}
				b.resetIdleTimer()
			}
			if err != nil {
				return
			}
		}
	})

	return nil
}

func (b *ChannelUDPRelayBackend) armIdleTimer() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.idleTmr = time.AfterFunc(b.idleTimeout, b.fireIdleReap)
}

func (b *ChannelUDPRelayBackend) resetIdleTimer() {
	b.mu.Lock()
	timer := b.idleTmr
	closed := b.closed
	b.mu.Unlock()
	if closed || timer == nil {
		return
	}
	timer.Reset(b.idleTimeout)
}

// fireIdleReap invokes the same close path as an explicit channel.close, mirroring how
// dynamic_forward_coordinator's armChannelTimeout closes the channel when its own
// PreBindTunnelTimeout fires with no activity.
func (b *ChannelUDPRelayBackend) fireIdleReap() {
	_ = b.CloseRemote()
	if b.onIdleReap != nil {
		b.onIdleReap()
	}
}

// CloseRemote closes the dialed UDP socket and cancels the idle-reap timer. Idempotent: safe to
// call after an already closed/never-dialed backend.
func (b *ChannelUDPRelayBackend) CloseRemote() error {
	b.mu.Lock()
	conn := b.conn
	b.conn = nil
	alreadyClosed := b.closed
	b.closed = true
	timer := b.idleTmr
	b.idleTmr = nil
	b.mu.Unlock()

	if timer != nil {
		timer.Stop()
	}

	if alreadyClosed || conn == nil {
		return nil
	}
	return conn.Close()
}
