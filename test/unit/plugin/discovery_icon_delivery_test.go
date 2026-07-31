package plugin_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	infrapluginassets "xquakshell/internal/infra/plugin/assets"
	presentation "xquakshell/internal/presentation/wails"
	"xquakshell/internal/usecase"
)

// Icon delivery, end to end: a bundle on disk -> the registry -> ListPlugins.
//
// ADR-014 gives icons no endpoint of their own. They ride along with the plugin list as base64 data
// URIs, and the frontend renders them strictly as <img src="data:...">, because inlining an SVG
// would execute scripts from the plugin's bundle inside the main window. These tests hold that
// shape: the right media type per extension, base64 rather than percent-encoding, and — the part
// that matters when a bundle is imperfect — one bad asset costing exactly one icon.

// installedIconPlugin writes a plugin whose ui/icons directory contains the given files (name ->
// contents) and returns it registered-ready. A nil content marks a file that is NOT created, i.e.
// an asset the manifest declares but the bundle does not ship.
func installedIconPlugin(t *testing.T, pluginID string, files map[string][]byte) domainplugin.InstalledPlugin {
	t.Helper()
	root := t.TempDir()
	iconDir := filepath.Join(root, "ui", "icons")
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		t.Fatal(err)
	}
	icons := make([]domainplugin.DiscoveryIconContribution, 0, len(files))
	for name, content := range files {
		id := strings.TrimSuffix(name, filepath.Ext(name))
		icons = append(icons, domainplugin.DiscoveryIconContribution{ID: id, Asset: "ui/icons/" + name})
		if content == nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(iconDir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:      pluginID,
			Name:    pluginID,
			Version: "1.0.0",
			Capabilities: domainplugin.CapabilitySet{
				Discovery: &domainplugin.DiscoveryCaps{ParentProtocols: []string{"ssh"}},
			},
			Contributions: domainplugin.Contributions{DiscoveryIcons: icons},
		},
		RootDir: root,
		Source:  domainplugin.SourceUser,
	}
}

// listPluginsWithIcons runs the real path the frontend uses: registry -> manager -> AppAPI.
func listPluginsWithIcons(t *testing.T, plugins ...domainplugin.InstalledPlugin) []presentation.PluginDTO {
	t.Helper()
	registry := usecase.NewPluginRegistry()
	registry.SetDiscoveryIconAssetReader(infrapluginassets.DiscoveryIconReader{})
	if err := registry.Load(plugins); err != nil {
		t.Fatalf("registry load: %v", err)
	}
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{Registry: registry, InstallRoot: t.TempDir()})
	api := &presentation.AppAPI{}
	api.SetPluginManager(manager)

	list, err := api.ListPlugins()
	if err != nil {
		t.Fatalf("ListPlugins: %v", err)
	}
	return list
}

func iconsOf(t *testing.T, list []presentation.PluginDTO, pluginID string) map[string]string {
	t.Helper()
	for _, dto := range list {
		if dto.ID == pluginID {
			return dto.DiscoveryIcons
		}
	}
	t.Fatalf("plugin %q missing from ListPlugins", pluginID)
	return nil
}

