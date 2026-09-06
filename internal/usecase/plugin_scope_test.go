package usecase

import (
	"context"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

func scopeManifest(id string) *domainplugin.Manifest {
	return &domainplugin.Manifest{
		ID: id, Name: "Sync",
		Capabilities: domainplugin.CapabilitySet{Scope: &domainplugin.ScopeCaps{}},
	}
}

func scopeVault(t *testing.T) (*PluginVaultSettings, *domain.VaultData) {
	t.Helper()
	data := domain.NewVaultData()
	return NewPluginVaultSettings(&revokeTestVault{data: data}), data
}

// A plugin that declares it needs a scope gets one folder and one root. Without this the anchor
// never fires, because there is nowhere for the user to put anything.
func TestAPluginDeclaringAScopeGetsAFolderAndARoot(t *testing.T) {
	svc, data := scopeVault(t)

	created, err := svc.EnsureScopeRoot(context.Background(), scopeManifest("com.example.sync"))
	if err != nil {
		t.Fatalf("EnsureScopeRoot err = %v, want nil", err)
	}

	if !created {
		t.Fatal("no scope was created for a plugin that declares one")
	}
	if len(data.Settings.Plugins.ScopeRoots) != 1 {
		t.Fatalf("%d scope roots recorded, want 1", len(data.Settings.Plugins.ScopeRoots))
	}
	root := data.Settings.Plugins.ScopeRoots[0]
	if root.PluginID != "com.example.sync" {
		t.Errorf("the root names plugin %q", root.PluginID)
	}
	var found *domain.ConnectionFolder
	for i := range data.Folders {
		if data.Folders[i].ID == root.FolderID {
			found = &data.Folders[i]
		}
	}
	if found == nil {
		t.Fatal("the scope root names a folder that was never created")
	}
	if found.Name != "Sync" {
		t.Errorf("the scope folder is named %q, want the plugin's name", found.Name)
	}
	if found.ParentID != "" {
		t.Errorf("the scope folder is nested under %q, want the top level", found.ParentID)
	}
}

// It runs on every unlock, so the second pass must find the scope already there. Creating another
// would give the plugin two folders and the user two places to be confused by.
func TestASecondPassCreatesNoSecondScope(t *testing.T) {
	svc, data := scopeVault(t)
	ctx := context.Background()
	if _, err := svc.EnsureScopeRoot(ctx, scopeManifest("com.example.sync")); err != nil {
		t.Fatalf("first pass: %v", err)
	}

	created, err := svc.EnsureScopeRoot(ctx, scopeManifest("com.example.sync"))
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}

	if created {
		t.Error("the second pass created another scope")
	}
	if n := len(data.Settings.Plugins.ScopeRoots); n != 1 {
		t.Fatalf("%d scope roots after two passes, want 1", n)
	}
	if n := len(data.Folders); n != 1 {
		t.Fatalf("%d folders after two passes, want 1", n)
	}
}

// A plugin that never asked for a scope must not be given one. The folder is a standing invitation
// to put credentials somewhere they leave the machine, and it appears only for a plugin whose whole
// purpose is to take them somewhere.
func TestAPluginThatDeclaresNoScopeGetsNone(t *testing.T) {
	svc, data := scopeVault(t)

	created, err := svc.EnsureScopeRoot(context.Background(),
		&domainplugin.Manifest{ID: "com.example.plain", Name: "Plain"})
	if err != nil {
		t.Fatalf("EnsureScopeRoot err = %v", err)
	}

	if created || len(data.Settings.Plugins.ScopeRoots) != 0 || len(data.Folders) != 0 {
		t.Fatalf("a plugin with no scope capability was given one: roots=%v folders=%v",
			data.Settings.Plugins.ScopeRoots, data.Folders)
	}
}

// Uninstall stops the exposure and keeps the data. The folder and everything the user put in it
// stay exactly where they are; what goes is the root that made them visible to the plugin.
func TestUninstallRemovesTheScopeButNotTheFolder(t *testing.T) {
	svc, data := scopeVault(t)
	ctx := context.Background()
	if _, err := svc.EnsureScopeRoot(ctx, scopeManifest("com.example.sync")); err != nil {
		t.Fatalf("EnsureScopeRoot: %v", err)
	}

	if err := svc.RevokeAllGrants(ctx, "com.example.sync"); err != nil {
		t.Fatalf("RevokeAllGrants: %v", err)
	}

	if n := len(data.Settings.Plugins.ScopeRoots); n != 0 {
		t.Errorf("%d scope roots survived the uninstall", n)
	}
	if n := len(data.Folders); n != 1 {
		t.Errorf("%d folders after uninstall, want the folder and its contents kept", n)
	}
}

