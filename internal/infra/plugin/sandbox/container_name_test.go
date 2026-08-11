package sandbox_test

import (
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/infra/plugin/sandbox"
)

// instance is one plugin process's identity, in the terms the host has when it spawns it.
type instance struct {
	pluginID  string
	sessionID string
	isolation domainplugin.IsolationMode
}

func (i instance) key() string {
	return infraplugin.PluginInstanceKey(i.pluginID, i.sessionID, i.isolation)
}

func (i instance) dataDir(dataRoot string) string {
	return infraplugin.PluginInstanceDataDir(dataRoot, i.pluginID, i.sessionID, i.isolation)
}

// adversarialInstances are the shapes a plugin author could choose deliberately, plus the session
// pairs the host produces on its own. Most of them collapse onto one string under any plausible
// sanitising function — too long, wrong charset, path-like, or differing only past a length limit —
// which is the reason the container name is a hash of the identity and not a cleaned-up copy of it.
func adversarialInstances() []instance {
	long := strings.Repeat("a", 300)
	perPlugin := domainplugin.IsolationPerPlugin
	perSession := domainplugin.IsolationPerSession
	return []instance{
		{pluginID: "com.example.a", isolation: perPlugin},
		{pluginID: "com.example.b", isolation: perPlugin},
		{pluginID: "com.example", isolation: perPlugin},
		{pluginID: "com.example.", isolation: perPlugin},
		{pluginID: "com.example..", isolation: perPlugin},
		{pluginID: "com/example", isolation: perPlugin},
		{pluginID: `com\example`, isolation: perPlugin},
		{pluginID: "com example", isolation: perPlugin},
		{pluginID: "com.example.ф", isolation: perPlugin},
		{pluginID: "com.example.Ф", isolation: perPlugin},
		{pluginID: long + "x", isolation: perPlugin},
		{pluginID: long + "y", isolation: perPlugin},
		{pluginID: "com.example", sessionID: "sess-a", isolation: perSession},
		{pluginID: "com.example", sessionID: "sess-b", isolation: perSession},
		// The pair the length prefix exists for: without it, ("com.example", "x") and
		// ("com.exampl", "ex") render to the same joined string.
		{pluginID: "com.example", sessionID: "x", isolation: perSession},
		{pluginID: "com.exampl", sessionID: "ex", isolation: perSession},
	}
}

func TestContainerNameNeverCollidesForDistinctInstances(t *testing.T) {
	seen := map[string]instance{}
	for _, inst := range adversarialInstances() {
		name := sandbox.ContainerName(inst.key())
		if previous, clash := seen[name]; clash {
			t.Errorf("%+v and %+v both derive container name %q; two instances sharing a container "+
				"share its SID, and therefore every ACE granted to it", previous, inst, name)
		}
		seen[name] = inst
	}
}

// TestTwoInstancesWithDifferentDataDirectoriesNeverShareAContainer is the property that makes the
// container safe to attach ACEs to.
//
// The container SID is what a data directory's ACL names, so the container must be at least as
// fine-grained as the directory. If two instances with different directories shared one SID, the
// ACL could no longer tell them apart and each would read the other's files — which is the
// isolation ADR-003 established, undone by the mechanism meant to reinforce it.
func TestTwoInstancesWithDifferentDataDirectoriesNeverShareAContainer(t *testing.T) {
	const dataRoot = `C:\data`
	all := adversarialInstances()
	for i, a := range all {
		for _, b := range all[i+1:] {
			if a.dataDir(dataRoot) == b.dataDir(dataRoot) {
				continue
			}
			if sandbox.ContainerName(a.key()) == sandbox.ContainerName(b.key()) {
				t.Errorf("%+v and %+v have different data directories and the same container", a, b)
			}
		}
	}
}

// TestPerPluginIsolationIgnoresTheSessionOnBothSides pins the other half of the agreement: where
// the data directory is not session-scoped, the container must not be either, or a per-plugin
// plugin would get a new profile per session and none of them would hold the ACE on the directory
// the first one was granted.
func TestPerPluginIsolationIgnoresTheSessionOnBothSides(t *testing.T) {
	a := instance{pluginID: "com.example", sessionID: "sess-a", isolation: domainplugin.IsolationPerPlugin}
	b := instance{pluginID: "com.example", sessionID: "sess-b", isolation: domainplugin.IsolationPerPlugin}

	if a.dataDir(`C:\data`) != b.dataDir(`C:\data`) {
		t.Fatalf("per-plugin isolation gave two sessions different data directories: %q and %q",
			a.dataDir(`C:\data`), b.dataDir(`C:\data`))
	}
	if got, want := sandbox.ContainerName(a.key()), sandbox.ContainerName(b.key()); got != want {
		t.Errorf("per-plugin isolation gave two sessions different containers (%q and %q) for one "+
			"data directory; only one of them could hold the ACE on it", got, want)
	}
}

func TestContainerNameIsStableAndFitsAProfileName(t *testing.T) {
	identity := instance{pluginID: "com.example", sessionID: "sess-a", isolation: domainplugin.IsolationPerSession}.key()

	first := sandbox.ContainerName(identity)
	if second := sandbox.ContainerName(identity); first != second {
		t.Fatalf("ContainerName is not deterministic: %q then %q; the profile created at start "+
			"could not be found again at teardown", first, second)
	}
	if len(first) != 33 {
		t.Errorf("ContainerName(%q) = %q, length %d, want 33", identity, first, len(first))
	}
	for i, r := range first {
		digit := r >= '0' && r <= '9'
		letter := r >= 'a' && r <= 'z'
		if !digit && !letter {
			t.Fatalf("ContainerName(%q) = %q contains %q, outside the profile-name charset",
				identity, first, string(r))
		}
		if i == 0 && !letter {
			t.Errorf("ContainerName starts with %q; a profile name is also a registry key and a "+
				"directory under Packages, and a leading digit is the kind of thing Windows "+
				"objects to late", string(r))
		}
	}
}
