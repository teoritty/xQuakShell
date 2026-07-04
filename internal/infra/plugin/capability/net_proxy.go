package capability

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

type ipResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// NetProxy dials outbound connections allowed by manifest network capabilities.
type NetProxy struct {
	patterns             []string
	allowArbitrary       bool
	allowPrivateNetworks bool
	pluginID             string
	resolver             ipResolver
	mu                   sync.Mutex
	handles              map[string]net.Conn
}

// NewNetProxy creates a network proxy from manifest network capabilities.
func NewNetProxy(pluginID string, caps *domainplugin.NetworkCaps) *NetProxy {
	p := &NetProxy{
		pluginID: pluginID,
		handles:  make(map[string]net.Conn),
		resolver: net.DefaultResolver,
	}
	if caps != nil {
		p.patterns = append([]string(nil), caps.Outbound...)
		p.allowArbitrary = caps.AllowArbitraryOutbound
		p.allowPrivateNetworks = caps.AllowPrivateNetworks
	}
	return p
}

type netDialParams struct {
	Network string `json:"network"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
}

type netHandleParams struct {
	HandleID string `json:"handleId"`
}

type netDialResult struct {
	HandleID string `json:"handleId"`
}

type netReadParams struct {
	HandleID string `json:"handleId"`
	MaxBytes int    `json:"maxBytes,omitempty"`
}

type netReadResult struct {
	ContentBase64 string `json:"contentBase64"`
	EOF           bool   `json:"eof"`
}

type netWriteParams struct {
	HandleID      string `json:"handleId"`
	ContentBase64 string `json:"contentBase64"`
}

// Dial validates the target against manifest patterns and returns a core-owned handle.
func (p *NetProxy) Dial(params json.RawMessage) (json.RawMessage, error) {
	var req netDialParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid net.dial params: %w", err)
	}
	if req.Network == "" {
		req.Network = "tcp"
	}
	if req.Network != "tcp" {
		return nil, domainplugin.ErrCapabilityDenied
	}
	if req.Host == "" || req.Port <= 0 || req.Port > 65535 {
		return nil, fmt.Errorf("invalid net.dial params: host and port required")
	}

	arbitraryAllowsHost := p.allowArbitrary

	allowlistAllowsHost := false
	patternHost := req.Host
	if len(p.patterns) > 0 {
		var ok bool
		patternHost, ok = matchingPatternHost(p.patterns, req.Host, req.Port)
		allowlistAllowsHost = ok
	}

	if !arbitraryAllowsHost && !allowlistAllowsHost {
		return nil, domainplugin.ErrCapabilityDenied
	}

	p.mu.Lock()
	if len(p.handles) >= domainplugin.MaxNetConnectionsPerPlugin {
		p.mu.Unlock()
		return nil, domainplugin.ErrRateLimited
	}
	p.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), domainplugin.NetDialTimeout)
	defer cancel()

	resolver := p.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupIPAddr(ctx, req.Host)
	if err != nil {
		return nil, fmt.Errorf("%w: lookup failed", domainplugin.ErrNetworkDialFailed)
	}

	portStr := fmt.Sprintf("%d", req.Port)
	var dialErr error
	for _, addr := range addrs {
		if !p.shouldAllowResolvedIP(patternHost, addr.IP) {
			continue
		}
		target := net.JoinHostPort(addr.IP.String(), portStr)
		dialer := net.Dialer{Timeout: domainplugin.NetDialTimeout}
		conn, err := dialer.DialContext(ctx, req.Network, target)
		if err != nil {
			dialErr = err
			continue
		}
		if ra, ok := conn.RemoteAddr().(*net.TCPAddr); ok && !p.shouldAllowResolvedIP(patternHost, ra.IP) {
			_ = conn.Close()
			continue
		}

		handleID, err := newNetHandleID()
		if err != nil {
			_ = conn.Close()
			return nil, err
		}

		p.mu.Lock()
		p.handles[handleID] = conn
		p.mu.Unlock()

		if p.allowArbitrary {
			slog.Info("plugin net.dial allowed",
				"pluginId", p.pluginID,
				"host", req.Host,
				"port", req.Port,
				"arbitrary", true,
			)
		}

		return json.Marshal(netDialResult{HandleID: handleID})
	}

	if dialErr != nil {
		return nil, fmt.Errorf("%w: %v", domainplugin.ErrNetworkDialFailed, dialErr)
	}
	return nil, domainplugin.ErrCapabilityDenied
}

// Close closes a handle owned by this plugin process.
func (p *NetProxy) Close(params json.RawMessage) (json.RawMessage, error) {
	var req netHandleParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid net.close params: %w", err)
	}
	if req.HandleID == "" {
		return nil, fmt.Errorf("invalid net.close params: handleId required")
	}
	conn, err := p.takeHandle(req.HandleID)
	if err != nil {
		return nil, err
	}
	_ = conn.Close()
	return json.Marshal(map[string]bool{"ok": true})
}

// Read reads bytes from a handle owned by this plugin process.
// Idle connections (no bytes yet) return an empty success so interactive
// protocols can poll without treating RPC or socket timeouts as disconnect.
func (p *NetProxy) Read(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var req netReadParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid net.read params: %w", err)
	}
	if req.HandleID == "" {
		return nil, fmt.Errorf("invalid net.read params: handleId required")
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 || maxBytes > domainplugin.MaxNetReadBytes {
		maxBytes = domainplugin.MaxNetReadBytes
	}

	conn, err := p.connForHandle(req.HandleID)
	if err != nil {
		return nil, err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	buf := make([]byte, maxBytes)
	type readResult struct {
		n   int
		err error
	}
	ch := make(chan readResult, 1)
	go func() {
		// Established TCP: block until data arrives or the connection closes.
		_ = conn.SetReadDeadline(time.Time{})
		n, readErr := conn.Read(buf)
		ch <- readResult{n: n, err: readErr}
	}()

	select {
	case <-ctx.Done():
		_ = conn.SetReadDeadline(time.Now())
		res := <-ch
		if res.n > 0 {
			return json.Marshal(netReadResult{
				ContentBase64: base64.StdEncoding.EncodeToString(buf[:res.n]),
				EOF:           res.err == io.EOF,
			})
		}
		return json.Marshal(netReadResult{ContentBase64: "", EOF: false})
	case res := <-ch:
		if res.err == io.EOF {
			return json.Marshal(netReadResult{
				ContentBase64: base64.StdEncoding.EncodeToString(buf[:res.n]),
				EOF:           true,
			})
		}
		if res.err != nil {
			if isNetReadIdle(res.err) {
				return json.Marshal(netReadResult{ContentBase64: "", EOF: false})
			}
			return nil, fmt.Errorf("net.read: %w", res.err)
		}
		return json.Marshal(netReadResult{
			ContentBase64: base64.StdEncoding.EncodeToString(buf[:res.n]),
			EOF:           false,
		})
	}
}

func isNetReadIdle(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// Write writes bytes to a handle owned by this plugin process.
func (p *NetProxy) Write(params json.RawMessage) (json.RawMessage, error) {
	var req netWriteParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid net.write params: %w", err)
	}
	if req.HandleID == "" {
		return nil, fmt.Errorf("invalid net.write params: handleId required")
	}
	data, err := base64.StdEncoding.DecodeString(req.ContentBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid contentBase64: %w", err)
	}
	if len(data) > domainplugin.MaxNetWriteBytes {
		return nil, domainplugin.ErrCapabilityDenied
	}

	conn, err := p.connForHandle(req.HandleID)
	if err != nil {
		return nil, err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(domainplugin.NetWriteTimeout)); err != nil {
		return nil, fmt.Errorf("net.write: %w", err)
	}
	if _, err := conn.Write(data); err != nil {
		return nil, fmt.Errorf("net.write: %w", err)
	}
	return json.Marshal(map[string]bool{"ok": true})
}

// CloseAll closes every open handle (process shutdown).
func (p *NetProxy) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, conn := range p.handles {
		_ = conn.Close()
		delete(p.handles, id)
	}
}

func (p *NetProxy) connForHandle(handleID string) (net.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	conn, ok := p.handles[handleID]
	if !ok {
		return nil, domainplugin.ErrHandleNotFound
	}
	return conn, nil
}

func (p *NetProxy) takeHandle(handleID string) (net.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	conn, ok := p.handles[handleID]
	if !ok {
		return nil, domainplugin.ErrHandleNotFound
	}
	delete(p.handles, handleID)
	return conn, nil
}

func newNetHandleID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// shouldAllowResolvedIP decides whether resolved ip may be dialed.
// Implements "either mode permits" (FR-6):
// - arbitrary mode: public IPs allowed; private/loopback only if allowPrivateNetworks
// - allowlist mode: delegates to domain AllowResolvedDialIP unchanged
// - If arbitrary didn't allow, falls through to allowlist check
func (p *NetProxy) shouldAllowResolvedIP(patternHost string, ip net.IP) bool {
	if p.allowArbitrary {
		if !domainplugin.IsRestrictedDialIP(ip) {
			return true
		}
		if p.allowPrivateNetworks {
			return true
		}
	}
	return domainplugin.AllowResolvedDialIP(patternHost, ip)
}