// Reinstalling gets a fresh, empty folder rather than the one from last time.
//
// A folder id derived from the plugin id would be reused, and the contents the user left in the old
// one would be exposed again the moment anything was installed under that id - with nobody asked.
// A plugin id is chosen by its author and verified against nothing, so "the same id" is not a
// coincidence an attacker has to wait for.
func TestReinstallingGetsAFreshScopeRatherThanTheOldOne(t *testing.T) {
	svc, data := scopeVault(t)
	ctx := context.Background()
	if _, err := svc.EnsureScopeRoot(ctx, scopeManifest("com.example.sync")); err != nil {
		t.Fatalf("first install: %v", err)
	}
	first := data.Settings.Plugins.ScopeRoots[0].FolderID
	if err := svc.RevokeAllGrants(ctx, "com.example.sync"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	if _, err := svc.EnsureScopeRoot(ctx, scopeManifest("com.example.sync")); err != nil {
		t.Fatalf("reinstall: %v", err)
	}

	second := data.Settings.Plugins.ScopeRoots[0].FolderID
	if second == first {
		t.Fatal("reinstalling reused the old scope folder, re-exposing whatever was left in it")
	}
	if n := len(data.Folders); n != 2 {
		t.Fatalf("%d folders, want the old one kept and a new one made", n)
	}
}

// One plugin's scope is not another's, and neither is one plugin's uninstall.
func TestRemovingOneScopeLeavesAnothersAlone(t *testing.T) {
	svc, data := scopeVault(t)
	ctx := context.Background()
	for _, id := range []string{"com.example.sync", "com.example.other"} {
		if _, err := svc.EnsureScopeRoot(ctx, scopeManifest(id)); err != nil {
			t.Fatalf("EnsureScopeRoot(%s): %v", id, err)
		}
	}

	if err := svc.RevokeAllGrants(ctx, "com.example.sync"); err != nil {
		t.Fatalf("RevokeAllGrants: %v", err)
	}

	roots := data.Settings.Plugins.ScopeRoots
	if len(roots) != 1 || roots[0].PluginID != "com.example.other" {
		t.Fatalf("roots after one uninstall = %v, want only the other plugin's", roots)
	}
}

// Composition can leave this unset and a manifest can fail to load; neither may panic on the path
// that runs every time the vault opens.
func TestEnsureScopeRootIsSafeWithNothingWired(t *testing.T) {
	ctx := context.Background()

	var unwired *PluginVaultSettings
	if created, err := unwired.EnsureScopeRoot(ctx, scopeManifest("com.example.sync")); created || err != nil {
		t.Errorf("unwired: created = %v, err = %v", created, err)
	}
	svc, _ := scopeVault(t)
	if created, err := svc.EnsureScopeRoot(ctx, nil); created || err != nil {
		t.Errorf("nil manifest: created = %v, err = %v", created, err)
	}
	if created, err := svc.EnsureScopeRoot(ctx, &domainplugin.Manifest{
		Capabilities: domainplugin.CapabilitySet{Scope: &domainplugin.ScopeCaps{}},
	}); created || err != nil {
		t.Errorf("manifest with no id: created = %v, err = %v", created, err)
	}
}

// The batch is what actually runs at unlock, so it needs its own test rather than resting on the
// single-plugin one: a loop that skipped everything would leave every plugin without a scope and
// nothing would say so.
func TestEveryInstalledPluginThatWantsAScopeGetsOne(t *testing.T) {
	svc, data := scopeVault(t)
	installed := []domainplugin.InstalledPlugin{
		{Manifest: *scopeManifest("com.example.sync")},
		{Manifest: domainplugin.Manifest{ID: "com.example.plain", Name: "Plain"}},
		{Manifest: *scopeManifest("com.example.other")},
	}

	created, err := svc.EnsureScopeRootsForAll(context.Background(), installed)
	if err != nil {
		t.Fatalf("EnsureScopeRootsForAll err = %v, want nil", err)
	}

	if created != 2 {
		t.Fatalf("created %d scopes, want 2 - one per plugin that asked", created)
	}
	if n := len(data.Settings.Plugins.ScopeRoots); n != 2 {
		t.Fatalf("%d scope roots recorded, want 2", n)
	}
}

// A second pass at unlock finds both scopes already there.
func TestASecondBatchCreatesNothing(t *testing.T) {
	svc, _ := scopeVault(t)
	installed := []domainplugin.InstalledPlugin{{Manifest: *scopeManifest("com.example.sync")}}
	ctx := context.Background()
	if _, err := svc.EnsureScopeRootsForAll(ctx, installed); err != nil {
		t.Fatalf("first batch: %v", err)
	}

	if created, err := svc.EnsureScopeRootsForAll(ctx, installed); err != nil || created != 0 {
		t.Fatalf("second batch created %d, err = %v; want 0 and nil", created, err)
	}
}

// A vault that refuses writes must be reported rather than reported as done, or the caller stops
// retrying and the plugin never gets a scope at all.
func TestABatchAgainstAnUnwritableVaultReportsIt(t *testing.T) {
	svc := NewPluginVaultSettings(readOnlyGrantVault{&revokeTestVault{data: domain.NewVaultData()}})

	created, err := svc.EnsureScopeRootsForAll(context.Background(),
		[]domainplugin.InstalledPlugin{{Manifest: *scopeManifest("com.example.sync")}})

	if err == nil {
		t.Fatal("an unwritable vault was reported as a successful scope creation")
	}
	if created != 0 {
		t.Errorf("created = %d, want 0 when nothing could be written", created)
	}
}

// A manifest with no display name still needs a folder the user can find, so it falls back to the
// plugin id rather than producing a nameless folder - which the folder validator would refuse and
// which nobody could tell apart from another nameless one.
func TestAScopeFolderFallsBackToThePluginIDForItsName(t *testing.T) {
	svc, data := scopeVault(t)

	if _, err := svc.EnsureScopeRoot(context.Background(), &domainplugin.Manifest{
		ID:           "com.example.sync",
		Capabilities: domainplugin.CapabilitySet{Scope: &domainplugin.ScopeCaps{}},
	}); err != nil {
		t.Fatalf("EnsureScopeRoot err = %v", err)
	}

	if len(data.Folders) != 1 || data.Folders[0].Name != "com.example.sync" {
		t.Fatalf("folders = %v, want one named after the plugin id", data.Folders)
	}
}
