package plugin

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/capability"
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
		ChannelResolverFor: func(domainplugin.InstalledPlugin, string) capability.ChannelBackendResolver {
			return func(string) (domainplugin.ChannelPurposeBackend, error) {
				return backend, nil
			}
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

// manifestScopedBackend is resolved by a resolver that closed over the plugin it was built for.
// It records what that resolver knew, which is the whole point of the factory: a backend cannot
// be constructed from a purpose string alone (exec needs the manifest's execCommands, relay needs
// its network caps, embed needs the plugin id).
type manifestScopedBackend struct {
	wiredChannelBackend
	pluginID  string
	sessionID string
	purpose   string
}

// TestNewConnResolvesChannelBackendsPerPluginProcess covers the seam B1 opens: the composition
// root supplies a factory of resolvers, and newConn — the one place that holds this process's
// manifest and sessionID — is what turns it into the resolver the proxy calls.
//
// A factory invoked with the wrong plugin, or a call site that keeps resolving by purpose alone,
// leaves every layer green while every real backend is unconstructible. The assertions below are
// on what the resolver *knew*, not merely that it ran.
func TestNewConnResolvesChannelBackendsPerPluginProcess(t *testing.T) {
	var built []*manifestScopedBackend
	host := NewProcessHost(HostConfig{
		DataRoot: t.TempDir(),
		ChannelResolverFor: func(p domainplugin.InstalledPlugin, sessionID string) capability.ChannelBackendResolver {
			return func(purpose string) (domainplugin.ChannelPurposeBackend, error) {
				b := &manifestScopedBackend{
					pluginID:  p.Manifest.ID,
					sessionID: sessionID,
					purpose:   purpose,
				}
				built = append(built, b)
				return b, nil
			}
		},
	})

	purposes := []string{domainplugin.PurposeExec, domainplugin.PurposeTCPRelay}
	plugin := domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:      "com.test.scoped",
			Name:    "T",
			Version: "1",
			Capabilities: domainplugin.CapabilitySet{
				Channel: &domainplugin.ChannelCaps{Purposes: purposes},
			},
		},
	}

	_, pluginOutW := io.Pipe()
	hostOutR, _ := io.Pipe()
	t.Cleanup(func() {
		_ = pluginOutW.Close()
		_ = hostOutR.Close()
	})

	conn, _, _, _, channelProxy, err := host.newConn(plugin, t.TempDir(), "sess-42", hostOutR, pluginOutW, domainplugin.NegotiatedDescriptor{})
	if err != nil {
		t.Fatalf("newConn: %v", err)
	}
	t.Cleanup(conn.Close)

	for _, purpose := range purposes {
		params, _ := json.Marshal(map[string]string{"parentSessionId": "sess-42", "purpose": purpose})
		if _, err := channelProxy.Open(context.Background(), params); err != nil {
			t.Fatalf("channel.open %q: %v", purpose, err)
		}
	}

	if len(built) != len(purposes) {
		t.Fatalf("resolver produced %d backends, want one per declared purpose (%d)", len(built), len(purposes))
	}
	for i, b := range built {
		if b.purpose != purposes[i] {
			t.Fatalf("backend %d resolved for purpose %q, want %q", i, b.purpose, purposes[i])
		}
		if b.pluginID != plugin.Manifest.ID {
			t.Fatalf("resolver for purpose %q knew plugin %q, want %q — a backend cannot be built "+
				"without the manifest it is authorized against", b.purpose, b.pluginID, plugin.Manifest.ID)
		}
		if b.sessionID != "sess-42" {
			t.Fatalf("resolver for purpose %q knew session %q, want %q", b.purpose, b.sessionID, "sess-42")
		}
		if b.handle == nil || b.handle.Data() == nil {
			t.Fatalf("backend for purpose %q reached no wired data path", b.purpose)
		}
	}
	// The resolver must hand out a fresh instance per channel.open (§3.5): the backends are
	// stateful and a shared one corrupts concurrent channels. B2 constructs the real backends
	// that make this consequential; here it is the factory's contract that is pinned.
	if built[0] == built[1] {
		t.Fatal("both channels share one backend instance")
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
