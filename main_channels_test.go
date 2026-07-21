package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/capability"
	"xquakshell/internal/infra/plugin/ipc"
	"xquakshell/internal/pkg/ratelimit"
	"xquakshell/internal/usecase"
)

// The tests here cover the composition root's channel resolver with the real thing on both sides
// (readiness report 7.1): the real resolver, the real ChannelProxy, a real ipc.Conn over real
// pipes, a real purpose backend, and a real TCP peer. Every layer below already has unit tests,
// and every one of them injects a fake for exactly the seam that did not exist -- which is how the
// bus shipped dead for three stages while staying green. A resolver returning a plausible backend
// that nothing can move a byte through would pass any test that stopped short of this one.

// channelTestPlugin is the far end of the pipe: it speaks frames, tracks its own send window the
// way ADR-011 requires a real plugin to, and records what the host sent it.
type channelTestPlugin struct {
	fw *ipc.FrameWriter
	r  io.Reader

	mu       sync.Mutex
	received map[uint32][]byte
	credit   map[uint32]int
	granted  chan struct{}
}

func newChannelTestPlugin(r io.Reader, w io.Writer) *channelTestPlugin {
	p := &channelTestPlugin{
		fw:       ipc.NewFrameWriter(w),
		r:        r,
		received: make(map[uint32][]byte),
		credit:   make(map[uint32]int),
		granted:  make(chan struct{}, 4096),
	}
	go p.readLoop()
	return p
}

func (p *channelTestPlugin) readLoop() {
	for {
		hdr, payload, err := ipc.ReadFrame(p.r)
		if err != nil {
			return
		}
		switch hdr.Kind {
		case domainplugin.FrameKindBinary:
			p.mu.Lock()
			p.received[hdr.ChannelID] = append(p.received[hdr.ChannelID], payload...)
			p.mu.Unlock()
			// Grant one back so the host's window reopens, as a real consumer would.
			_ = p.grantCredit(hdr.ChannelID, 1)
		case domainplugin.FrameKindCredit:
			p.mu.Lock()
			p.credit[hdr.ChannelID]++
			p.mu.Unlock()
			select {
			case p.granted <- struct{}{}:
			default:
			}
		}
	}
}

func (p *channelTestPlugin) bytesFor(channelID uint32) []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.received[channelID]...)
}

// openWindow seeds the window the host granted this channel at open time.
func (p *channelTestPlugin) openWindow(channelID uint32, purpose string) {
	p.mu.Lock()
	p.credit[channelID] = domainplugin.InitialCredit(purpose)
	p.mu.Unlock()
}

// send writes one data frame, waiting for a grant when the window is shut. A plugin that ignored
// this would be killed by the host, which is the intended contract.
func (p *channelTestPlugin) send(t *testing.T, channelID uint32, payload []byte) {
	t.Helper()
	for {
		p.mu.Lock()
		if p.credit[channelID] > 0 {
			p.credit[channelID]--
			p.mu.Unlock()
			if err := p.fw.Write(domainplugin.FrameKindBinary, channelID, payload); err != nil {
				t.Errorf("channel %d: write frame: %v", channelID, err)
			}
			return
		}
		p.mu.Unlock()
		select {
		case <-p.granted:
		case <-time.After(3 * time.Second):
			t.Errorf("channel %d: the host never granted credit to send; replenishment is broken", channelID)
			return
		}
	}
}

func (p *channelTestPlugin) grantCredit(channelID uint32, n uint32) error {
	payload := make([]byte, 8)
	payload[0] = byte(channelID >> 24)
	payload[1] = byte(channelID >> 16)
	payload[2] = byte(channelID >> 8)
	payload[3] = byte(channelID)
	payload[4] = byte(n >> 24)
	payload[5] = byte(n >> 16)
	payload[6] = byte(n >> 8)
	payload[7] = byte(n)
	return p.fw.Write(domainplugin.FrameKindCredit, channelID, payload)
}

// echoListener is the relay's real peer: it echoes every byte back, so one channel proves both
// directions of the data path.
func echoListener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().String()
}

// relayPlugin declares tcp-relay against a loopback target, which needs both network grants: the
// allowlist cannot name a port chosen at runtime, and 127.0.0.1 is private.
func relayPlugin(id string) domainplugin.InstalledPlugin {
	return domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:      id,
			Name:    "T",
			Version: "1",
			Capabilities: domainplugin.CapabilitySet{
				Network: &domainplugin.NetworkCaps{
					AllowArbitraryOutbound: true,
					AllowPrivateNetworks:   true,
				},
				Session: &domainplugin.SessionCaps{Embed: true},
				Channel: &domainplugin.ChannelCaps{
					Purposes: []string{
						domainplugin.PurposeTCPRelay,
						domainplugin.PurposeUDPRelay,
						domainplugin.PurposeExec,
						domainplugin.PurposeEmbedStream,
					},
					MaxConcurrent: 4,
				},
			},
		},
	}
}

