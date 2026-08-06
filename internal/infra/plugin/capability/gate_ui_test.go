package capability

import (
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

var surfaceMethods = []string{
	"surface.open",
	"surface.write",
	"surface.updateState",
	"surface.setTitle",
	"surface.close",
}

func uiManifest(t *testing.T, ui *domainplugin.UICaps) domainplugin.Manifest {
	t.Helper()
	m := domainplugin.Manifest{ID: "com.example.ui", Name: "UI", Version: "1.0.0"}
	m.Engine.Type = "go-binary"
	m.Engine.Entry = "ui.exe"
	m.Capabilities.UI = ui
	return m
}

func TestGateDeniesSurfaceWithoutUICapability(t *testing.T) {
	g := newGate(t, uiManifest(t, nil))
	for _, method := range surfaceMethods {
		if g.Allow(method) {
			t.Fatalf("%s allowed without capabilities.ui", method)
		}
	}
}

func TestGateDeniesSurfaceWhenNoKindIsDeclared(t *testing.T) {
	g := newGate(t, uiManifest(t, &domainplugin.UICaps{Dialogs: true}))
	for _, method := range surfaceMethods {
		if g.Allow(method) {
			t.Fatalf("%s allowed with a ui block declaring no surface kind", method)
		}
	}
}

func TestGateAllowsSurfaceMethodsWithADeclaredKind(t *testing.T) {
	g := newGate(t, uiManifest(t, &domainplugin.UICaps{Surfaces: []string{"log"}}))
	for _, method := range surfaceMethods {
		if !g.Allow(method) {
			t.Fatalf("%s denied for a plugin declaring ui.surfaces", method)
		}
	}
}

// The gate answers one question — may this plugin speak surface.* at all — and deliberately not
// which KIND it may open. That check needs the request body, which the gate never sees, and lives
// in the use case (SurfaceService.open). Pinning the boundary here keeps a later reader from
// implementing the kind check twice and letting the two copies drift.
func TestGateDoesNotDiscriminateBetweenDeclaredKinds(t *testing.T) {
	logOnly := newGate(t, uiManifest(t, &domainplugin.UICaps{Surfaces: []string{"log"}}))
	if !logOnly.Allow("surface.open") {
		t.Fatal("surface.open must be gate-allowed for a plugin declaring any surface kind")
	}
}
