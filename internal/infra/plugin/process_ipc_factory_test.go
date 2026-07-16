package plugin

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

// wiredChannelBackend accepts anything and records the handle it was wired with, so the test can
// assert the handle carried a live data path rather than merely existing.
type wiredChannelBackend struct {
	handle *domainplugin.ChannelHandle
}

func (b *wiredChannelBackend) Authorize(string, string, string) error { return nil }

func (b *wiredChannelBackend) Wire(_ context.Context, ch *domainplugin.ChannelHandle) error {
	b.handle = ch
	return nil
}

func (b *wiredChannelBackend) CloseRemote() error { return nil }

// TestNewConnWiresChannelProxyToTheBus covers the composition root itself — the one place that
// can fail in a way no unit test on either side would notice.
//
// The proxy and the conn are mutually dependent (the conn's request handler routes channel.open
// into the proxy), so the opener cannot be passed to the constructor and is attached afterwards.
// An attachment that is simply forgotten leaves every layer below correct and individually
// green, while channel.open hands the plugin an id wired to nothing. That was the original
// defect, in exactly this file.
func TestNewConnWiresChannelProxyToTheBus(t *testing.T) {
	backend := &wiredChannelBackend{}
	host := NewProcessHost(HostConfig{
		DataRoot: t.TempDir(),
		ChannelResolver: func(string) (domainplugin.ChannelPurposeBackend, error) {
			return backend, nil
		},
	})

	plugin := domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:      "com.test",
			Name:    "T",
			Version: "1",
			Capabilities: domainplugin.CapabilitySet{
				Channel: &domainplugin.ChannelCaps{Purposes: []string{domainplugin.PurposeExec}},
			},
		},
	}

	_, pluginOutW := io.Pipe()
	hostOutR, _ := io.Pipe()
	t.Cleanup(func() {
		_ = pluginOutW.Close()
		_ = hostOutR.Close()
	})

	conn, _, _, _, channelProxy, err := host.newConn(plugin, t.TempDir(), "sess-1", hostOutR, pluginOutW, domainplugin.NegotiatedDescriptor{})
	if err != nil {
		t.Fatalf("newConn: %v", err)
	}
	t.Cleanup(conn.Close)

	params, _ := json.Marshal(map[string]string{"parentSessionId": "sess-1", "purpose": domainplugin.PurposeExec})
	if _, err := channelProxy.Open(context.Background(), params); err != nil {
		t.Fatalf("channel.open through the composition root failed: %v — the proxy was never "+
			"attached to a data path opener", err)
	}

	if backend.handle == nil {
		t.Fatal("backend was never wired")
	}
	if backend.handle.Data() == nil {
		t.Fatal("the handle reached the backend with no data path: the channel can move no bytes")
	}
}

// TestChannelThroughputKbpsResolvesManifestOrDefault: the manifest field is only meaningful if
// the composition root actually reads it. It went unread entirely, so a declared bandwidth cap
// the user consented to at install time did nothing.
func TestChannelThroughputKbpsResolvesManifestOrDefault(t *testing.T) {
	if got := channelThroughputKbps(nil); got != domainplugin.DefaultChannelThroughputKbps {
		t.Fatalf("nil caps = %d, want host default %d", got, domainplugin.DefaultChannelThroughputKbps)
	}
	if got := channelThroughputKbps(&domainplugin.ChannelCaps{}); got != domainplugin.DefaultChannelThroughputKbps {
		t.Fatalf("unset maxThroughputKbps = %d, want host default %d", got, domainplugin.DefaultChannelThroughputKbps)
	}
	if got := channelThroughputKbps(&domainplugin.ChannelCaps{MaxThroughputKbps: 64}); got != 64 {
		t.Fatalf("declared maxThroughputKbps = %d, want 64", got)
	}
}
