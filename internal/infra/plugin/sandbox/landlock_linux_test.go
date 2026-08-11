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
	if _, err := landlockABI(); err != nil {
		t.Skipf("kernel has no usable Landlock: %v", err)
	}
	layout := newProbeLayout(t)

	cmd := exec.Command(os.Args[0], "-test.run=TestConfinedChildProbesItsBoundary", "-test.v")
	cmd.Env = append(os.Environ(), confinedChildEnv+"="+strings.Join(layout, string(os.PathListSeparator)))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the confined child did not agree with its boundary: %v\n%s", err, out)
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
	spec := os.Getenv(confinedChildEnv)
	if spec == "" {
		t.Skip("child-process body, driven by TestApplyLandlockLeavesTheProcessAbleToReachOnlyWhatItWasGranted")
	}
	parts := strings.Split(spec, string(os.PathListSeparator))
	root, install, instance, outside := parts[0], parts[1], parts[2], parts[3]

	abi, err := landlockABI()
	if err != nil {
		t.Fatalf("probe landlock: %v", err)
	}
	args := ShimArgs{
		DataRoot: root,
		AllowRX:  []string{install},
		AllowRW:  []string{instance},
		Exec:     filepath.Join(install, "plugin"),
	}
	if err := applyLandlock(abi, args); err != nil {
		t.Fatalf("applyLandlock: %v", err)
	}

	if _, err := os.ReadFile(filepath.Join(install, "plugin.json")); err != nil {
		t.Errorf("reading its own installed file failed: %v; the plugin could not start", err)
	}
	if err := os.WriteFile(filepath.Join(instance, "state"), []byte("x"), 0o600); err != nil {
		t.Errorf("writing its own instance directory failed: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(outside, "id_ed25519")); !errors.Is(err, os.ErrPermission) {
		t.Errorf("reading outside the grants returned %v, want a permission error; this is the "+
			"whole point of the ruleset", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "dropped"), []byte("x"), 0o600); !errors.Is(err, os.ErrPermission) {
		t.Errorf("writing outside the grants returned %v, want a permission error", err)
	}
}