// execHint is the channel.open hint for an exec channel: the request is a JSON document naming a
// declared template index, never a free-form command string.
const execHint = `{"template":0}`

// wiredSessionRegistry is the state the composition root reaches once wireEmbed has pushed the
// registry in. Tests that are not about the wiring ordering itself start from there.
func wiredSessionRegistry() *sessionRegistryHolder {
	h := newSessionRegistryHolder()
	h.set(usecase.NewSessionRegistry())
	return h
}

// execPlugin declares the exec purpose with one argv template. A manifest like this one cannot be
// installed at all without the user granting exec access (D3), which is what the resolver relies on
// when it passes consentGranted.
func execPlugin(id string) domainplugin.InstalledPlugin {
	return domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:      id,
			Name:    "T",
			Version: "1",
			Capabilities: domainplugin.CapabilitySet{
				Channel: &domainplugin.ChannelCaps{
					Purposes:      []string{domainplugin.PurposeExec},
					MaxConcurrent: 4,
					ExecCommands: []domainplugin.ExecCommandTemplate{
						{Argv: []string{"uname", "-a"}},
					},
				},
			},
		},
	}
}

// newChannelSeam builds the real chain the composition root produces: resolver -> proxy -> conn.
func newChannelSeam(t *testing.T, plugin domainplugin.InstalledPlugin) (*capability.ChannelProxy, *ipc.Conn, *channelTestPlugin) {
	t.Helper()
	pluginOutR, pluginOutW := io.Pipe() // plugin -> host
	hostOutR, hostOutW := io.Pipe()     // host -> plugin

	resolve := newChannelResolverFor(nil, nil, nil, wiredSessionRegistry())(plugin, "sess-1")
	proxy := capability.NewChannelProxy(plugin.Manifest.ID, plugin.Manifest.Capabilities.Channel, resolve, nil)

	conn := ipc.NewConn(pluginOutR, hostOutW, nil, nil, 0)
	proxy.AttachDataPathOpener(conn)
	t.Cleanup(func() {
		proxy.CloseAll()
		conn.Close()
		_ = pluginOutW.Close()
		_ = hostOutR.Close()
	})

	return proxy, conn, newChannelTestPlugin(hostOutR, pluginOutW)
}

// openChannel drives ChannelProxy.Open directly and then releases the channel's outbound gate,
// standing in for Conn.handleIncomingRequest which does that after the channel.open reply in
// production. Without the release, a server-speaks-first backend's outbound pump stays blocked.
func openChannel(t *testing.T, proxy *capability.ChannelProxy, conn *ipc.Conn, purpose, hint string) (uint32, error) {
	t.Helper()
	params, _ := json.Marshal(map[string]string{
		"parentSessionId": "sess-1",
		"purpose":         purpose,
		"hint":            hint,
	})
	res, err := proxy.Open(context.Background(), params)
	if err != nil {
		return 0, err
	}
	var opened struct {
		ChannelID uint32 `json:"channelId"`
	}
	if err := json.Unmarshal(res, &opened); err != nil {
		t.Fatalf("channel.open result: %v", err)
	}
	conn.MarkChannelOpened(opened.ChannelID)
	return opened.ChannelID, nil
}

