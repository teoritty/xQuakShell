package plugin

import "testing"

// manifestWithUI builds the smallest manifest that reaches validateUICaps, so each test below
// varies exactly one thing: the ui block.
func manifestWithUI(ui *UICaps) Manifest {
	m := Manifest{ID: "com.example.p", Name: "P", Version: "1.0.0"}
	m.Engine.Type = "go-binary"
	m.Engine.Entry = "p.exe"
	m.Capabilities.UI = ui
	return m
}

func manifestWithUIAndDiscovery(ui *UICaps) Manifest {
	m := manifestWithUI(ui)
	m.Capabilities.Discovery = &DiscoveryCaps{ParentProtocols: []string{"ssh"}}
	return m
}

func TestUICapsAbsentIsValid(t *testing.T) {
	m := manifestWithUI(nil)
	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("a manifest without a ui block must validate: %v", err)
	}
}

func TestUICapsRejectsUnknownSurfaceKind(t *testing.T) {
	m := manifestWithUI(&UICaps{Surfaces: []string{"hologram"}})
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected unknown surface kind to be rejected")
	}
}

func TestUICapsRejectsDuplicateSurfaceKind(t *testing.T) {
	m := manifestWithUI(&UICaps{Surfaces: []string{"log", "log"}})
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected a duplicate surface kind to be rejected")
	}
}

func TestUICapsRejectsMaxSurfacesAboveCeiling(t *testing.T) {
	m := manifestWithUI(&UICaps{Surfaces: []string{"log"}, MaxSurfaces: MaxSurfacesPerPluginCeiling + 1})
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected maxSurfaces above the ceiling to be rejected")
	}
}

func TestUICapsRejectsNegativeMaxSurfaces(t *testing.T) {
	m := manifestWithUI(&UICaps{Surfaces: []string{"log"}, MaxSurfaces: -1})
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected a negative maxSurfaces to be rejected")
	}
}

func TestUICapsRejectsMaxSurfacesWithoutSurfaces(t *testing.T) {
	m := manifestWithUI(&UICaps{Dialogs: true, MaxSurfaces: 4})
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected maxSurfaces without a surfaces list to be rejected")
	}
}

// A ui block that grants nothing is not a harmless no-op: it reads as a permission the user was
// shown and the plugin then cannot use, which is exactly the kind of drift manifest validation
// exists to catch.
func TestUICapsRejectsEmptyGrant(t *testing.T) {
	m := manifestWithUI(&UICaps{})
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected a ui block granting nothing to be rejected")
	}
}

// nodeDetails describes a node in a discovery subtree. Without capabilities.discovery there is no
// subtree, so the grant could never be exercised.
func TestUICapsRejectsNodeDetailsWithoutDiscovery(t *testing.T) {
	m := manifestWithUI(&UICaps{NodeDetails: true})
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected nodeDetails without capabilities.discovery to be rejected")
	}
}

func TestUICapsAcceptsNodeDetailsWithDiscovery(t *testing.T) {
	m := manifestWithUIAndDiscovery(&UICaps{NodeDetails: true})
	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("nodeDetails with discovery must validate: %v", err)
	}
}

func TestUICapsAcceptsValidDeclaration(t *testing.T) {
	m := manifestWithUIAndDiscovery(&UICaps{
		Surfaces:    []string{"terminal", "log"},
		Dialogs:     true,
		NodeDetails: true,
	})
	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("expected valid ui caps to pass: %v", err)
	}
	if got := m.Capabilities.UI.EffectiveMaxSurfaces(); got != MaxSurfacesPerPluginDefault {
		t.Fatalf("EffectiveMaxSurfaces() = %d, want the host default %d", got, MaxSurfacesPerPluginDefault)
	}
	if !m.Capabilities.UI.AllowsSurfaceKind("log") {
		t.Fatal("AllowsSurfaceKind(log) = false for a declared kind")
	}
	if m.Capabilities.UI.AllowsSurfaceKind("hologram") {
		t.Fatal("AllowsSurfaceKind(hologram) = true for an undeclared kind")
	}
}

func TestUICapsEffectiveMaxSurfacesHonoursDeclaration(t *testing.T) {
	ui := &UICaps{Surfaces: []string{"log"}, MaxSurfaces: 3}
	if got := ui.EffectiveMaxSurfaces(); got != 3 {
		t.Fatalf("EffectiveMaxSurfaces() = %d, want 3", got)
	}
}

// Every accessor is nil-safe because the gate and the use case both reach for the capability of a
// plugin that may not declare it at all.
func TestUICapsNilAccessorsAreSafe(t *testing.T) {
	var ui *UICaps
	if ui.AllowsSurfaceKind("log") {
		t.Fatal("nil UICaps must allow no surface kind")
	}
	if got := ui.EffectiveMaxSurfaces(); got != 0 {
		t.Fatalf("nil UICaps EffectiveMaxSurfaces() = %d, want 0", got)
	}
}

func TestUICapsAddsPermissionSummaryLine(t *testing.T) {
	m := manifestWithUI(&UICaps{Surfaces: []string{"log"}})
	found := false
	for _, line := range m.PermissionSummary() {
		if line == "Show its own tabs, dialogs and node details" {
			found = true
		}
	}
	if !found {
		t.Fatalf("PermissionSummary() = %v, want a ui line", m.PermissionSummary())
	}
}
