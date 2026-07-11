package capability

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// udpDialPolicyFixtures reuses dialPolicyFixtures verbatim (the same shared table
// tcp-relay's tests consume) and rewrites only the proto token on each pattern from
// "tcp:" to "udp:", so the IP-restriction core is exercised identically for both
// purposes without a parallel table of IP-blocking cases (ADR-011 Stage 6b).
func udpDialPolicyFixtures() []dialPolicyFixture {
	base := dialPolicyFixtures()
	out := make([]dialPolicyFixture, len(base))
	for i, f := range base {
		udpPatterns := make([]string, len(f.patterns))
		for j, p := range f.patterns {
			udpPatterns[j] = strings.Replace(p, "tcp:", "udp:", 1)
		}
		f.patterns = udpPatterns
		out[i] = f
	}
	return out
}

func TestChannelUDPRelayBackend_HintRejectedIdenticallyToDialPolicyFixtures(t *testing.T) {
	for _, f := range udpDialPolicyFixtures() {
		t.Run(f.name, func(t *testing.T) {
			backend := NewChannelUDPRelayBackend("com.test", &domainplugin.NetworkCaps{
				Outbound:               f.patterns,
				AllowArbitraryOutbound: f.allowArbitrary,
				AllowPrivateNetworks:   f.allowPrivate,
			}, nil)
			backend.resolver = mapResolver{f.host: {f.resolvedIP}}

			hint := net.JoinHostPort(f.host, strconv.Itoa(f.port))
			err := backend.Authorize(domainplugin.PurposeUDPRelay, "sess-1", hint)
			denied := errors.Is(err, domainplugin.ErrCapabilityDenied)
			if denied != f.wantDenied {
				t.Fatalf("%s: denied=%v (err=%v), want denied=%v", f.name, denied, err, f.wantDenied)
			}
		})
	}
}

func TestChannelUDPRelayBackend_LoopbackBlockedUnlessAllowPrivateNetworks(t *testing.T) {
	backend := NewChannelUDPRelayBackend("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
	}, nil)
	backend.resolver = mapResolver{"printer.local": {net.ParseIP("127.0.0.1")}}

	if err := backend.Authorize(domainplugin.PurposeUDPRelay, "sess-1", "printer.local:9"); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("expected loopback denied without allowPrivateNetworks, got %v", err)
	}

	backend2 := NewChannelUDPRelayBackend("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
		AllowPrivateNetworks:   true,
	}, nil)
	backend2.resolver = mapResolver{"printer.local": {net.ParseIP("127.0.0.1")}}
	if err := backend2.Authorize(domainplugin.PurposeUDPRelay, "sess-1", "printer.local:9"); errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("expected loopback allowed with allowPrivateNetworks, got %v", err)
	}
}

func TestChannelUDPRelayBackend_AuditRecordsCanonicalTargetNotRawHint(t *testing.T) {
	peer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()

	port := peer.LocalAddr().(*net.UDPAddr).Port
	rawHint := net.JoinHostPort("dial-me.example", strconv.Itoa(port))
	canonical := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	var mu sync.Mutex
	var entries []domainplugin.ChannelAuditEntry
	backend := NewChannelUDPRelayBackend("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
		AllowPrivateNetworks:   true,
	}, func(entry domainplugin.ChannelAuditEntry) {
		mu.Lock()
		entries = append(entries, entry)
		mu.Unlock()
	})
	backend.resolver = mapResolver{"dial-me.example": {net.ParseIP("127.0.0.1")}}

	if err := backend.Authorize(domainplugin.PurposeUDPRelay, "sess-1", rawHint); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	handle := &domainplugin.ChannelHandle{ChannelID: 1, PluginID: "com.test", Purpose: domainplugin.PurposeUDPRelay, ParentSessionID: "sess-1", Hint: rawHint}
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	defer backend.CloseRemote()

	mu.Lock()
	defer mu.Unlock()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0].Target != "udp:"+canonical {
		t.Fatalf("expected canonical target %q, got %q", "udp:"+canonical, entries[0].Target)
	}
	if entries[0].Target == rawHint {
		t.Fatal("audit target must not equal raw hint")
	}
}

