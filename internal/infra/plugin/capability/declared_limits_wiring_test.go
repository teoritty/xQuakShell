package capability

import (
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

// The clamp is only worth anything if it reaches the counter the proxy actually compares against.
// Each of these constructors used to read the manifest number straight into that field, so this is
// the assertion that the resolver is not merely present but wired in.
func TestChannelProxyClampsADeclaredConcurrencyLimit(t *testing.T) {
	proxy := NewChannelProxy("com.example.plugin", &domainplugin.ChannelCaps{
		MaxConcurrent: 1_000_000,
	}, nil, nil)

	if proxy.max != domainplugin.MaxAllowedConcurrentChannels {
		t.Errorf("channel proxy max = %d, want the ceiling %d; the manifest set its own limit",
			proxy.max, domainplugin.MaxAllowedConcurrentChannels)
	}
}

func TestChannelProxyHonoursASmallerDeclaredLimit(t *testing.T) {
	proxy := NewChannelProxy("com.example.plugin", &domainplugin.ChannelCaps{MaxConcurrent: 2}, nil, nil)

	if proxy.max != 2 {
		t.Errorf("channel proxy max = %d, want 2; a self-imposed smaller limit must survive", proxy.max)
	}
}

func TestChannelProxyFallsBackToTheHostDefault(t *testing.T) {
	proxy := NewChannelProxy("com.example.plugin", nil, nil, nil)

	if proxy.max != domainplugin.DefaultMaxConcurrentChannels {
		t.Errorf("channel proxy max = %d, want the host default %d", proxy.max, domainplugin.DefaultMaxConcurrentChannels)
	}
}

// Each tunnel channel is a live SSH channel on the user's own connection, so this one spends a
// resource shared with the session the user is working in.
func TestTunnelDialProxyClampsADeclaredChannelLimit(t *testing.T) {
	proxy := NewTunnelDialProxy("p1", &domainplugin.TunnelCaps{
		MaxConcurrentChannels: 1_000_000,
	}, nil)

	if proxy.max != domainplugin.MaxAllowedTunnelChannels {
		t.Errorf("tunnel proxy max = %d, want the ceiling %d", proxy.max, domainplugin.MaxAllowedTunnelChannels)
	}
}

func TestTunnelDialProxyHonoursASmallerDeclaredLimit(t *testing.T) {
	proxy := NewTunnelDialProxy("p1", &domainplugin.TunnelCaps{MaxConcurrentChannels: 3}, nil)

	if proxy.max != 3 {
		t.Errorf("tunnel proxy max = %d, want 3", proxy.max)
	}
}