// TestChannelResolverWiresARealTCPRelayEndToEnd is the seam this whole blocker exists for. The
// resolver was a stub returning ErrNotImplemented, so every channel.open was rejected and nothing
// below Stage 6 was reachable at all -- while every backend, the bus, credit and the proxy stayed
// individually green.
//
// It drives InitialCredit x 3 frames in both directions (7.2): a run of InitialCredit frames
// passes with credit replenishment entirely broken, because the window never has to reopen.
func TestChannelResolverWiresARealTCPRelayEndToEnd(t *testing.T) {
	target := echoListener(t)
	proxy, conn, plugin := newChannelSeam(t, relayPlugin("com.test.relay"))

	id, err := openChannel(t, proxy, conn, domainplugin.PurposeTCPRelay, target)
	if err != nil {
		t.Fatalf("channel.open tcp-relay: %v -- the composition root resolved no usable backend", err)
	}
	plugin.openWindow(id, domainplugin.PurposeTCPRelay)

	total := domainplugin.InitialCredit(domainplugin.PurposeTCPRelay) * 3
	var want bytes.Buffer
	for i := 0; i < total; i++ {
		payload := []byte(fmt.Sprintf("frame-%02d;", i))
		want.Write(payload)
		plugin.send(t, id, payload)
	}

	// The relay is a byte stream, so what comes back may be re-chunked; the contract is the bytes
	// and their order, not the framing.
	deadline := time.After(5 * time.Second)
	for {
		got := plugin.bytesFor(id)
		if bytes.Equal(got, want.Bytes()) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("the echoed stream stalled at %d of %d bytes (%q): the relay moved the first "+
				"window and no more", len(got), want.Len(), got)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestChannelResolverBuildsAFreshBackendPerOpen pins the 3.5 rule as a test rather than a comment.
//
// Every backend is stateful and single-use: it stores the resolved target/argv/tunnelId in a field
// during Authorize, keeps its dialed conn in another, and refuses reuse after CloseRemote. A
// resolver handing out a shared instance therefore lets one channel.open overwrite the target
// another has authorized but not yet wired, and the loser silently dials the winner's peer.
//
// The assertion is on instance identity, deliberately, and it is the whole test. Asserting the
// consequence instead -- two concurrent channels landing on the wrong peers -- looks stronger and
// is much weaker: it only fails when Authorize and Wire happen to interleave, so a shared backend
// passes it almost every run. Identity is the rule the resolver actually owes, and it is
// falsifiable every time.
func TestChannelResolverBuildsAFreshBackendPerOpen(t *testing.T) {
	plugin := relayPlugin("com.test.fresh")
	resolve := newChannelResolverFor(nil, nil, nil, wiredSessionRegistry())(plugin, "sess-1")

	// embed-stream belongs in this loop for the same reason the relays do, and with the sharpest
	// consequence: it stores the tunnel id from `hint` during Authorize, so a shared instance sends
	// every tunnel of a multi-tunnel session to whichever one opened last.
	purposes := []string{
		domainplugin.PurposeTCPRelay,
		domainplugin.PurposeUDPRelay,
		domainplugin.PurposeEmbedStream,
	}
	for _, purpose := range purposes {
		// Resolved concurrently: the resolver is called from whichever goroutine serves a
		// channel.open, so a per-purpose instance cached under a mutex would still be a shared one.
		const opens = 8
		got := make([]domainplugin.ChannelPurposeBackend, opens)
		var wg sync.WaitGroup
		for i := 0; i < opens; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				b, err := resolve(purpose)
				if err != nil {
					t.Errorf("resolve %q: %v", purpose, err)
					return
				}
				got[i] = b
			}(i)
		}
		wg.Wait()
		if t.Failed() {
			t.FailNow()
		}

		seen := make(map[domainplugin.ChannelPurposeBackend]int, opens)
		for _, b := range got {
			seen[b]++
		}
		if len(seen) != opens {
			t.Fatalf("%q: %d channel.open calls produced %d distinct backends, want %d -- a shared "+
				"instance lets one channel overwrite another's resolved target",
				purpose, opens, len(seen), opens)
		}
	}
}

