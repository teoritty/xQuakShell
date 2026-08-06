package plugin

import (
	"path/filepath"
	"strings"
)

// DeclaredUIAssets returns every bundle-relative path under ui/ that the manifest promises to
// ship: view entries, the embed entry of each connection protocol, and discovery icons. Paths
// are slash-separated, deduplicated, and returned in declaration order.
//
// The defaults applied here mirror the ones validation applies, and they have to stay in step:
// validateViewEntries defaults an empty view entry to ui/index.html, and
// validateConnectionProtocolCaps defaults an empty embedEntry to ui/embed.html but only under
// capabilities.session.embed. Defaulting the embed entry unconditionally would claim a file from
// every plugin that merely contributes a connection protocol, which is a form the manifest never
// promised and the bundle has no reason to contain.
func (m *Manifest) DeclaredUIAssets() []string {
	var assets []string
	seen := make(map[string]struct{})
	add := func(entry string) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return
		}
		entry = filepath.ToSlash(entry)
		if _, ok := seen[entry]; ok {
			return
		}
		seen[entry] = struct{}{}
		assets = append(assets, entry)
	}

	for _, v := range m.Contributions.Views {
		entry := strings.TrimSpace(v.Entry)
		if entry == "" {
			entry = defaultViewEntry
		}
		add(entry)
	}
	if m.Capabilities.Session != nil && m.Capabilities.Session.Embed {
		for _, cp := range m.Contributions.ConnectionProtocols {
			entry := strings.TrimSpace(cp.EmbedEntry)
			if entry == "" {
				entry = defaultEmbedEntry
			}
			add(entry)
		}
	}
	for _, icon := range m.Contributions.DiscoveryIcons {
		add(icon.Asset)
	}
	return assets
}

// DeclaresUIAssets reports whether the plugin ships files under ui/. A plugin for which this is
// true cannot be installed from a bare release binary: that asset carries no ui/ tree, so every
// path above is missing on disk and the panel the plugin opens is a 404 with nothing on screen
// to explain it.
//
// capabilities.ui is deliberately not part of this: surfaces, dialogs and node details are drawn
// by the host from data the plugin sends over IPC, so a plugin can declare them and ship no
// assets at all. Treating that capability as an asset promise would refuse installs that are
// perfectly correct.
func (m *Manifest) DeclaresUIAssets() bool {
	return len(m.DeclaredUIAssets()) > 0
}
