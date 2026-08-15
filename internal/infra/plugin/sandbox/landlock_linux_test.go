//go:build linux

package sandbox

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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
	// The other half of the condition, which went unwritten for a while: a kernel that HAS network
	// rules must have them named. Only asserting the empty case leaves a handledAccessNet that
	// returns 0 forever passing, and a ruleset that handles no network right confines no socket.
	want := uint64(unix.LANDLOCK_ACCESS_NET_BIND_TCP | unix.LANDLOCK_ACCESS_NET_CONNECT_TCP)
	for _, abi := range []int{abiNetwork, abiIoctlDev, abiIoctlDev + 4} {
		if got := handledAccessNet(abi); got != want {
			t.Errorf("handledAccessNet(%d) = %#x, want %#x; a kernel at or above %d governs TCP "+
				"and a ruleset that does not say so leaves it open", abi, got, want, abiNetwork)
		}
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

// TestTheShimWillNotExecFromAThreadItDidNotConfine covers the guard the shim relies on, and does it
// without depending on the scheduler.
//
// Landlock commits its domain onto a thread, not onto the process, and Go moves a goroutine between
// threads at any scheduling point. The shim therefore records which thread it confined and refuses
// to exec from any other, because exec'ing from an unconfined thread starts the plugin with no
// restrictions at all while the shim exits zero and the host reports it as sandboxed.
//
// Provoking a real migration would make this a test of the scheduler's mood. The refusal itself is
// what has to be correct, so it is asked directly: the thread it was given, and a thread it was not.
func TestTheShimWillNotExecFromAThreadItDidNotConfine(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	here := unix.Gettid()
	if err := confinementStillHolds(here); err != nil {
		t.Errorf("confinementStillHolds(%d) = %v on that very thread; the shim would refuse every "+
			"correct exec and no plugin would ever start", here, err)
	}

	// Any tid this thread is not. Negative rather than "here+1", which the kernel may well have
	// handed to a real thread of this process.
	if err := confinementStillHolds(-1); err == nil {
		t.Error("confinementStillHolds(-1) = nil; the shim would exec the plugin from a thread that " +
			"was never confined and report it as sandboxed")
	}
}

func runConfinedChild(t *testing.T, name string) {
	t.Helper()
	if _, err := landlockABI(); err != nil {
		t.Skipf("kernel has no usable Landlock: %v", err)
	}
	layout, dial := newProbeLayout(t)

	cmd := exec.Command(os.Args[0], "-test.run="+name, "-test.v")
	cmd.Env = append(os.Environ(),
		confinedChildEnv+"="+strings.Join(layout, string(os.PathListSeparator)),
		confinedChildDialEnv+"="+dial)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the confined child did not agree with its boundary: %v\n%s", err, out)
	}
}

// TestAConfinedChildCannotOpenATCPConnectionWhereTheKernelHasNetworkRules is the only automated
// check of the network half of this confinement, and on most machines it does not run.
//
// Landlock governs TCP from ABI 4 (kernel 6.7) onward, and applyLandlock adds no network rule at
// all: naming the right in the ruleset and granting it to nothing is what denies every connect. The
// mask tests above assert the naming; only this one asserts that the kernel then refuses a socket.
//
// Support() reports Network false on every Linux kernel by design — Landlock covers no UDP and no
// raw socket, so the product never claims whole network confinement — which means no test driven by
// the exported contract can ever reach this. It has to be asked of the ruleset directly.
func TestAConfinedChildCannotOpenATCPConnectionWhereTheKernelHasNetworkRules(t *testing.T) {
	abi, err := landlockABI()
	if err != nil {
		t.Skipf("kernel has no usable Landlock (%v); its network rules are UNVERIFIED here", err)
	}
	if abi < abiNetwork {
		t.Skipf("this kernel reports Landlock ABI %d and TCP rules need ABI %d, so the network "+
			"half of the confinement is UNVERIFIED on this machine; it runs where the kernel is "+
			"6.7 or newer", abi, abiNetwork)
	}
	runConfinedChild(t, "TestConfinedChildCannotDialOut")
}

// childLayout is the child's side of the environment variables: the directories it was granted, the
// one it must not reach, and an address that answers until the ruleset stops it being reachable.
type childLayout struct {
	root, install, instance, outside, dial string
}

// confinedChildDialEnv carries the listener address on its own, and deliberately not in the
// path list above. On Linux the path list separator is ':', which is also what separates a host
// from its port, so "127.0.0.1:39121" split into two elements and the child dialled an address
// with no port. It cost a CI run to find because on Windows the separator is ';'.
const confinedChildDialEnv = "XQS_TEST_LANDLOCK_DIAL"

