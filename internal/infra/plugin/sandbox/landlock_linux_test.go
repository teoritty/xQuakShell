//go:build linux

package sandbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestHandledAccessFSTakesResponsibilityForEveryRightTheABIKnows(t *testing.T) {
	cases := []struct {
		abi  int
		want uint64
		why  string
	}{
		{abi: abiFilesystem, want: 0x1fff, why: "the 13 rights 5.13 shipped, and not one more"},
		{abi: abiRefer, want: 0x3fff, why: "5.19 added REFER"},
		{abi: abiTruncate, want: 0x7fff, why: "6.2 added TRUNCATE"},
		{abi: abiNetwork, want: 0x7fff, why: "6.7 added only network rights, no filesystem right"},
		{abi: abiIoctlDev, want: 0xffff, why: "6.10 added IOCTL_DEV"},
		{abi: abiIoctlDev + 4, want: 0xffff, why: "a kernel newer than this code knows is used as the newest it does"},
	}

	for _, tc := range cases {
		if got := handledAccessFS(tc.abi); got != tc.want {
			t.Errorf("handledAccessFS(%d) = %#x, want %#x; %s", tc.abi, got, tc.want, tc.why)
		}
	}
}

func TestHandledAccessNetIsEmptyUntilTheKernelHasNetworkRules(t *testing.T) {
	for abi := abiFilesystem; abi < abiNetwork; abi++ {
		if got := handledAccessNet(abi); got != 0 {
			t.Errorf("handledAccessNet(%d) = %#x, want 0; naming a network right on a kernel "+
				"without them makes landlock_create_ruleset fail with EINVAL and confines nothing",
				abi, got)
		}
	}
	want := uint64(unix.LANDLOCK_ACCESS_NET_BIND_TCP | unix.LANDLOCK_ACCESS_NET_CONNECT_TCP)
	if got := handledAccessNet(abiNetwork); got != want {
		t.Errorf("handledAccessNet(%d) = %#x, want %#x", abiNetwork, got, want)
	}
}

func TestEveryGrantIsASubsetOfWhatTheRulesetHandles(t *testing.T) {
	// A rule naming a right the ruleset does not handle is rejected outright, so a grant that
	// outgrew its ABI does not over-permit — it stops the plugin from starting at all.
	for abi := abiFilesystem; abi <= abiIoctlDev; abi++ {
		handled := handledAccessFS(abi)
		grants := map[string]uint64{
			"read-only":    accessReadOnly(abi),
			"read-execute": accessReadExecute(abi),
			"read-write":   accessReadWrite(abi),
		}
		for name, grant := range grants {
			if extra := grant &^ handled; extra != 0 {
				t.Errorf("abi %d: %s grant names %#x which the ruleset does not handle", abi, name, extra)
			}
		}
	}
}

func TestTheInstanceDirectoryGrantNeverIncludesExecute(t *testing.T) {
	for abi := abiFilesystem; abi <= abiIoctlDev; abi++ {
		if accessReadWrite(abi)&unix.LANDLOCK_ACCESS_FS_EXECUTE != 0 {
			t.Errorf("abi %d: the writable grant includes EXECUTE, so a plugin could write a "+
				"binary into its data directory and run it", abi)
		}
	}
}

// confinedChildEnv carries the probe layout to the child process. Landlock cannot be undone, so
// applyLandlock can only be exercised in a process the suite is willing to lose.
const confinedChildEnv = "XQS_TEST_LANDLOCK_CHILD"

func TestApplyLandlockLeavesTheProcessAbleToReachOnlyWhatItWasGranted(t *testing.T) {
	runConfinedChild(t, "TestConfinedChildProbesItsBoundary")
}

// TestASecondRulesetCannotWidenTheFirst pins the property the shim's safety rests on.
//
// A Landlock domain only ever narrows: a process already inside one that adds a ruleset granting
// more gets the intersection, not the union. That is what makes it safe for the shim to exec an
// untrusted binary into the confinement — the plugin can re-invoke the shim, or call the syscalls
// itself, and cannot talk its way back out. If the kernel ever stopped behaving this way, the shim
// would be a suggestion rather than a boundary, and this is where that would show up.
func TestASecondRulesetCannotWidenTheFirst(t *testing.T) {
	runConfinedChild(t, "TestConfinedChildCannotGrantItselfMore")
}