// TestConcurrentChannelsStayOnTheirOwnPeers is the consequence of the rule above, end to end: two
// channels opened concurrently, each to its own TCP peer, each driven InitialCredit x 3 frames
// (7.2). It cannot replace the identity assertion (a shared backend usually passes it), but it is
// what proves the resolver's output actually relays per channel rather than merely differing.
func TestConcurrentChannelsStayOnTheirOwnPeers(t *testing.T) {
	targets := []string{echoListener(t), echoListener(t)}
	proxy, conn, plugin := newChannelSeam(t, relayPlugin("com.test.fresh"))

	ids := make([]uint32, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(i int, target string) {
			defer wg.Done()
			id, err := openChannel(t, proxy, conn, domainplugin.PurposeTCPRelay, target)
			if err != nil {
				t.Errorf("channel.open %s: %v", target, err)
				return
			}
			ids[i] = id
		}(i, target)
	}
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}
	if ids[0] == ids[1] {
		t.Fatalf("both channels got id %d", ids[0])
	}

	// Each channel gets its own distinctive payload. A shared backend dials whichever target
	// Authorize resolved last, so both channels' bytes land on one peer and come back on one
	// channel -- the assertion below is per channel, which is what makes that visible.
	total := domainplugin.InitialCredit(domainplugin.PurposeTCPRelay) * 3
	wants := make([]bytes.Buffer, len(ids))
	for i, id := range ids {
		plugin.openWindow(id, domainplugin.PurposeTCPRelay)
		for n := 0; n < total; n++ {
			payload := []byte(fmt.Sprintf("ch%d-%02d;", i, n))
			wants[i].Write(payload)
			plugin.send(t, id, payload)
		}
	}

	deadline := time.After(5 * time.Second)
	for {
		done := true
		for i, id := range ids {
			if !bytes.Equal(plugin.bytesFor(id), wants[i].Bytes()) {
				done = false
			}
		}
		if done {
			return
		}
		select {
		case <-deadline:
			for i, id := range ids {
				if got := plugin.bytesFor(id); !bytes.Equal(got, wants[i].Bytes()) {
					t.Errorf("channel %d echoed %q, want %q -- the two channels are not talking to "+
						"their own peers", id, got, wants[i].Bytes())
				}
			}
			t.FailNow()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestChannelResolverBuildsEmbedStreamOnTheRealSink pins that embed-stream now resolves to a real
// ChannelEmbedBackend built on the production EmbedTunnelService, not to an ErrNotImplemented.
//
// The sink is the whole point: EmbedTunnelService could not be passed to NewChannelEmbedBackend at
// all until it grew SubscribeOutbound (3.6), so the resolver withheld the purpose. Asserting the
// concrete type is deliberate -- it is what distinguishes "wired to the real service" from "wired
// to something that compiles".
func TestChannelResolverBuildsEmbedStreamOnTheRealSink(t *testing.T) {
	sink := usecase.NewEmbedTunnelService(ratelimit.Factory{})
	resolve := newChannelResolverFor(nil, sink, nil, wiredSessionRegistry())(relayPlugin("com.test.embed"), "sess-1")

	first, err := resolve(domainplugin.PurposeEmbedStream)
	if err != nil {
		t.Fatalf("resolve embed-stream: %v -- the composition root still refuses the purpose", err)
	}
	if _, ok := first.(*usecase.ChannelEmbedBackend); !ok {
		t.Fatalf("embed-stream backend is %T, want *usecase.ChannelEmbedBackend", first)
	}
	second, err := resolve(domainplugin.PurposeEmbedStream)
	if err != nil {
		t.Fatalf("resolve embed-stream twice: %v", err)
	}
	if first == second {
		t.Fatal("embed-stream resolved to one shared backend: the second channel.open would " +
			"overwrite the first's tunnelId (3.5)")
	}
}

// TestChannelResolverBuildsExecOnTheRealSessionRegistry pins that exec, which B2 deliberately
// withheld because NewChannelExecBackend's consentGranted had no source at all (3.9), now resolves
// to a real backend. Asserting the concrete type is deliberate: it is what distinguishes "wired to
// the real thing" from "wired to something that compiles".
func TestChannelResolverBuildsExecOnTheRealSessionRegistry(t *testing.T) {
	resolve := newChannelResolverFor(nil, nil, nil, wiredSessionRegistry())(execPlugin("com.test.exec"), "sess-1")

	first, err := resolve(domainplugin.PurposeExec)
	if err != nil {
		t.Fatalf("resolve exec: %v -- the composition root still refuses the purpose", err)
	}
	if _, ok := first.(*usecase.ChannelExecBackend); !ok {
		t.Fatalf("exec backend is %T, want *usecase.ChannelExecBackend", first)
	}
	second, err := resolve(domainplugin.PurposeExec)
	if err != nil {
		t.Fatalf("resolve exec twice: %v", err)
	}
	if first == second {
		t.Fatal("exec resolved to one shared backend: the second channel.open would overwrite the " +
			"first's argv (3.5)")
	}
}

// TestExecChannelIsRefusedWithoutInstallTimeConsent is B6's reason to exist. The exec purpose runs
// commands over the user's already authenticated SSH session, and the only thing standing between a
// plugin and that is an install-time grant the user gave explicitly (D3): PluginManager.Install
// refuses the install outright when a manifest declares exec and the grant is absent, so
// "installed AND declares exec" is what consentGranted must trace to -- never a hardcoded true,
// which would make that gate decorative.
//
// The check runs at both layers on purpose. The proxy refuses a purpose the manifest never
// declared, so a plugin that never went through the exec consent prompt cannot open the channel at
// all; and the backend the resolver built refuses to authorize it independently, so the gate does
// not rest on the proxy's allowlist alone.
func TestExecChannelIsRefusedWithoutInstallTimeConsent(t *testing.T) {
	// Everything an exec channel needs EXCEPT the declared purpose: keeping the argv templates is
	// what makes this test discriminating. A manifest with no templates would fail Authorize on the
	// argv match no matter what consentGranted said, and would go on passing if the resolver
	// hardcoded it to true.
	unconsented := execPlugin("com.test.noexec")
	unconsented.Manifest.Capabilities.Channel.Purposes = []string{domainplugin.PurposeTCPRelay}

	proxy, conn, _ := newChannelSeam(t, unconsented)
	if _, err := openChannel(t, proxy, conn, domainplugin.PurposeExec, execHint); err == nil {
		t.Fatal("channel.open exec succeeded for a plugin that never asked for -- and was never " +
			"granted -- exec consent at install time")
	}

	// And again below the proxy: the backend itself must not authorize an exec channel for a
	// manifest whose install never demanded the exec grant.
	resolve := newChannelResolverFor(nil, nil, nil, wiredSessionRegistry())(unconsented, "sess-1")
	backend, err := resolve(domainplugin.PurposeExec)
	if err != nil {
		return // Refusing to build it at all is a stronger answer than refusing to authorize.
	}
	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", execHint); err == nil {
		t.Fatal("exec backend authorized a channel for a plugin without install-time exec consent")
	}
}

// TestSessionRegistryReachesTheExecBackendAfterItIsWired pins the ordering the holder exists for.
// newPluginRuntime -- and therefore ChannelResolverFor -- is built BEFORE NewSessionManager
// constructs the registry the exec backend needs, so a resolver handed the registry as a value
// would capture nil and every exec channel would fail to find its parent session.
func TestSessionRegistryReachesTheExecBackendAfterItIsWired(t *testing.T) {
	holder := newSessionRegistryHolder()

	// Resolve-time: exactly the order the composition root uses -- no registry yet.
	resolve := newChannelResolverFor(nil, nil, nil, holder)(execPlugin("com.test.exec"), "sess-1")
	if _, err := resolve(domainplugin.PurposeExec); err == nil {
		t.Fatal("exec resolved before the registry was wired: the backend would dereference nil " +
			"on its first frame instead of failing at channel.open")
	}

	registry := usecase.NewSessionRegistry()
	holder.set(registry)

	backend, err := resolve(domainplugin.PurposeExec)
	if err != nil {
		t.Fatalf("resolve exec: %v", err)
	}
	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", execHint); err != nil {
		t.Fatalf("authorize exec: %v", err)
	}
	// No such session is registered, so Wire must fail on the lookup -- which is only reachable at
	// all if the backend found the registry the holder received after the resolver ran.
	err = backend.Wire(context.Background(), nil)
	if err == nil {
		t.Fatal("Wire succeeded without a parent session")
	}
	if err == domainplugin.ErrCapabilityDenied {
		t.Fatal("Wire never reached the session lookup: the backend has no registry")
	}
}

