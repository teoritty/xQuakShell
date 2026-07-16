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

// ChannelRelayBackend implements the tcp-relay purpose (ADR-011 Stage 6): it wires a channel
// to a host-dialed TCP target chosen by the plugin's hint, validated through the same
// dial-policy chain NetProxy already enforces for net.dial — matchingPatternHost and
// shouldAllowResolvedIP (net_proxy_match.go), backed by IsRestrictedDialIP/AllowResolvedDialIP
// (domain net_dial_policy.go). There is no separate validation path here, only a new caller.
type ChannelRelayBackend struct {
	pluginID string
	caps     *domainplugin.NetworkCaps
	resolver ipResolver
	audit    domainplugin.ChannelAuditRecorder

	mu     sync.Mutex
	target string // canonical resolved host:port, set by Authorize, dialed by Wire
	conn   net.Conn
	closed bool
}

// NewChannelRelayBackend creates a tcp-relay backend for one channel.open request. caps is the
// plugin's manifest network capability (the same NetworkCaps NetProxy/TunnelDialProxy already
// enforce) — tcp-relay reuses its Outbound allowlist and allowArbitrary/allowPrivateNetworks
// flags rather than introducing a parallel per-purpose allowlist.
func NewChannelRelayBackend(pluginID string, caps *domainplugin.NetworkCaps, audit domainplugin.ChannelAuditRecorder) *ChannelRelayBackend {
	return &ChannelRelayBackend{
		pluginID: pluginID,
		caps:     caps,
		resolver: net.DefaultResolver,
		audit:    audit,
	}
}

// Authorize validates hint ("host:port") through the existing dial-policy chain against the
// plugin's tcp: allowlist entries only, leaving udp: entries to the Stage 6b udp-relay backend.
func (b *ChannelRelayBackend) Authorize(purpose, _ string, hint string) error {
	if purpose != domainplugin.PurposeTCPRelay {
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
		patterns = tcpOnlyPatterns(b.caps.Outbound)
	}

	// Matches NetProxy.Dial exactly: when patterns are configured but none match, patternHost
	// becomes "" rather than staying host, so an unmatched IP-literal hint cannot pass
	// AllowResolvedDialIP's "explicit pattern host == resolved IP" carve-out. Only an actual
	// allowlist match (or the zero-patterns arbitrary-only case below) may set patternHost.
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

// tcpOnlyPatterns filters a plugin's manifest outbound patterns to the tcp: entries Authorize
// matches against, via the same ParseNetworkPattern every other dial-policy caller uses.
func tcpOnlyPatterns(patterns []string) []string {
	out := make([]string, 0, len(patterns))
	for _, p := range patterns {
		parsed, err := domainplugin.ParseNetworkPattern(p)
		if err != nil || parsed.Proto != "tcp" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// Wire dials the target Authorize validated and, once connected, pumps bytes between the TCP
// conn and the channel's data path: inbound plugin frames are written to the conn; conn reads
// are gated on ch.Data.WaitForCapacity so backpressure suspends the upstream TCP read instead
// of growing an unbounded in-process buffer (ADR-011 §2b).
func (b *ChannelRelayBackend) Wire(ctx context.Context, ch *domainplugin.ChannelHandle) error {
	b.mu.Lock()
	target := b.target
	b.mu.Unlock()
	if target == "" {
		return domainplugin.ErrCapabilityDenied
	}

	dialer := net.Dialer{Timeout: domainplugin.NetDialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", target)
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
			ChannelID:       ch.ChannelID(),
			Purpose:         ch.Purpose(),
			ParentSessionID: ch.ParentSessionID(),
			Target:          target,
			Success:         true,
		})
	}

	data := ch.Data()

	safego.GoNamed("plugin.channelRelayInbound", func() {
		for {
			payload, ok := data.Recv()
			if !ok {
				_ = conn.Close()
				return
			}
			if _, err := conn.Write(payload); err != nil {
				_ = conn.Close()
				return
			}
			// The bytes are on the relayed socket: the plugin may send one more frame. A slow
			// or blocked peer therefore holds the plugin's window shut instead of letting it
			// pile frames up in the host.
			if err := data.Ack(ctx); err != nil {
				_ = conn.Close()
				return
			}
		}
	})

	safego.GoNamed("plugin.channelRelayOutbound", func() {
		buf := make([]byte, 32*1024)
		for {
			if err := data.WaitForCapacity(ctx); err != nil {
				return
			}
			n, err := conn.Read(buf)
			if n > 0 {
				if sendErr := data.Send(ctx, append([]byte(nil), buf[:n]...)); sendErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	})

	return nil
}

// CloseRemote closes the dialed TCP connection. Idempotent: safe to call after an already
// closed/never-dialed backend.
func (b *ChannelRelayBackend) CloseRemote() error {
	b.mu.Lock()
	conn := b.conn
	b.conn = nil
	alreadyClosed := b.closed
	b.closed = true
	b.mu.Unlock()

	if alreadyClosed || conn == nil {
		return nil
	}
	return conn.Close()
}