func TestChannelUDPRelayBackend_RoundTripOneFrameOneDatagram(t *testing.T) {
	peer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()

	port := peer.LocalAddr().(*net.UDPAddr).Port
	host := "127.0.0.1"

	backend := NewChannelUDPRelayBackend("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
		AllowPrivateNetworks:   true,
	}, nil)
	backend.resolver = mapResolver{host: {net.ParseIP(host)}}

	hint := net.JoinHostPort(host, strconv.Itoa(port))
	if err := backend.Authorize(domainplugin.PurposeUDPRelay, "sess-1", hint); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	data := newFakeChannelDataPath()
	handle := &domainplugin.ChannelHandle{ChannelID: 2, PluginID: "com.test", Purpose: domainplugin.PurposeUDPRelay, ParentSessionID: "sess-1", Hint: hint, Data: data}
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	defer backend.CloseRemote()

	data.pushInbound([]byte("ping"))

	buf := make([]byte, 64)
	peer.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, remoteAddr, err := peer.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("peer read: %v", err)
	}
	if string(buf[:n]) != "ping" {
		t.Fatalf("peer got %q, want ping", buf[:n])
	}

	if _, err := peer.WriteToUDP([]byte("pong"), remoteAddr); err != nil {
		t.Fatalf("peer write: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		frames := data.sentFrames()
		if len(frames) > 0 {
			if string(frames[0]) != "pong" {
				t.Fatalf("relay->plugin frame = %q, want pong", frames[0])
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for relay->plugin frame")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestChannelUDPRelayBackend_CreditZeroSuspendsSocketReads(t *testing.T) {
	peer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()

	port := peer.LocalAddr().(*net.UDPAddr).Port
	host := "127.0.0.1"

	backend := NewChannelUDPRelayBackend("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
		AllowPrivateNetworks:   true,
	}, nil)
	backend.resolver = mapResolver{host: {net.ParseIP(host)}}

	hint := net.JoinHostPort(host, strconv.Itoa(port))
	if err := backend.Authorize(domainplugin.PurposeUDPRelay, "sess-1", hint); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	data := newFakeChannelDataPath()
	data.blockCapacity()
	handle := &domainplugin.ChannelHandle{ChannelID: 3, PluginID: "com.test", Purpose: domainplugin.PurposeUDPRelay, ParentSessionID: "sess-1", Hint: hint, Data: data}
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	defer backend.CloseRemote()

	// Need the backend's local UDP address to send datagrams to it.
	backend.mu.Lock()
	localAddr := backend.conn.LocalAddr()
	backend.mu.Unlock()

	if _, err := peer.WriteTo([]byte("held-back"), localAddr); err != nil {
		t.Fatalf("peer write: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	if frames := data.sentFrames(); len(frames) != 0 {
		t.Fatalf("expected no frames delivered while capacity blocked, got %d", len(frames))
	}

	data.releaseCapacity()

	if _, err := peer.WriteTo([]byte("flows-now"), localAddr); err != nil {
		t.Fatalf("peer write: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		frames := data.sentFrames()
		found := false
		for _, f := range frames {
			if string(f) == "flows-now" {
				found = true
			}
		}
		if found {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for datagram after releasing capacity")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestChannelUDPRelayBackend_IdleReapClosesChannel(t *testing.T) {
	peer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()

	port := peer.LocalAddr().(*net.UDPAddr).Port
	host := "127.0.0.1"

	backend := NewChannelUDPRelayBackend("com.test", &domainplugin.NetworkCaps{
		AllowArbitraryOutbound: true,
		AllowPrivateNetworks:   true,
	}, nil)
	backend.resolver = mapResolver{host: {net.ParseIP(host)}}
	backend.idleTimeout = 50 * time.Millisecond

	var closed atomicBool
	backend.onIdleReap = func() { closed.set(true) }

	hint := net.JoinHostPort(host, strconv.Itoa(port))
	if err := backend.Authorize(domainplugin.PurposeUDPRelay, "sess-1", hint); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	data := newFakeChannelDataPath()
	handle := &domainplugin.ChannelHandle{ChannelID: 4, PluginID: "com.test", Purpose: domainplugin.PurposeUDPRelay, ParentSessionID: "sess-1", Hint: hint, Data: data}
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for !closed.get() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for idle reap")
		case <-time.After(10 * time.Millisecond):
		}
	}

	backend.mu.Lock()
	isClosed := backend.closed
	backend.mu.Unlock()
	if !isClosed {
		t.Fatal("expected backend closed after idle reap")
	}
}

type atomicBool struct {
	mu sync.Mutex
	v  bool
}

func (a *atomicBool) set(v bool) {
	a.mu.Lock()
	a.v = v
	a.mu.Unlock()
}

func (a *atomicBool) get() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.v
}