// TestChannelCloseNotifierBindsAfterTheResolverRan pins the ordering hazard the holder exists for.
// ProcessHost calls ChannelResolverFor while the process's Conn is still being built and only
// supplies AttachChannelCloseNotifier afterwards, so a backend that captured the notifier VALUE at
// resolve time would capture nothing and report its close reason into the void — silently, since
// nothing fails when a notification is dropped. Resolution must happen at call time.
func TestChannelCloseNotifierBindsAfterTheResolverRan(t *testing.T) {
	notifiers := newChannelCloseNotifiers()
	plugin := relayPlugin("com.test.notify")

	// Resolve-time: exactly the order newConn uses — the notifier does not exist yet.
	notify := notifiers.notifierFor(plugin.Manifest.ID, "sess-1")

	// A close reported before the Conn exists must be harmless, not a panic (7.5).
	notify(3, "ws-buffer-full", "no conn yet")

	type call struct {
		id              uint32
		reason, message string
	}
	got := make(chan call, 1)
	notifiers.attach(plugin, "sess-1", func(id uint32, reason, message string) {
		got <- call{id, reason, message}
	})

	notify(7, "ws-buffer-full", "consumer gone")
	select {
	case c := <-got:
		if c.id != 7 || c.reason != "ws-buffer-full" || c.message != "consumer gone" {
			t.Fatalf("notify = %+v, want {7 ws-buffer-full consumer gone}", c)
		}
	default:
		t.Fatal("close reason never reached the process's notifier — it was bound at resolve time, when it did not exist")
	}
}
