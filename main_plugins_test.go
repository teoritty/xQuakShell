package main

import (
	"context"
	"reflect"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// The plugin runtime is a composition root (ADR-010): its whole job is to leave
// nothing unconnected. Every unit it assembles has its own tests and can still
// be dead in production if newPluginRuntime forgets to assign it — a field left
// nil produces no compile error, no panic at startup, and a feature that
// silently does nothing. This walks the struct instead of naming fields, so a
// field added later is covered the day it is added rather than the day someone
// remembers to extend a test.
//
// The check runs after wireEmbed because assembly is split across the two:
// newPluginRuntime builds the graph, wireEmbed fills in what needs the AppAPI
// (embedBridge). Only the pair of them together is supposed to leave the struct
// complete, so only the pair of them is worth asserting.
func TestPluginRuntimeLeavesNoFieldUnwired(t *testing.T) {
	api, runtime := composeDiscoveryRuntime(t)
	runtime.wireEmbed(api)

	// Closed allowlist for fields a bare compose legitimately leaves nil. It is
	// empty on purpose: every field is wired today, including both GitHub
	// services, which survive a nil portable data store. An entry here must
	// state why, and the test fails once that reason stops being true — an
	// exemption that quietly outlives its cause is worse than no test.
	allowedNil := map[string]string{}

	v := reflect.ValueOf(runtime).Elem()
	typ := v.Type()
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		field := v.Field(i)
		switch field.Kind() {
		case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
		default:
			t.Fatalf("%s is not a nilable kind (%s); this test can no longer prove it was wired", name, field.Kind())
		}
		reason, allowed := allowedNil[name]
		if field.IsNil() && !allowed {
			t.Fatalf("pluginRuntime.%s was never wired by newPluginRuntime", name)
		}
		if !field.IsNil() && allowed {
			t.Fatalf("pluginRuntime.%s is wired now — drop its allowlist entry (%q)", name, reason)
		}
	}
}

// grantVault records which capability map a grant landed in.
type grantVault struct {
	data domain.VaultData
}

func (*grantVault) Exists() bool                          { return true }
func (*grantVault) Create(context.Context, string) error  { return nil }
func (*grantVault) Unlock(context.Context, string) error  { return nil }
func (*grantVault) Lock()                                 {}
func (*grantVault) IsUnlocked() bool                      { return true }
func (v *grantVault) GetData() (*domain.VaultData, error) { return &v.data, nil }
func (v *grantVault) UpdateData(_ context.Context, mutate func(*domain.VaultData) error) error {
	return mutate(&v.data)
}

// The five grant methods are byte-for-byte identical apart from the capability
// they delegate to, which is exactly the shape where a copy-paste slip grants
// the wrong thing and nothing observable changes until a plugin has an access
// it was never approved for. This pins each one to its own capability.
func TestPluginRuntimeGrantsTheCapabilityItNames(t *testing.T) {
	cases := []struct {
		name  string
		grant func(*pluginRuntime, context.Context, string) error
		got   func(domain.PluginSettings) map[string]bool
	}{
		{"multiSession", (*pluginRuntime).grantMultiSessionAccess, func(p domain.PluginSettings) map[string]bool { return p.MultiSessionAccessGranted }},
		{"secret", (*pluginRuntime).grantSecretAccess, func(p domain.PluginSettings) map[string]bool { return p.SecretAccessGranted }},
		{"authProvider", (*pluginRuntime).grantAuthProviderAccess, func(p domain.PluginSettings) map[string]bool { return p.AuthProviderAccessGranted }},
		{"tunnelProvider", (*pluginRuntime).grantTunnelProviderAccess, func(p domain.PluginSettings) map[string]bool { return p.TunnelProviderAccessGranted }},
		{"arbitraryNetwork", (*pluginRuntime).grantArbitraryNetworkAccess, func(p domain.PluginSettings) map[string]bool { return p.ArbitraryNetworkAccessGranted }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vault := &grantVault{}
			r := &pluginRuntime{vaultSettings: usecase.NewPluginVaultSettings(vault)}
			if err := tc.grant(r, context.Background(), "p1"); err != nil {
				t.Fatal(err)
			}
			plugins := vault.data.Settings.Plugins
			if !tc.got(plugins)["p1"] {
				t.Fatalf("%s grant did not reach its own capability map", tc.name)
			}
			for _, other := range cases {
				if other.name == tc.name {
					continue
				}
				if other.got(plugins)["p1"] {
					t.Fatalf("%s grant also granted %s", tc.name, other.name)
				}
			}
		})
	}
}

// A grant on an unbuilt runtime must be a no-op rather than a nil dereference:
// composeApp calls these before the vault is necessarily present.
func TestPluginRuntimeGrantsAreNilSafe(t *testing.T) {
	var absent *pluginRuntime
	empty := &pluginRuntime{}
	for _, r := range []*pluginRuntime{absent, empty} {
		if err := r.grantSecretAccess(context.Background(), "p1"); err != nil {
			t.Fatalf("grant on an unbuilt runtime = %v, want nil", err)
		}
		if r.assetHandler() != nil {
			t.Fatal("assetHandler on an unbuilt runtime must be nil")
		}
		r.setSessionRecoverer(nil)
	}
}

// shutdown cancels the context the idle suspender exits on. It runs from
// deferred cleanup and from the Wails shutdown hook, so it must tolerate both.
func TestPluginRuntimeShutdownCancelsAndRepeats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &pluginRuntime{cancel: cancel}

	r.shutdown()
	if ctx.Err() == nil {
		t.Fatal("shutdown did not cancel the runtime context")
	}
	r.shutdown()

	(&pluginRuntime{}).shutdown()
}