// TestDiscoveryIconsReachTheFrontendAsBase64DataURIs pins the media type per allowed extension and
// the encoding. The bytes are decoded back and compared, so an encoder that produced a well-formed
// but wrong URI cannot pass on the prefix alone.
func TestDiscoveryIconsReachTheFrontendAsBase64DataURIs(t *testing.T) {
	// Deliberately awkward payloads: an SVG carrying the quotes and angle brackets that make
	// percent-encoding unsafe in an attribute, and binary bytes for the raster formats.
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect fill="#fff"/></svg>`)
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0xff}
	ico := []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10}

	list := listPluginsWithIcons(t, installedIconPlugin(t, "com.example.icons", map[string][]byte{
		"vector.svg": svg,
		"raster.png": png,
		"legacy.ico": ico,
	}))
	icons := iconsOf(t, list, "com.example.icons")

	cases := []struct {
		id      string
		mime    string
		content []byte
	}{
		{"vector", "image/svg+xml", svg},
		{"raster", "image/png", png},
		{"legacy", "image/x-icon", ico},
	}
	for _, tc := range cases {
		uri, ok := icons[tc.id]
		if !ok {
			t.Fatalf("icon %q missing from %v", tc.id, icons)
		}
		prefix := "data:" + tc.mime + ";base64,"
		if !strings.HasPrefix(uri, prefix) {
			t.Fatalf("icon %q: want prefix %q, got %q", tc.id, prefix, uri)
		}
		if strings.Contains(uri, "%") {
			t.Fatalf("icon %q is percent-encoded, must be base64: %q", tc.id, uri)
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, prefix))
		if err != nil {
			t.Fatalf("icon %q payload is not valid base64: %v", tc.id, err)
		}
		if string(decoded) != string(tc.content) {
			t.Fatalf("icon %q round-trip mismatch", tc.id)
		}
	}
}

// TestUnreadableDiscoveryIconCostsOnlyThatIcon is the failure mode that matters. A bundle can ship
// a manifest that names an asset it does not contain, or a path that is not a readable file at all.
// Neither may take down the plugin or ListPlugins — the frontend would then lose every plugin in
// the settings list because one glyph was missing.
func TestUnreadableDiscoveryIconCostsOnlyThatIcon(t *testing.T) {
	plugin := installedIconPlugin(t, "com.example.broken", map[string][]byte{
		"good.svg":    []byte("<svg/>"),
		"missing.png": nil,
		"empty.svg":   {},
	})
	// A directory where a file is declared: readable as a path, unreadable as an icon.
	if err := os.MkdirAll(filepath.Join(plugin.RootDir, "ui", "icons", "directory.svg"), 0o755); err != nil {
		t.Fatal(err)
	}
	plugin.Manifest.Contributions.DiscoveryIcons = append(plugin.Manifest.Contributions.DiscoveryIcons,
		domainplugin.DiscoveryIconContribution{ID: "directory", Asset: "ui/icons/directory.svg"})

	list := listPluginsWithIcons(t, plugin)
	if len(list) != 1 {
		t.Fatalf("a broken icon must not remove the plugin from ListPlugins, got %d entries", len(list))
	}
	icons := iconsOf(t, list, "com.example.broken")
	if _, ok := icons["good"]; !ok {
		t.Fatalf("the readable icon must survive its broken neighbours, got %v", icons)
	}
	for _, id := range []string{"missing", "empty", "directory"} {
		if uri, present := icons[id]; present {
			t.Fatalf("unreadable icon %q must be absent, got %q", id, uri)
		}
	}
}

// TestPluginWithoutDiscoveryIconsCarriesNone keeps the field off the wire for the overwhelming
// majority of plugins, which declare no icons at all.
func TestPluginWithoutDiscoveryIconsCarriesNone(t *testing.T) {
	list := listPluginsWithIcons(t, installedIconPlugin(t, "com.example.plain", nil))
	if icons := iconsOf(t, list, "com.example.plain"); len(icons) != 0 {
		t.Fatalf("expected no icons, got %v", icons)
	}
}

// TestDiscoveryIconAssetOutsideThePluginRootIsRefused: asset paths are confined to the bundle at
// install time, but this reader takes a plugin-authored string and joins it to a directory. A
// traversal must produce no icon rather than an arbitrary file encoded as one.
func TestDiscoveryIconAssetOutsideThePluginRootIsRefused(t *testing.T) {
	plugin := installedIconPlugin(t, "com.example.escape", map[string][]byte{"good.svg": []byte("<svg/>")})
	outside := filepath.Join(filepath.Dir(plugin.RootDir), "outside.svg")
	if err := os.WriteFile(outside, []byte("<svg>secret</svg>"), 0o644); err != nil {
		t.Fatal(err)
	}
	plugin.Manifest.Contributions.DiscoveryIcons = append(plugin.Manifest.Contributions.DiscoveryIcons,
		domainplugin.DiscoveryIconContribution{ID: "escape", Asset: "../outside.svg"})

	icons := iconsOf(t, listPluginsWithIcons(t, plugin), "com.example.escape")
	if uri, present := icons["escape"]; present {
		t.Fatalf("asset outside the plugin root must not be delivered, got %q", uri)
	}
	if _, ok := icons["good"]; !ok {
		t.Fatalf("the legitimate icon must still be delivered, got %v", icons)
	}
}

// TestDiscoveryIconMIMEIsDrivenByExtension checks the pure mapping directly, including the case a
// manifest can produce on a case-insensitive filesystem and the extensions that are not allowed at
// all — those must yield no media type rather than a guessed one the browser would sniff.
func TestDiscoveryIconMIMEIsDrivenByExtension(t *testing.T) {
	allowed := map[string]string{
		"ui/icons/a.svg": "image/svg+xml",
		"ui/icons/a.PNG": "image/png",
		"ui/icons/a.Ico": "image/x-icon",
	}
	for asset, want := range allowed {
		if got := domainplugin.DiscoveryIconMIME(asset); got != want {
			t.Fatalf("%s: want %q, got %q", asset, want, got)
		}
	}
	for _, asset := range []string{"ui/icons/a.html", "ui/icons/a.js", "ui/icons/a", "ui/icons/a.svgz"} {
		if got := domainplugin.DiscoveryIconMIME(asset); got != "" {
			t.Fatalf("%s must have no media type, got %q", asset, got)
		}
		if _, ok := domainplugin.DiscoveryIconDataURI(asset, []byte("x")); ok {
			t.Fatalf("%s must not produce a data URI", asset)
		}
	}
}
