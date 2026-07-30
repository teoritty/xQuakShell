package plugin_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/bundle"
)

// iconManifest builds a manifest declaring the named icon assets under ui/icons.
func iconManifest(assets ...string) domainplugin.Manifest {
	icons := make([]domainplugin.DiscoveryIconContribution, 0, len(assets))
	for _, asset := range assets {
		id := strings.TrimSuffix(filepath.Base(asset), filepath.Ext(asset))
		icons = append(icons, domainplugin.DiscoveryIconContribution{ID: id, Asset: asset})
	}
	return domainplugin.Manifest{
		ID:      "com.example.icons",
		Name:    "Icons",
		Version: "1.0.0",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Discovery: &domainplugin.DiscoveryCaps{ParentProtocols: []string{"ssh"}},
		},
		Contributions: domainplugin.Contributions{DiscoveryIcons: icons},
	}
}

// writeIcon creates an asset of exactly size bytes under installDir and returns its manifest-
// relative path.
func writeIcon(t *testing.T, installDir, name string, size int) string {
	t.Helper()
	rel := "ui/icons/" + name
	full := filepath.Join(installDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	return rel
}

// TestOversizedDiscoveryIconRefusesThePlugin pins the per-asset budget and, just as importantly,
// the error message: an author told only "plugin too large" has 64 files to search, while one told
// which asset and by how much has one file to shrink.
func TestOversizedDiscoveryIconRefusesThePlugin(t *testing.T) {
	installDir := t.TempDir()
	small := writeIcon(t, installDir, "ok.svg", 1024)
	big := writeIcon(t, installDir, "huge.svg", int(domainplugin.MaxDiscoveryIconBytes)+1)
	m := iconManifest(small, big)

	err := bundle.ValidateCapabilitiesForInstall(&m, installDir)
	if err == nil {
		t.Fatal("an icon over 64 KiB must refuse the plugin, not be silently dropped")
	}
	if !strings.Contains(err.Error(), "huge.svg") {
		t.Fatalf("the error must name the offending asset, got %v", err)
	}
	if strings.Contains(err.Error(), "ok.svg") {
		t.Fatalf("the error must name only the offending asset, got %v", err)
	}
}

// TestExactlyMaxSizedDiscoveryIconIsAccepted pins the boundary so "> limit" cannot drift into
// ">= limit" and start refusing plugins that are exactly at the documented budget.
func TestExactlyMaxSizedDiscoveryIconIsAccepted(t *testing.T) {
	installDir := t.TempDir()
	asset := writeIcon(t, installDir, "edge.svg", int(domainplugin.MaxDiscoveryIconBytes))
	m := iconManifest(asset)

	if err := bundle.ValidateCapabilitiesForInstall(&m, installDir); err != nil {
		t.Fatalf("an icon of exactly the per-icon limit must be accepted: %v", err)
	}
}

// TestDiscoveryIconsOverTotalBudgetRefuseThePlugin covers the limit no single asset violates: 32
// icons of 48 KiB are each well under the per-icon cap and together exceed 1 MiB.
func TestDiscoveryIconsOverTotalBudgetRefuseThePlugin(t *testing.T) {
	installDir := t.TempDir()
	const each = 48 * 1024
	var assets []string
	for i := 0; i < 32; i++ {
		assets = append(assets, writeIcon(t, installDir, fmt.Sprintf("i%02d.svg", i), each))
	}
	m := iconManifest(assets...)

	err := bundle.ValidateCapabilitiesForInstall(&m, installDir)
	if err == nil {
		t.Fatalf("%d icons of %d bytes exceed the 1 MiB total and must refuse the plugin", len(assets), each)
	}
	// The asset that tipped the total over is named: it is where the author has to start cutting.
	if !strings.Contains(err.Error(), ".svg") {
		t.Fatalf("the total-budget error must name the asset it stopped at, got %v", err)
	}
}

// TestDiscoveryIconsWithinBudgetAreAccepted is the negative control: the same shape of manifest,
// under both limits, must install.
func TestDiscoveryIconsWithinBudgetAreAccepted(t *testing.T) {
	installDir := t.TempDir()
	var assets []string
	for i := 0; i < 8; i++ {
		assets = append(assets, writeIcon(t, installDir, fmt.Sprintf("i%02d.svg", i), 32*1024))
	}
	m := iconManifest(assets...)

	if err := bundle.ValidateCapabilitiesForInstall(&m, installDir); err != nil {
		t.Fatalf("8 icons of 32 KiB are within both budgets: %v", err)
	}
}
