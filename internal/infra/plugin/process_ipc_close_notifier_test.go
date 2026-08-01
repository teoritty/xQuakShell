package plugin

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/capability"
	"xquakshell/internal/infra/plugin/ipc"
)


// notifyingBackend raises a terminal close the way a real backend does: from Wire, naming the id
// it was wired with. Nothing about it is a fake notifier — the func it calls is whatever the
// composition root handed the attach seam.
type notifyingBackend struct {
	wiredChannelBackend
	notify func(channelID uint32, reason, message string)
	fired  chan struct{}
}

func (b *notifyingBackend) Wire(ctx context.Context, ch *domainplugin.ChannelHandle) error {
	if err := b.wiredChannelBackend.Wire(ctx, ch); err != nil {
		return err
	}
	go func() {
		b.notify(ch.ChannelID(), "exit", "process exited")
		close(b.fired)
	}()
	return nil
}

// readCloseNotification drains frames the host wrote to the plugin until a channel.close
// notification arrives, decoding it from the real wire rather than from a recorded call.
func readCloseNotification(t *testing.T, r io.Reader) (uint32, string, string) {
	t.Helper()
	type closeParams struct {
		ChannelID uint32 `json:"channelId"`
		Reason    string `json:"reason"`
		Message   string `json:"message"`
	}
	for {
		hdr, payload, err := ipc.ReadFrame(r)
		if err != nil {
			t.Fatalf("reading the plugin's stdin: %v — no channel.close ever reached the wire", err)
		}
		if hdr.Kind != domainplugin.FrameKindJSONRPC {
			continue
		}
		var msg struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(payload, &msg); err != nil || msg.Method != "channel.close" {
			continue
		}
		var p closeParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			t.Fatalf("channel.close params: %v", err)
		}
		return p.ChannelID, p.Reason, p.Message
	}
}

// TestNewConnDeliversBackendCloseToThePluginConn is B8's seam (§7.1). A unit test against a fake
// notifier proves only that a backend calls a func — which is exactly the class of test that let
// ChannelCloseNotifier sit with no supplier at all while every layer stayed green. This drives a
// real backend, through the real composition root, onto a real Conn, and reads the notification
// off the wire the plugin actually gets.
//
// The notifier binds once per plugin process (it closes over that process's Conn.Notify), so the
// only thing that can name the channel is the id the backend supplies — the assertion below is on
// that id, not merely on a notification arriving.
func TestNewConnDeliversBackendCloseToThePluginConn(t *testing.T) {
	backend := &notifyingBackend{fired: make(chan struct{})}

	// The resolver and the attach come back from ONE factory call, so the process a notifier
	// belongs to is fixed by construction rather than by a key looked up later.
	var resolvedPlugin string
	var resolvedSession string
	host := NewProcessHost(HostConfig{
		DataRoot: t.TempDir(),
		ChannelResolverFor: func(p domainplugin.InstalledPlugin, sessionID string) (capability.ChannelBackendResolver, AttachChannelCloseNotify) {
			resolvedPlugin = p.Manifest.ID
			resolvedSession = sessionID
			return func(string) (domainplugin.ChannelPurposeBackend, error) {
					return backend, nil
				}, func(notify ChannelCloseNotify) {
					backend.notify = notify
				}
		},
	})

	plugin := domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:      "com.test.closer",
			Name:    "T",
			Version: "1",
			Capabilities: domainplugin.CapabilitySet{
				Channel: &domainplugin.ChannelCaps{Purposes: []string{domainplugin.PurposeExec}},
			},
		},
	}

	// hostInR is what the host reads (the plugin says nothing here); hostOutR is the plugin's
	// stdin, where a real plugin would see this notification arrive.
	hostInR, _ := io.Pipe()
	hostOutR, hostOutW := io.Pipe()
	t.Cleanup(func() {
		_ = hostOutW.Close()
		_ = hostInR.Close()
	})

	conn, _, _, _, channelProxy, err := host.newConn(plugin, t.TempDir(), "sess-9", hostInR, hostOutW, domainplugin.NegotiatedDescriptor{})
	if err != nil {
		t.Fatalf("newConn: %v", err)
	}
	t.Cleanup(conn.Close)

	if backend.notify == nil {
		t.Fatal("newConn never attached a close notifier: a backend has no way to reach this " +
			"process's Conn, so channel.close can never be raised")
	}
	if resolvedPlugin != plugin.Manifest.ID || resolvedSession != "sess-9" {
		t.Fatalf("channel wiring built for (%q, %q), want (%q, %q)", resolvedPlugin, resolvedSession,
			plugin.Manifest.ID, "sess-9")
	}

	// The pipe is synchronous: the host's write blocks until the plugin reads, so the reader must
	// be running before anything raises the close.
	type got struct {
		id               uint32
		reason, message  string
	}
	received := make(chan got, 1)
	go func() {
		id, reason, message := readCloseNotification(t, hostOutR)
		received <- got{id, reason, message}
	}()

	params, _ := json.Marshal(map[string]string{"parentSessionId": "sess-9", "purpose": domainplugin.PurposeExec})
	openRes, err := channelProxy.Open(context.Background(), params)
	if err != nil {
		t.Fatalf("channel.open: %v", err)
	}
	var opened struct {
		ChannelID uint32 `json:"channelId"`
	}
	if err := json.Unmarshal(openRes, &opened); err != nil {
		t.Fatalf("channel.opened result: %v", err)
	}

	select {
	case g := <-received:
		if g.id != opened.ChannelID {
			t.Fatalf("channel.close named channel %d, want %d — the plugin cannot tell which "+
				"channel died", g.id, opened.ChannelID)
		}
		if g.reason != "exit" || g.message != "process exited" {
			t.Fatalf("channel.close = {%q, %q}, want {exit, process exited}", g.reason, g.message)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no channel.close reached the plugin conn")
	}
}

