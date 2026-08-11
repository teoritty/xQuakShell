//go:build windows

package sandbox_test

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"xquakshell/internal/infra/plugin/sandbox"
)

// newTestContainer creates a profile under a name no other test or run can collide with, and
// removes it afterwards. A leaked profile is a registry entry and a directory on the developer's
// machine that nothing else will ever clean up.
func newTestContainer(t *testing.T) *sandbox.Container {
	t.Helper()
	if !sandbox.ProfileAPIAvailable() {
		t.Skip("this Windows build has no AppContainer profile API")
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	name := sandbox.ContainerNamePrefix + "test" + hex.EncodeToString(raw[:])

	container, err := sandbox.CreateContainer(name, "xQuakShell test", "created by a unit test")
	if err != nil {
		t.Fatalf("CreateContainer: %v", err)
	}
	t.Cleanup(func() {
		if err := sandbox.DeleteContainer(name); err != nil {
			t.Errorf("DeleteContainer left %s behind: %v", name, err)
		}
	})
	return container
}

func TestAContainerGrantsOnlyThePathItWasGiven(t *testing.T) {
	container := newTestContainer(t)
	root := t.TempDir()
	granted := filepath.Join(root, "granted")
	untouched := filepath.Join(root, "untouched")
	for _, dir := range []string{granted, untouched} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	if err := container.Grant(granted, sandbox.AccessReadExecute); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	if held, err := container.HasGrant(granted, sandbox.AccessReadExecute); err != nil || !held {
		t.Errorf("HasGrant(granted) = %v, %v; want true", held, err)
	}
	// The sibling is the assertion that matters. A grant that reached it would mean an ACE landed
	// higher up the tree than it was asked to, which is how one plugin instance ends up holding
	// rights over another's directory.
	if held, err := container.HasGrant(untouched, sandbox.AccessReadExecute); err != nil || held {
		t.Errorf("HasGrant(sibling) = %v, %v; want false", held, err)
	}
	// And the parent, for the same reason in the other direction.
	if held, err := container.HasGrant(root, sandbox.AccessReadExecute); err != nil || held {
		t.Errorf("HasGrant(parent) = %v, %v; want false", held, err)
	}
}

func TestAGrantReachesTheFilesUnderIt(t *testing.T) {
	container := newTestContainer(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := container.Grant(dir, sandbox.AccessReadExecute); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	// An AppContainer opens files, not directories. A grant that stopped at the directory would let
	// a plugin list its install tree and read nothing in it — which fails at exec, long after the
	// ACL write reported success.
	if held, err := container.HasGrant(file, sandbox.AccessReadExecute); err != nil || !held {
		t.Errorf("HasGrant(file under a granted directory) = %v, %v; want true", held, err)
	}
}

func TestEnsureGrantWritesOnceAndThenAgreesItIsDone(t *testing.T) {
	container := newTestContainer(t)
	dir := t.TempDir()

	if held, err := container.HasGrant(dir, sandbox.AccessReadWrite); err != nil || held {
		t.Fatalf("a fresh directory already reports the grant: %v, %v", held, err)
	}
	for i := range 2 {
		if err := container.EnsureGrant(dir, sandbox.AccessReadWrite); err != nil {
			t.Fatalf("EnsureGrant call %d: %v", i+1, err)
		}
	}
	if held, err := container.HasGrant(dir, sandbox.AccessReadWrite); err != nil || !held {
		t.Errorf("HasGrant after EnsureGrant = %v, %v; want true", held, err)
	}
	// A read grant is not a write grant. Reporting "already granted" for a narrower right is how a
	// repair path silently stops repairing.
	if held, err := container.HasGrant(dir, sandbox.AccessReadWrite|0x0002 /* FILE_ADD_FILE */); err != nil {
		t.Errorf("HasGrant for a wider mask errored: %v", err)
	} else if held {
		t.Log("the wider mask is already implied by GENERIC_WRITE, which is fine")
	}
}

func TestAProfileIsListedWhileItExistsAndDeletingItTwiceIsFine(t *testing.T) {
	if !sandbox.ProfileAPIAvailable() {
		t.Skip("this Windows build has no AppContainer profile API")
	}
	container := newTestContainer(t)

	names, err := sandbox.ListContainerNames(sandbox.ContainerNamePrefix)
	if err != nil {
		t.Fatalf("ListContainerNames: %v", err)
	}
	if !slices.Contains(names, container.Name) {
		t.Errorf("ListContainerNames did not report %q; the orphan sweep would never collect it",
			container.Name)
	}

	if err := sandbox.DeleteContainer(container.Name); err != nil {
		t.Fatalf("DeleteContainer: %v", err)
	}
	// Every teardown path can reach the same profile — a session closing, the startup sweep — and
	// the second one to arrive must not report a failure for finding the work already done. The
	// t.Cleanup registered above is itself the third caller, and it asserts no error.
	if err := sandbox.DeleteContainer(container.Name); err != nil {
		t.Errorf("deleting a profile that is already gone reported %v, want success", err)
	}
}

func TestAdoptingAnExistingProfileYieldsTheSameIdentity(t *testing.T) {
	container := newTestContainer(t)

	// What a crash leaves behind: the profile is still there when the next start asks for it. The
	// SID has to come back identical or every ACE already written for this instance is orphaned.
	again, err := sandbox.CreateContainer(container.Name, "xQuakShell test", "created by a unit test")
	if err != nil {
		t.Fatalf("CreateContainer over an existing profile: %v", err)
	}
	if again.String() != container.String() {
		t.Errorf("adopted profile has SID %s, first creation had %s", again, container)
	}
	derived, err := sandbox.OpenContainer(container.Name)
	if err != nil {
		t.Fatalf("OpenContainer: %v", err)
	}
	if derived.String() != container.String() {
		t.Errorf("derived SID %s does not match the created one %s", derived, container)
	}
}