func readChildLayout(t *testing.T, driver string) (childLayout, bool) {
	t.Helper()
	spec := os.Getenv(confinedChildEnv)
	if spec == "" {
		t.Skip("child-process body, driven by " + driver)
		return childLayout{}, false
	}

	// Every child below applies Landlock and then keeps working in the same process, which is the
	// one thing production never does — the shim execs instead, and guards the thread rather than
	// holding it. A test that carries on has to hold the thread, or it is asserting against whatever
	// thread the scheduler last handed it: that is how the network child came to fail on a loaded
	// runner and pass on a retry of the same commit.
	//
	// Never unlocked. These processes exist to be confined and then to exit, and the lock costs them
	// nothing: they run under no RLIMIT_DATA, which is the whole reason the shim cannot do the same.
	runtime.LockOSThread()

	p := strings.Split(spec, string(os.PathListSeparator))
	return childLayout{
		root: p[0], install: p[1], instance: p[2], outside: p[3],
		dial: os.Getenv(confinedChildDialEnv),
	}, true
}

func (l childLayout) shimArgs() ShimArgs {
	return ShimArgs{
		DataRoot: l.root,
		AllowRX:  []string{l.install},
		AllowRW:  []string{l.instance},
		Exec:     filepath.Join(l.install, "plugin"),
	}
}

// newProbeLayout builds the directories the child is granted, the one it must not reach and a
// listener it may try to reach, and returns them in the order the child reads them.
//
// The listener belongs to the parent and stays open for as long as the child runs, so a refused
// connection is the ruleset's doing and not a socket that had already gone away.
func newProbeLayout(t *testing.T) ([]string, string) {
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for the confined child to dial: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	return []string{root, install, instance, outside}, listener.Addr().String()
}

// TestConfinedChildCannotDialOut is the body of the child process started above, not a test of its
// own: without the environment variable it has nothing to probe and skips.
func TestConfinedChildCannotDialOut(t *testing.T) {
	layout, ok := readChildLayout(t, "TestAConfinedChildCannotOpenATCPConnectionWhereTheKernelHasNetworkRules")
	if !ok {
		return
	}
	abi, err := landlockABI()
	if err != nil {
		t.Fatalf("probe landlock: %v", err)
	}

	// The control, in the same process and against the same address: a dial that was already
	// failing would make every assertion below pass for the wrong reason.
	conn, err := net.DialTimeout("tcp", layout.dial, dialWait)
	if err != nil {
		t.Fatalf("dialing %s before the ruleset failed: %v; nothing below would mean anything",
			layout.dial, err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close the control connection: %v", err)
	}

	if _, err := applyLandlock(abi, layout.shimArgs()); err != nil {
		t.Fatalf("applyLandlock: %v", err)
	}

	if conn, err := net.DialTimeout("tcp", layout.dial, dialWait); err == nil {
		_ = conn.Close()
		t.Errorf("the confined process connected to %s; on ABI %d the ruleset handles "+
			"CONNECT_TCP and grants it to nothing, so this connection must not exist", layout.dial, abi)
	} else if !errors.Is(err, os.ErrPermission) {
		// A refusal for some other reason is not evidence of confinement. It is how a network
		// test quietly stops testing the network — the address went away, the port was reused —
		// and the control dial above cannot rule that out for the second attempt.
		t.Errorf("dialing %s after the ruleset returned %v, want a permission error", layout.dial, err)
	}

	// The other half of the network mask, asked at the parent's own address rather than at port 0.
	// Landlock permits binding to port 0 on purpose - the kernel picks the port, so nothing is
	// being claimed - and a first attempt at this assertion failed in CI for exactly that reason.
	//
	// Binding a port the parent already holds is refused either way, so the errno is the whole
	// assertion: unconfined it is EADDRINUSE, and only a ruleset that handles BIND_TCP turns it
	// into a permission error.
	if listener, err := net.Listen("tcp", layout.dial); err == nil {
		_ = listener.Close()
		t.Errorf("the confined process bound %s, which the parent is listening on", layout.dial)
	} else if !errors.Is(err, os.ErrPermission) {
		t.Errorf("binding %s returned %v, want a permission error; anything else means the bind "+
			"was refused by the address already being in use and BIND_TCP was never consulted",
			layout.dial, err)
	}
}

// dialWait is long enough that a slow runner is not mistaken for a boundary, and short enough that
// a test which is genuinely blocked does not hold the suite.
const dialWait = 5 * time.Second

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
	if _, err := applyLandlock(abi, layout.shimArgs()); err != nil {
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
	if _, err := applyLandlock(abi, layout.shimArgs()); err != nil {
		t.Fatalf("applyLandlock: %v", err)
	}

	wider := layout.shimArgs()
	wider.AllowRW = append(wider.AllowRW, layout.outside)
	if _, err := applyLandlock(abi, wider); err != nil {
		t.Fatalf("applying a second, wider ruleset failed outright: %v; the interesting answer is "+
			"that it succeeds and changes nothing", err)
	}

	if _, err := os.ReadFile(filepath.Join(layout.outside, "id_ed25519")); !errors.Is(err, os.ErrPermission) {
		t.Errorf("after granting itself the directory, reading it returned %v, want a permission "+
			"error; a domain that can be widened from inside is not a boundary, and the shim's "+
			"whole design assumes it cannot be", err)
	}
}
