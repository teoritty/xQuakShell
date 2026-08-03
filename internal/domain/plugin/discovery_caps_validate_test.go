package plugin_test

import (
	"fmt"
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func baseDiscoveryManifest() domainplugin.Manifest {
	return domainplugin.Manifest{
		ID:      "com.example.discovery",
		Name:    "Discovery",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "discovery.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Discovery: &domainplugin.DiscoveryCaps{
				ParentProtocols: []string{"ssh"},
			},
		},
		Contributions: domainplugin.Contributions{
			DiscoveryIcons: []domainplugin.DiscoveryIconContribution{
				{ID: "docker", Asset: "ui/icons/docker.svg"},
			},
		},
	}
}

func TestValidateDiscoveryCapsAcceptsValidManifest(t *testing.T) {
	m := baseDiscoveryManifest()
	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("expected valid discovery manifest, got %v", err)
	}
}

func TestValidateDiscoveryCapsAllowsNoDiscoveryCapability(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "com.example.nodiscovery",
		Name:    "NoDiscovery",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "x.exe"},
	}
	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("expected nil discovery caps to be valid, got %v", err)
	}
}

func TestValidateDiscoveryCapsRejectsEmptyParentProtocols(t *testing.T) {
	m := baseDiscoveryManifest()
	m.Capabilities.Discovery.ParentProtocols = nil
	m.Contributions.DiscoveryIcons = nil
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "parentProtocols") {
		t.Fatalf("expected parentProtocols error, got %v", err)
	}
}

func TestValidateDiscoveryCapsRejectsEmptyProtocolEntry(t *testing.T) {
	m := baseDiscoveryManifest()
	m.Contributions.DiscoveryIcons = nil
	m.Capabilities.Discovery.ParentProtocols = []string{"ssh", "  "}
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "parentProtocols") {
		t.Fatalf("expected empty protocol entry to be rejected with a parentProtocols error, got %v", err)
	}
}

func TestValidateDiscoveryCapsRejectsIconsWithoutCapability(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "com.example.iconsonly",
		Name:    "IconsOnly",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "x.exe"},
		Contributions: domainplugin.Contributions{
			DiscoveryIcons: []domainplugin.DiscoveryIconContribution{
				{ID: "docker", Asset: "ui/icons/docker.svg"},
			},
		},
	}
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "requires capabilities.discovery") {
		t.Fatalf("expected discoveryIcons-without-capability error, got %v", err)
	}
}

func TestValidateDiscoveryCapsRejectsTooManyIcons(t *testing.T) {
	m := baseDiscoveryManifest()
	icons := make([]domainplugin.DiscoveryIconContribution, 65)
	for i := range icons {
		icons[i] = domainplugin.DiscoveryIconContribution{
			ID:    fmt.Sprintf("icon-%d", i),
			Asset: "ui/icons/icon.svg",
		}
	}
	m.Contributions.DiscoveryIcons = icons
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected more than 64 icons to be rejected with an exceeds error, got %v", err)
	}
}

// TestValidateDiscoveryCapsAcceptsExactlyMaxIcons pins the boundary: exactly 64 icons must pass,
// so the "> 64" check in validateDiscoveryIcons cannot silently drift into "> 63" or "> 65".
func TestValidateDiscoveryCapsAcceptsExactlyMaxIcons(t *testing.T) {
	m := baseDiscoveryManifest()
	icons := make([]domainplugin.DiscoveryIconContribution, 64)
	for i := range icons {
		icons[i] = domainplugin.DiscoveryIconContribution{
			ID:    fmt.Sprintf("icon-%d", i),
			Asset: "ui/icons/icon.svg",
		}
	}
	m.Contributions.DiscoveryIcons = icons
	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("expected exactly 64 icons to be accepted, got %v", err)
	}
}

func TestValidateDiscoveryCapsRejectsDuplicateIconID(t *testing.T) {
	m := baseDiscoveryManifest()
	m.Contributions.DiscoveryIcons = []domainplugin.DiscoveryIconContribution{
		{ID: "docker", Asset: "ui/icons/a.svg"},
		{ID: "docker", Asset: "ui/icons/b.svg"},
	}
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate icon id error, got %v", err)
	}
}

