//go:build windows

package plugin

import (
	"os"
	"path/filepath"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/sandbox"
)

// probeInstall lays out a plugin the way an installed one sits on disk, which is the only layout
// where this test means anything: the data directory is a CHILD of the install directory, so an ACE
// written one level too high reaches every session's files.
func probeInstall(t *testing.T, dataRoot, pluginID string) (domainplugin.InstalledPlugin, string) {
	t.Helper()
	root := filepath.Join(PluginsRoot(dataRoot), pluginID)
	for _, dir := range []string{filepath.Join(root, "bin"), filepath.Join(root, "ui")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	plugin := domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{ID: pluginID, Isolation: domainplugin.IsolationPerSession},
		RootDir:  root,
	}
	instanceDir, err := EnsurePluginInstanceDataDir(dataRoot, pluginID, "sess-a", domainplugin.IsolationPerSession)
	if err != nil {
		t.Fatal(err)
	}
	return plugin, instanceDir
}

// TestAPreparedContainerHoldsTwoGrantsAndNoOthers is the ACL statement of the whole Windows design.
//
// A container that can read its own installed files and write its own instance directory is what a
// plugin needs. An ACE anywhere else under the data root is the failure mode this phase exists to
// avoid, and the one it nearly shipped: the install directory is the PARENT of every session's data
// directory, so granting it as one path put this session's SID on all of them.
func TestAPreparedContainerHoldsTwoGrantsAndNoOthers(t *testing.T) {
	if !sandbox.Support().Available {
		t.Skip("this build cannot create an AppContainer")
	}
	const pluginID = "com.xquakshell.acl-probe"
	dataRoot := t.TempDir()
	plugin, instanceDir := probeInstall(t, dataRoot, pluginID)
	otherSession, err := EnsurePluginInstanceDataDir(dataRoot, pluginID, "sess-b", domainplugin.IsolationPerSession)
	if err != nil {
		t.Fatal(err)
	}

	container, err := prepareContainer(plugin, dataRoot, "sess-a", instanceDir)
	if err != nil {
		t.Fatalf("prepareContainer: %v", err)
	}
	t.Cleanup(func() {
		if err := releaseContainer(plugin, dataRoot, "sess-a"); err != nil {
			t.Errorf("the profile was left behind: %v", err)
		}
	})

	granted := map[string]uint32{
		filepath.Join(plugin.RootDir, "bin"):         sandbox.AccessReadExecute,
		filepath.Join(plugin.RootDir, "ui"):          sandbox.AccessReadExecute,
		filepath.Join(plugin.RootDir, "plugin.json"): sandbox.AccessReadExecute,
		instanceDir: sandbox.AccessReadWrite,
	}
	for path, mask := range granted {
		held, err := container.HasGrant(path, mask)
		if err != nil {
			t.Fatalf("HasGrant(%s): %v", path, err)
		}
		if !held {
			t.Errorf("the container has no grant on %s; the plugin cannot use its own files", path)
		}
	}

	// The three that must carry nothing. The data base directory is the one that matters most: it
	// is the parent every session's directory hangs off, and an ACE there is an ACE on all of them.
	forbidden := []string{
		PluginDataDir(dataRoot, pluginID),
		otherSession,
		plugin.RootDir,
	}
	for _, path := range forbidden {
		// Any right at all, not just the ones granted: the question is whether this container's SID
		// appears in that ACL, not how much it was given.
		for _, mask := range []uint32{sandbox.AccessReadExecute, sandbox.AccessReadWrite} {
			held, err := container.HasGrant(path, mask)
			if err != nil {
				t.Fatalf("HasGrant(%s): %v", path, err)
			}
			if held {
				t.Errorf("the container holds %#x on %s, which is outside the two directories a "+
					"plugin instance is entitled to", mask, path)
			}
		}
	}
}

// TestAConfinedPluginsTempDirectoryStaysInsideItsInstanceDirectory pins the property that made the
// substituted LOCALAPPDATA worth doing.
//
// Windows computes an AppContainer's TEMP from its LOCALAPPDATA and overwrites whatever the parent
// set. Handing over the user's real profile path would have put every plugin's temporary files back
// in a directory shared with the rest of the machine — undoing #76 — and disclosed where the user's
// account lives for no reason.
func TestAConfinedPluginsTempDirectoryStaysInsideItsInstanceDirectory(t *testing.T) {
	if !sandbox.Support().Available {
		t.Skip("this build cannot create an AppContainer")
	}
	const pluginID = "com.xquakshell.temp-probe"
	dataRoot := t.TempDir()
	plugin, instanceDir := probeInstall(t, dataRoot, pluginID)

	if _, err := prepareContainer(plugin, dataRoot, "sess-a", instanceDir); err != nil {
		t.Fatalf("prepareContainer: %v", err)
	}
	t.Cleanup(func() { _ = releaseContainer(plugin, dataRoot, "sess-a") })

	tempDir := PluginInstanceEffectiveTempDir(plugin, "sess-a", instanceDir)
	if !pathIsUnder(instanceDir, tempDir) {
		t.Errorf("the plugin's temp directory %s is outside its instance directory %s", tempDir, instanceDir)
	}
	// It also has to exist before the process starts: Windows creates this tree only under the real
	// LOCALAPPDATA, so with a substituted one the plugin's first temporary file fails with "path
	// not found" — which reads as a broken plugin rather than a missing directory.
	if info, err := os.Stat(tempDir); err != nil || !info.IsDir() {
		t.Errorf("stat(%s) = %v, %v; the container computes this path and nothing else creates it",
			tempDir, info, err)
	}
}

