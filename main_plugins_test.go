package main

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
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

func (*grantVault) Exists() bool                                       { return true }
func (*grantVault) Create(context.Context, string) error               { return nil }
func (*grantVault) Unlock(context.Context, string) error               { return nil }
func (*grantVault) VerifyMasterPassword(context.Context, string) error { return nil }
func (*grantVault) Lock()                                              {}
func (*grantVault) IsUnlocked() bool                                   { return true }
func (v *grantVault) GetData() (*domain.VaultData, error)              { return &v.data, nil }
func (v *grantVault) UpdateData(_ context.Context, mutate func(*domain.VaultData) error) error {
	return mutate(&v.data)
}

// recordConsent replaced five byte-for-byte identical grant methods - exactly the shape where a
// copy-paste slip grants the wrong capability and nothing observable changes until a plugin holds an
// access nobody approved. Which permission each consent box governs is now settled in one place and
// tested there; what this pins is that the runtime persists that result instead of dropping it, and
// persists only it.
func TestPluginRuntimeRecordsTheConsentItIsGiven(t *testing.T) {
	vault := &grantVault{}
	r := &pluginRuntime{vaultSettings: usecase.NewPluginVaultSettings(vault)}
	manifest := &domainplugin.Manifest{
		ID: "p1",
		Capabilities: domainplugin.CapabilitySet{
			Auth:   &domainplugin.AuthCaps{Provider: true},
			Tunnel: &domainplugin.TunnelCaps{Provider: true},
		},
	}

	err := r.recordConsent(context.Background(), manifest, domainplugin.ConsentFlags{AuthProvider: true})
	if err != nil {
		t.Fatalf("recordConsent err = %v, want nil", err)
	}

	grant, ok := vault.data.Settings.Plugins.GrantFor("p1")
	if !ok {
		t.Fatal("the consent never reached the vault")
	}
	if !slices.Contains(grant.Granted, domainplugin.PermissionAuthProvider) {
		t.Errorf("the role the user agreed to is missing: %v", grant.Granted)
	}
	if slices.Contains(grant.Granted, domainplugin.PermissionTunnelProvider) {
		t.Errorf("a role the user did not agree to was recorded: %v", grant.Granted)
	}
}

// A grant on an unbuilt runtime must be a no-op rather than a nil dereference:
// composeApp calls these before the vault is necessarily present.
func TestPluginRuntimeGrantsAreNilSafe(t *testing.T) {
	var absent *pluginRuntime
	empty := &pluginRuntime{}
	for _, r := range []*pluginRuntime{absent, empty} {
		if err := r.recordConsent(context.Background(), &domainplugin.Manifest{ID: "p1"}, domainplugin.ConsentFlags{}); err != nil {
			t.Fatalf("recording consent on an unbuilt runtime = %v, want nil", err)
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

// Unlock must survive a runtime with no replication wired at all rather than panicking on the way
// into the application.
func TestSyncReplicasAtUnlockToleratesNoService(t *testing.T) {
	(&pluginRuntime{}).syncReplicasAtUnlock(context.Background(), nil)
}
