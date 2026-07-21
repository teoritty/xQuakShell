package plugin_test

import (
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func TestHostRegistryIsImmutable(t *testing.T) {
	// Descriptor returns a copy; mutating it must not affect the shared registry.
	d, ok := domainplugin.HostRegistry().Descriptor(domainplugin.CapVault)
	if !ok {
		t.Fatal("expected vault descriptor")
	}
	d.Features[0] = "MUTATED"

	if domainplugin.HostRegistry().HasFeature(domainplugin.CapVault, "MUTATED") {
		t.Fatal("registry leaked a mutable reference to the source of truth")
	}
	if !domainplugin.HostRegistry().HasFeature(domainplugin.CapVault, domainplugin.FeatVaultGetSecret) {
		t.Fatal("expected vault.getSecret feature")
	}
}

func TestRegistryLookups(t *testing.T) {
	r := domainplugin.HostRegistry()

	if !r.Has("session") {
		t.Fatal("expected session capability")
	}
	if r.Has("nonexistent") {
		t.Fatal("did not expect unknown capability")
	}
	if !r.HasFeature("filesystem", "read") || !r.HasFeature("filesystem", "write") {
		t.Fatal("expected filesystem read+write features")
	}
	if r.HasFeature("filesystem", "delete") {
		t.Fatal("did not expect filesystem.delete feature")
	}

	v, ok := r.CapabilityVersion("network")
	if !ok {
		t.Fatal("expected network version")
	}
	if v.Major != 1 || v.Minor != 0 {
		t.Fatalf("network version = %s, want 1.0.x", v)
	}
	if _, ok := r.CapabilityVersion("nonexistent"); ok {
		t.Fatal("did not expect version for unknown capability")
	}
}

func TestHostDescriptor(t *testing.T) {
	d := domainplugin.HostDescriptor()
	if d.PluginAPI != domainplugin.PluginAPIVersion {
		t.Fatalf("pluginApi = %q, want %q", d.PluginAPI, domainplugin.PluginAPIVersion)
	}
	vault, ok := d.Capabilities["vault"]
	if !ok {
		t.Fatal("expected vault in descriptor")
	}
	if vault.Version != "1.0.0" {
		t.Fatalf("vault version = %q, want 1.0.0", vault.Version)
	}
}