func TestValidateDiscoveryCapsRejectsDisallowedExtension(t *testing.T) {
	m := baseDiscoveryManifest()
	m.Contributions.DiscoveryIcons = []domainplugin.DiscoveryIconContribution{
		{ID: "docker", Asset: "ui/icons/docker.gif"},
	}
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "extension") {
		t.Fatalf("expected disallowed extension error, got %v", err)
	}
}

func TestValidateDiscoveryCapsAcceptsAllowedExtensionsCaseInsensitively(t *testing.T) {
	m := baseDiscoveryManifest()
	m.Contributions.DiscoveryIcons = []domainplugin.DiscoveryIconContribution{
		{ID: "a", Asset: "ui/icons/a.SVG"},
		{ID: "b", Asset: "ui/icons/b.Png"},
		{ID: "c", Asset: "ui/icons/c.ICO"},
	}
	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("expected case-insensitive extensions to be accepted, got %v", err)
	}
}

func TestValidateDiscoveryCapsRejectsPathTraversal(t *testing.T) {
	m := baseDiscoveryManifest()
	m.Contributions.DiscoveryIcons = []domainplugin.DiscoveryIconContribution{
		{ID: "docker", Asset: "ui/../../../etc/passwd.svg"},
	}
	// Confirms the traversal is actually caught by the reused ValidateViewAssetEntry (its "must
	// not contain .." message), not by some unrelated check further down the function.
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "..") {
		t.Fatalf("expected path traversal to be rejected by ValidateViewAssetEntry, got %v", err)
	}
}

func TestValidateDiscoveryCapsRejectsEmptyAsset(t *testing.T) {
	m := baseDiscoveryManifest()
	m.Contributions.DiscoveryIcons = []domainplugin.DiscoveryIconContribution{
		{ID: "docker", Asset: ""},
	}
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "asset is required") {
		t.Fatalf("expected empty asset to be rejected with 'asset is required', got %v", err)
	}
}

func TestPermissionSummaryIncludesDiscoveryLine(t *testing.T) {
	m := baseDiscoveryManifest()
	lines := m.PermissionSummary()
	found := false
	for _, l := range lines {
		if strings.Contains(l, "discovered resources") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected discovery permission line, got %v", lines)
	}
}

// TestDiscoveryCapabilityIsGrantable is a regression test for a real bug found in review:
// GrantedCapabilityNames() initially had no branch for Discovery, so grantsCapability(CapDiscovery)
// returned false even for a manifest that plainly declared capabilities.discovery — which made
// requires.capabilities.discovery rejected as "requires capability that is not granted" and the
// plugin uninstallable. This exercises the full path (grant + requires + Validate), not just
// GrantedCapabilityNames in isolation, so a future regression here fails loudly.
func TestDiscoveryCapabilityIsGrantable(t *testing.T) {
	m := baseDiscoveryManifest()
	m.Requires = &domainplugin.RequirementSet{
		PluginAPI: "1.0.0",
		Capabilities: map[domainplugin.CapabilityID]domainplugin.CapabilityRequirement{
			domainplugin.CapDiscovery: {Min: "1.0.0", Features: []domainplugin.FeatureID{domainplugin.FeatDiscoveryPublish}},
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected discovery capability to be grantable and satisfiable, got %v", err)
	}
	if err := m.CheckHostCompatibility(); err != nil {
		t.Fatalf("expected discovery requirement to be satisfied by the host, got %v", err)
	}
}

func TestPermissionSummaryOmitsDiscoveryLineWithoutCapability(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "com.example.nodiscovery2",
		Name:    "NoDiscovery",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "x.exe"},
	}
	lines := m.PermissionSummary()
	for _, l := range lines {
		if strings.Contains(l, "discovered resources") {
			t.Fatalf("did not expect discovery permission line, got %v", lines)
		}
	}
}
