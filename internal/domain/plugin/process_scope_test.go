package plugin_test

import (
	"errors"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestHostProcessScopePerPlugin(t *testing.T) {
	m := domainplugin.Manifest{Isolation: domainplugin.IsolationPerPlugin}
	scope, err := m.HostProcessScope("")
	if err != nil {
		t.Fatal(err)
	}
	if scope != "" {
		t.Fatalf("expected empty scope, got %q", scope)
	}
	scope, err = m.HostProcessScope("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if scope != "" {
		t.Fatalf("per-plugin must ignore session id, got %q", scope)
	}
}

func TestHostProcessScopePerSessionRequiresSessionID(t *testing.T) {
	m := domainplugin.Manifest{Isolation: domainplugin.IsolationPerSession}
	if _, err := m.HostProcessScope(""); !errors.Is(err, domainplugin.ErrSessionScopeRequired) {
		t.Fatalf("expected ErrSessionScopeRequired, got %v", err)
	}
	scope, err := m.HostProcessScope("sess-42")
	if err != nil {
		t.Fatal(err)
	}
	if scope != "sess-42" {
		t.Fatalf("expected sess-42, got %q", scope)
	}
}