func runConfinedChild(t *testing.T, name string) {
	t.Helper()
	if _, err := landlockABI(); err != nil {
		t.Skipf("kernel has no usable Landlock: %v", err)
	}
	layout := newProbeLayout(t)

	cmd := exec.Command(os.Args[0], "-test.run="+name, "-test.v")
	cmd.Env = append(os.Environ(), confinedChildEnv+"="+strings.Join(layout, string(os.PathListSeparator)))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the confined child did not agree with its boundary: %v\n%s", err, out)
	}
}

// childLayout is the child's side of the environment variable: the directories it was granted and
// the one it must not reach.
type childLayout struct {
	root, install, instance, outside string
}

func readChildLayout(t *testing.T, driver string) (childLayout, bool) {
	t.Helper()
	spec := os.Getenv(confinedChildEnv)
	if spec == "" {
		t.Skip("child-process body, driven by " + driver)
		return childLayout{}, false
	}
	p := strings.Split(spec, string(os.PathListSeparator))
	return childLayout{root: p[0], install: p[1], instance: p[2], outside: p[3]}, true
}

func (l childLayout) shimArgs() ShimArgs {
	return ShimArgs{
		DataRoot: l.root,
		AllowRX:  []string{l.install},
		AllowRW:  []string{l.instance},
		Exec:     filepath.Join(l.install, "plugin"),
	}
}

// newProbeLayout builds the directories the child is granted and the one it must not reach, and
// returns them in the order the child reads them.
func newProbeLayout(t *testing.T) []string {
	t.Helper()
	root := t.TempDir()
	install := filepath.Join(root, "install")
	instance := filepath.Join(root, "data", "sess-1")
	outside := filepath.Join(root, "elsewhere")
	for _, dir := range []string{install, instance, outside} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}
	for _, file := range []string{filepath.Join(install, "plugin.json"), filepath.Join(outside, "id_ed25519")} {
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}
	return []string{root, install, instance, outside}
}

// TestConfinedChildProbesItsBoundary is the body of the child process the test above starts, not a
// test of its own: without the environment variable it has nothing to probe and skips.
func TestConfinedChildProbesItsBoundary(t *testing.T) {
	layout, ok := readChildLayout(t, "TestApplyLandlockLeavesTheProcessAbleToReachOnlyWhatItWasGranted")
	if !ok {
		return
	}
	abi, err := landlockABI()
	if err != nil {
		t.Fatalf("probe landlock: %v", err)
	}
	if err := applyLandlock(abi, layout.shimArgs()); err != nil {
		t.Fatalf("applyLandlock: %v", err)
	}

	if _, err := os.ReadFile(filepath.Join(layout.install, "plugin.json")); err != nil {
		t.Errorf("reading its own installed file failed: %v; the plugin could not start", err)
	}
	if err := os.WriteFile(filepath.Join(layout.instance, "state"), []byte("x"), 0o600); err != nil {
		t.Errorf("writing its own instance directory failed: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(layout.outside, "id_ed25519")); !errors.Is(err, os.ErrPermission) {
		t.Errorf("reading outside the grants returned %v, want a permission error; this is the "+
			"whole point of the ruleset", err)
	}
	if err := os.WriteFile(filepath.Join(layout.outside, "dropped"), []byte("x"), 0o600); !errors.Is(err, os.ErrPermission) {
		t.Errorf("writing outside the grants returned %v, want a permission error", err)
	}
}

// TestConfinedChildCannotGrantItselfMore is the body of the second child process: it confines
// itself, then does what a hostile plugin would do and applies a ruleset that grants the directory
// the first one withheld.
func TestConfinedChildCannotGrantItselfMore(t *testing.T) {
	layout, ok := readChildLayout(t, "TestASecondRulesetCannotWidenTheFirst")
	if !ok {
		return
	}
	abi, err := landlockABI()
	if err != nil {
		t.Fatalf("probe landlock: %v", err)
	}
	if err := applyLandlock(abi, layout.shimArgs()); err != nil {
		t.Fatalf("applyLandlock: %v", err)
	}

	wider := layout.shimArgs()
	wider.AllowRW = append(wider.AllowRW, layout.outside)
	if err := applyLandlock(abi, wider); err != nil {
		t.Fatalf("applying a second, wider ruleset failed outright: %v; the interesting answer is "+
			"that it succeeds and changes nothing", err)
	}

	if _, err := os.ReadFile(filepath.Join(layout.outside, "id_ed25519")); !errors.Is(err, os.ErrPermission) {
		t.Errorf("after granting itself the directory, reading it returned %v, want a permission "+
			"error; a domain that can be widened from inside is not a boundary, and the shim's "+
			"whole design assumes it cannot be", err)
	}
}
