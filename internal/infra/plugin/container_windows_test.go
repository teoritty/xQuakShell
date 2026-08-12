//go:build windows

package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

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

// discardCloser stands in for the redacting stderr writer, which a spawn requires and this test has
// no use for.
type discardCloser struct{}

func (discardCloser) Write(p []byte) (int, error) { return len(p), nil }
func (discardCloser) Close() error                { return nil }

// TestASpawnIsNotRacedByATeardownOfTheSameContainer covers the window between creating a profile and
// starting a process inside it.
//
// A delete landing in that window fails CreateProcess with "the system cannot find the file
// specified" — the file it cannot find is the profile, not the plugin binary, so the error points at
// the one thing that is not wrong. It is reached by ordinary use: a Stop, a crash teardown or a
// supervisor restart runs the delete while another Start is running the create.
//
// The deleter runs flat out for as long as the spawns take rather than a fixed number of times,
// because a race is only demonstrated by a detector dense enough to land inside the window. With the
// serialisation removed this fails on the first or second spawn; with it in place the window does not
// exist.
func TestASpawnIsNotRacedByATeardownOfTheSameContainer(t *testing.T) {
	if !sandbox.Support().Available {
		t.Skip("this build cannot create an AppContainer")
	}
	const pluginID = "com.xquakshell.spawn-race-probe"
	dataRoot := t.TempDir()
	plugin, instanceDir := probeInstall(t, dataRoot, pluginID)
	entry := copySystemShell(t, filepath.Join(plugin.RootDir, "bin"))
	t.Cleanup(func() { _ = releaseContainer(plugin, dataRoot, "sess-a") })

	done := make(chan struct{})
	deleted := make(chan struct{})
	go func() {
		defer close(deleted)
		for {
			select {
			case <-done:
				return
			default:
			}
			_ = releaseContainer(plugin, dataRoot, "sess-a")
		}
	}()

	req := childRequest{
		dataRoot:        dataRoot,
		plugin:          plugin,
		sessionID:       "sess-a",
		entryPath:       entry,
		instanceDataDir: instanceDir,
		env:             PluginProcessEnv(instanceDir, pluginID, "sess-a"),
		stderr:          discardCloser{},
	}
	for i := range 5 {
		started, err := startContainedChild(req, sandbox.Support())
		if err != nil {
			close(done)
			<-deleted
			t.Fatalf("spawn %d raced a teardown of its own container: %v", i, err)
		}
		_ = started.stdin.Close()
		_ = started.child.Kill()
	}
	close(done)
	<-deleted
}

// copySystemShell puts a real executable inside the plugin's install tree, because the spawn under
// test needs a process that actually starts: a container that cannot run its image fails for a
// reason this test is not about.
func copySystemShell(t *testing.T, dir string) string {
	t.Helper()
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatalf("locate the system directory: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(system, "cmd.exe"))
	if err != nil {
		t.Fatalf("read the system shell: %v", err)
	}
	entry := filepath.Join(dir, "plugin.exe")
	if err := os.WriteFile(entry, body, 0o700); err != nil {
		t.Fatalf("write the probe binary: %v", err)
	}
	return entry
}