func pathIsUnder(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) &&
		(len(rel) < 2 || rel[:2] != "..")
}

// grantState reads whether the container still holds a grant, failing the test if the question
// itself cannot be answered — an unreadable DACL is not the same as an absent ACE, and reporting it
// as one would turn a broken check into a passing test.
func grantState(t *testing.T, container *sandbox.Container, path string, mask uint32) bool {
	t.Helper()
	held, err := container.HasGrant(path, mask)
	if err != nil {
		t.Fatalf("read the dacl of %s: %v", path, err)
	}
	return held
}

// TestReleasingASessionScopedContainerTakesItsAccessBackWithIt is what keeps a plugin installable
// for the life of the installation rather than for the first fifteen hundred connections.
//
// A session-scoped container's name comes from a fresh random session id, so its SID is used once
// and never again. Deleting the profile without revoking its ACEs left one permanent entry per
// connection on every install-tree path; a DACL is capped at 64 KB, and past that the grant fails,
// which fails the start — for good, and surviving a reinstall, because the ACL belongs to the plugin
// directory and not to the application.
func TestReleasingASessionScopedContainerTakesItsAccessBackWithIt(t *testing.T) {
	if !sandbox.Support().Available {
		t.Skip("this build cannot create an AppContainer")
	}
	const pluginID = "com.xquakshell.revoke-probe"
	dataRoot := t.TempDir()
	plugin, instanceDir := probeInstall(t, dataRoot, pluginID)
	binDir := filepath.Join(plugin.RootDir, "bin")

	container, err := prepareContainer(plugin, dataRoot, "sess-a", instanceDir)
	if err != nil {
		t.Fatalf("prepareContainer: %v", err)
	}
	t.Cleanup(func() { _ = releaseContainer(plugin, dataRoot, "sess-a") })

	if !grantState(t, container, binDir, sandbox.AccessReadExecute) {
		t.Fatalf("%s carries no grant before the release; this test would prove nothing", binDir)
	}

	if err := releaseContainer(plugin, dataRoot, "sess-a"); err != nil {
		t.Fatalf("releaseContainer: %v", err)
	}

	if grantState(t, container, binDir, sandbox.AccessReadExecute) {
		t.Errorf("%s still names the container SID after its profile was deleted; the ACE outlived "+
			"the identity it was written for and nothing will ever remove it", binDir)
	}
	if grantState(t, container, instanceDir, sandbox.AccessReadWrite) {
		t.Errorf("%s still names the container SID after its profile was deleted", instanceDir)
	}
}

// TestReleasingAPerPluginContainerKeepsTheAccessItWillNeedAgain is the other half of the rule, and
// it is a test rather than a comment because the two are one line apart.
//
// A per-plugin container answers to the same name at every start for the life of the installation.
// Revoking its ACEs on the way out would mean rewriting the ACL of the whole install subtree on the
// way back in, every single start, to restore exactly what was just removed — the cost EnsureGrant
// exists to avoid, reintroduced by a cleanup aimed at a problem this mode does not have.
func TestReleasingAPerPluginContainerKeepsTheAccessItWillNeedAgain(t *testing.T) {
	if !sandbox.Support().Available {
		t.Skip("this build cannot create an AppContainer")
	}
	const pluginID = "com.xquakshell.keep-probe"
	dataRoot := t.TempDir()
	plugin, _ := probeInstall(t, dataRoot, pluginID)
	plugin.Manifest.Isolation = domainplugin.IsolationPerPlugin
	binDir := filepath.Join(plugin.RootDir, "bin")

	instanceDir, err := EnsurePluginInstanceDataDir(dataRoot, pluginID, "sess-a", domainplugin.IsolationPerPlugin)
	if err != nil {
		t.Fatal(err)
	}
	container, err := prepareContainer(plugin, dataRoot, "sess-a", instanceDir)
	if err != nil {
		t.Fatalf("prepareContainer: %v", err)
	}
	t.Cleanup(func() { _ = releaseContainer(plugin, dataRoot, "sess-a") })

	if err := releaseContainer(plugin, dataRoot, "sess-a"); err != nil {
		t.Fatalf("releaseContainer: %v", err)
	}

	if !grantState(t, container, binDir, sandbox.AccessReadExecute) {
		t.Errorf("%s lost its grant when the profile was deleted; the next start of this plugin "+
			"has to rewrite the ACL of the entire install tree to put back what it just had", binDir)
	}
}
