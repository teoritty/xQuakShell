package plugin_test

import (
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
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected empty protocol entry to be rejected")
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
			ID:    "icon-" + string(rune('a'+i%26)) + string(rune('0'+i%10)),
			Asset: "ui/icons/icon.svg",
		}
	}
	m.Contributions.DiscoveryIcons = icons
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected more than 64 icons to be rejected")
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
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected path traversal in discoveryIcons asset to be rejected")
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
