package usecase

import (
	"context"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

func replicatingPlugin(id string) domainplugin.InstalledPlugin {
	return domainplugin.InstalledPlugin{Manifest: domainplugin.Manifest{
		ID: id,
		Capabilities: domainplugin.CapabilitySet{
			Scope:   &domainplugin.ScopeCaps{},
			Replica: &domainplugin.ReplicaCaps{},
		},
	}}
}

func plainPlugin(id string) domainplugin.InstalledPlugin {
	return domainplugin.InstalledPlugin{Manifest: domainplugin.Manifest{ID: id}}
}

// A plugin that never asked to replicate is not synchronised, whatever else is installed. Reaching
// its transport at all would be calling an RPC it never declared.
func TestOnlyPluginsThatDeclaredReplicationAreSynchronised(t *testing.T) {
	service, _, remote, _ := syncFixture(t)

	synced, err := service.SyncAll(context.Background(),
		[]domainplugin.InstalledPlugin{plainPlugin("com.example.tool"), replicatingPlugin("com.example.sync")})
	if err != nil {
		t.Fatalf("SyncAll: %v", err)
	}

	if synced != 1 {
		t.Fatalf("synced %d plugins, want only the one that declared replication", synced)
	}
	if remote.pushes != 1 {
		t.Fatalf("%d pushes, want one", remote.pushes)
	}
}

// A plugin the user has not given a key yet is skipped rather than reported as broken. It is the
// ordinary state of a freshly installed sync plugin, and the unlock path must not shout about it.
func TestAPluginWithNoKeyIsSkippedQuietly(t *testing.T) {
	service, vault, remote, _ := syncFixture(t)
	vault.data.ReplicaKeys = nil

	synced, err := service.SyncAll(context.Background(),
		[]domainplugin.InstalledPlugin{replicatingPlugin("com.example.sync")})

	if err != nil {
		t.Fatalf("SyncAll err = %v, want a plugin without a key to be skipped", err)
	}
	if synced != 0 {
		t.Errorf("synced = %d, want nothing synchronised", synced)
	}
	if remote.pushes != 0 {
		t.Error("something was pushed for a plugin with no key")
	}
}

// One plugin's failure must not cost the others their sync. This runs once per unlock, and a single
// unreachable server would otherwise stop every other scope from converging.
func TestOneFailureDoesNotStopTheOthers(t *testing.T) {
	service, vault, remote, _ := syncFixture(t)
	vault.data.Settings.Plugins.ScopeRoots = append(vault.data.Settings.Plugins.ScopeRoots,
		domain.ScopeRoot{FolderID: "second", PluginID: "com.example.two"})
	vault.data.Folders = append(vault.data.Folders, domain.ConnectionFolder{ID: "second", Name: "Second"})
	vault.data.ReplicaKeys["com.example.two"] = "OTHERKEY"
	// A key but no scope: the plugin is set up for replication and still has nothing it is allowed
	// to send, which is a failure rather than the "not configured yet" the key check screens out.
	vault.data.ReplicaKeys["com.example.missing"] = "THIRDKEY"

	// The first plugin has no scope at all, so its sync fails; the second is set up correctly.
	synced, err := service.SyncAll(context.Background(), []domainplugin.InstalledPlugin{
		replicatingPlugin("com.example.missing"),
		replicatingPlugin("com.example.two"),
	})

	if err == nil {
		t.Fatal("the failure was not reported")
	}
	if synced != 1 {
		t.Fatalf("synced = %d, want the healthy plugin to have synchronised anyway", synced)
	}
	if remote.pushes != 1 {
		t.Errorf("%d pushes, want the healthy plugin's", remote.pushes)
	}
}

// Cancellation stops the loop. A window closing during unlock must not leave the application
// working through every installed plugin's server.
func TestSyncAllStopsWhenCancelled(t *testing.T) {
	service, _, remote, _ := syncFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.SyncAll(ctx, []domainplugin.InstalledPlugin{replicatingPlugin("com.example.sync")})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if remote.pushes != 0 {
		t.Error("a cancelled sync still pushed")
	}
}

// Nothing installed is not an error, and must not report work that did not happen.
func TestSyncingNothingIsNotAnError(t *testing.T) {
	service, _, _, _ := syncFixture(t)

	if synced, err := service.SyncAll(context.Background(), nil); err != nil || synced != 0 {
		t.Fatalf("synced = %d, err = %v; want 0 and nil", synced, err)
	}
}
