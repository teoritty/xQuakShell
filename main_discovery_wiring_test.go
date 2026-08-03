package main

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/loghub"
	"xquakshell/internal/infra/persistence"
	infrapinger "xquakshell/internal/infra/pinger"
	"xquakshell/internal/pkg/conlimit"
	presentation "xquakshell/internal/presentation/wails"
	"xquakshell/internal/usecase"
)

// Wiring, not code. Every discovery unit below this file is tested in isolation, and all of it can
// still be dead in production if wireEmbed stops connecting it: GetDiscoveryTree would return an
// empty tree, commands would fail with "discovery service unavailable", the tree-changed event
// would never fire, and no unit test would notice. This file asserts the two connections that make
// the difference between written and connected.

// composeDiscoveryRuntime builds the same pair composeApp builds — a real AppAPI and a real plugin
// runtime — over temporary directories, and returns them UNWIRED so a test can observe both sides
// of wireEmbed.
func composeDiscoveryRuntime(t *testing.T) (*presentation.AppAPI, *pluginRuntime) {
	t.Helper()
	vaultRepo := persistence.NewVaultRepo(t.TempDir())
	connRepo := persistence.NewConnectionRepo(vaultRepo)
	identRepo := persistence.NewIdentityRepo(vaultRepo)
	passwordRepo := persistence.NewPasswordRepo(vaultRepo)
	knownHosts := persistence.NewKnownHostsRepo(vaultRepo)

	api := presentation.NewAppAPI(
		vaultRepo, connRepo, identRepo, passwordRepo, knownHosts,
		nil, usecase.SSHSessionDeps{}, nil,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil,
		nil, nil, nil, nil,
		loghub.Default(), nil,
		conlimit.New(4), conlimit.New(4),
		func() domain.ConcurrencyLimiter { return conlimit.New(4) },
		infrapinger.NewTCPPinger(time.Second), nil, nil,
	)

	runtime := newPluginRuntime(t.TempDir(), nil, pluginRuntimeDeps{
		ConnRepo:      connRepo,
		PasswordRepo:  passwordRepo,
		IdentRepo:     identRepo,
		VaultSettings: usecase.NewPluginVaultSettings(vaultRepo),
		ExeDir:        t.TempDir(),
	})
	t.Cleanup(runtime.shutdown)
	return api, runtime
}

// TestWireEmbedConnectsTheDiscoveryServiceToTheAPI: after composition, a command from the frontend
// must reach the real use case.
//
// The proof is the error it comes back with. Before wiring, the handler answers "no service" from
// its own nil check without consulting anything. After wiring, an action on a node that does not
// exist is refused by the invoker's own tree check — an error only the real use case can produce.
// A test that merely asserted "no error" would pass against a handler wired to nothing at all.
func TestWireEmbedConnectsTheDiscoveryServiceToTheAPI(t *testing.T) {
	api, runtime := composeDiscoveryRuntime(t)

	if err := api.SetDiscoveryObserved("conn-1", nil); err == nil {
		t.Fatal("expected discovery to be inert before wireEmbed; the assertions below would prove nothing")
	}

	runtime.wireEmbed(api)

	if err := api.SetDiscoveryObserved("conn-1", []string{""}); err != nil {
		t.Fatalf("SetDiscoveryObserved did not reach the use case after wiring: %v", err)
	}
	err := api.InvokeDiscoveryAction("conn-1", "plugin-a", []string{"no-such-node"}, "stop")
	if !errors.Is(err, usecase.ErrDiscoveryNodeNotFound) {
		t.Fatalf("action did not reach the real invoker: got %v, want %v", err, usecase.ErrDiscoveryNodeNotFound)
	}
	if _, err := api.GetDiscoveryTree("conn-1"); err != nil {
		t.Fatalf("GetDiscoveryTree: %v", err)
	}
}

// TestWireEmbedConnectsTreeChangeEmissionToTheAPI: the other direction. The emit coalescer is built
// before the AppAPI exists, so its callback is late-bound through discoveryEmitHolder; if wireEmbed
// stops filling it, every tree update is swallowed and the frontend never repaints.
//
// The callback is compared by identity rather than by calling it, because emitting for real needs
// the Wails runtime context, which no test has. The two facts together — the holder holds
// EmitDiscoveryTreeChanged (here) and the holder forwards both arguments to whatever it holds
// (TestDiscoveryEmitHolderIsInertUntilWiredThenForwardsBoth) — close the path.
func TestWireEmbedConnectsTreeChangeEmissionToTheAPI(t *testing.T) {
	api, runtime := composeDiscoveryRuntime(t)

	if emitOf(runtime) != nil {
		t.Fatal("expected no emit callback before wireEmbed")
	}

	runtime.wireEmbed(api)

	emit := emitOf(runtime)
	if emit == nil {
		t.Fatal("wireEmbed left the tree-change callback unset: the frontend would never be told a tree changed")
	}
	want := reflect.ValueOf(api.EmitDiscoveryTreeChanged).Pointer()
	if got := reflect.ValueOf(emit).Pointer(); got != want {
		t.Fatal("the tree-change callback is not AppAPI.EmitDiscoveryTreeChanged")
	}
}

func emitOf(r *pluginRuntime) func(connectionID, nodeID string) {
	r.discoveryEmit.mu.Lock()
	defer r.discoveryEmit.mu.Unlock()
	return r.discoveryEmit.emit
}
