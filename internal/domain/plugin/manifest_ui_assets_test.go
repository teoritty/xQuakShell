package plugin_test

import (
	"reflect"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func TestDeclaredUIAssets(t *testing.T) {
	cases := []struct {
		name     string
		manifest domainplugin.Manifest
		want     []string
	}{
		{
			name:     "no contributions declares nothing",
			manifest: domainplugin.Manifest{},
			want:     nil,
		},
		{
			name: "view entry is taken verbatim",
			manifest: domainplugin.Manifest{
				Contributions: domainplugin.Contributions{
					Views: []domainplugin.ViewContribution{{ID: "panel", Entry: "ui/panel.html"}},
				},
			},
			want: []string{"ui/panel.html"},
		},
		{
			name: "empty view entry defaults to ui/index.html",
			manifest: domainplugin.Manifest{
				Contributions: domainplugin.Contributions{
					Views: []domainplugin.ViewContribution{{ID: "panel"}},
				},
			},
			want: []string{"ui/index.html"},
		},
		{
			name: "embed entry counts when session.embed is granted",
			manifest: domainplugin.Manifest{
				Capabilities: domainplugin.CapabilitySet{
					Session: &domainplugin.SessionCaps{Embed: true},
				},
				Contributions: domainplugin.Contributions{
					ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{
						{ID: "vnc", EmbedEntry: "ui/vnc.html"},
					},
				},
			},
			want: []string{"ui/vnc.html"},
		},
		{
			name: "empty embed entry defaults to ui/embed.html",
			manifest: domainplugin.Manifest{
				Capabilities: domainplugin.CapabilitySet{
					Session: &domainplugin.SessionCaps{Embed: true},
				},
				Contributions: domainplugin.Contributions{
					ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{{ID: "vnc"}},
				},
			},
			want: []string{"ui/embed.html"},
		},
		{
			name: "embed entry is not claimed without session.embed",
			manifest: domainplugin.Manifest{
				Capabilities: domainplugin.CapabilitySet{
					Session: &domainplugin.SessionCaps{Terminal: true},
				},
				Contributions: domainplugin.Contributions{
					ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{
						{ID: "telnet", EmbedEntry: "ui/telnet.html"},
					},
				},
			},
			want: nil,
		},
		{
			name: "discovery icons count",
			manifest: domainplugin.Manifest{
				Contributions: domainplugin.Contributions{
					DiscoveryIcons: []domainplugin.DiscoveryIconContribution{
						{ID: "docker", Asset: "ui/icons/docker.svg"},
					},
				},
			},
			want: []string{"ui/icons/docker.svg"},
		},
		{
			name: "ui capability alone declares no assets",
			manifest: domainplugin.Manifest{
				Capabilities: domainplugin.CapabilitySet{
					UI: &domainplugin.UICaps{Surfaces: []string{"log"}, Dialogs: true},
				},
			},
			want: nil,
		},
		{
			name: "repeated entries are reported once",
			manifest: domainplugin.Manifest{
				Contributions: domainplugin.Contributions{
					Views: []domainplugin.ViewContribution{
						{ID: "a", Entry: "ui/index.html"},
						{ID: "b"},
						{ID: "c", Entry: " ui/index.html "},
					},
				},
			},
			want: []string{"ui/index.html"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.manifest.DeclaredUIAssets()
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("DeclaredUIAssets() = %v, want %v", got, tc.want)
			}
			if want := len(tc.want) > 0; tc.manifest.DeclaresUIAssets() != want {
				t.Fatalf("DeclaresUIAssets() = %v, want %v", !want, want)
			}
		})
	}
}