// TestChannelCloseNotifierForAForgottenChannelIsHarmless pins §7.5 at this seam. An unknown
// channel id is fatal to the connection by ADR-011 §2a, and a close notification is the one place
// that rule could be reintroduced backwards: the host raises it for a channel the plugin may have
// closed and forgotten already. It must be an ordinary notification, never a failed conn.
func TestChannelCloseNotifierForAForgottenChannelIsHarmless(t *testing.T) {
	var notify ChannelCloseNotify
	host := NewProcessHost(HostConfig{
		DataRoot: t.TempDir(),
		ChannelResolverFor: func(domainplugin.InstalledPlugin, string) (capability.ChannelBackendResolver, AttachChannelCloseNotify) {
			return nil, func(n ChannelCloseNotify) { notify = n }
		},
	})

	plugin := domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{ID: "com.test.forgot", Name: "T", Version: "1"},
	}

	hostInR, _ := io.Pipe()
	hostOutR, hostOutW := io.Pipe()
	t.Cleanup(func() {
		_ = hostOutW.Close()
		_ = hostInR.Close()
	})

	conn, _, _, _, _, err := host.newConn(plugin, t.TempDir(), "sess-1", hostInR, hostOutW, domainplugin.NegotiatedDescriptor{})
	if err != nil {
		t.Fatalf("newConn: %v", err)
	}
	t.Cleanup(conn.Close)
	if notify == nil {
		t.Fatal("newConn never attached a close notifier")
	}

	done := make(chan struct{})
	go func() {
		id, _, _ := readCloseNotification(t, hostOutR)
		if id != 4242 {
			t.Errorf("channel.close named %d, want 4242", id)
		}
		close(done)
	}()

	// 4242 was never opened on this conn — the plugin has no such channel.
	notify(4242, "tunnel-closed", "the tunnel went away")

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the notification for an unopened channel never reached the wire")
	}
	if err := conn.ReadError(); err != nil {
		t.Fatalf("the conn failed after a close for an unknown channel: %v — a notification "+
			"must never be the thing that kills a well-behaved plugin", err)
	}
}
