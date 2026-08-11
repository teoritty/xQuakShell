package plugin

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// childHelperEnv makes TestChildProcessHelper exit with a chosen status instead of skipping, so
// these tests get a real OS process with a known exit code and no fixture binary. Re-invoking the
// test binary is the standard os/exec pattern and behaves the same on every platform the host
// builds for.
const childHelperEnv = "XQS_CHILD_HELPER_EXIT"

// helperBlock asks the helper to block until it is killed instead of exiting.
const helperBlock = "block"

// TestChildProcessHelper is not a test. It is the body of the child process the tests below spawn,
// and it does nothing at all unless the environment variable selects it.
func TestChildProcessHelper(t *testing.T) {
	code := os.Getenv(childHelperEnv)
	switch {
	case code == "":
		t.Skip("helper process body; runs only when re-invoked with " + childHelperEnv)
	case code == helperBlock:
		// Blocks until killed. Reading stdin is what holds it: the parent owns the write end and
		// never writes, so this returns only when the process dies.
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	n, err := strconv.Atoi(code)
	if err != nil {
		n = 1
	}
	os.Exit(n)
}

// startHelperChild starts this test binary as a child that exits with the given status.
func startHelperChild(t *testing.T, exitCode int) *exec.Cmd {
	t.Helper()
	return startHelperChildMode(t, strconv.Itoa(exitCode))
}

func startHelperChildMode(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestChildProcessHelper$")
	cmd.Env = append(os.Environ(), childHelperEnv+"="+mode)
	if mode == helperBlock {
		// The child blocks on stdin, so it needs one that stays open. The parent keeps the write
		// end for the life of the test and never writes to it.
		if _, err := cmd.StdinPipe(); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper child: %v", err)
	}
	return cmd
}

// TestExecChildReportsTheRealPidAndExitStatus covers the adapter directly because the end-to-end
// suites cover it badly. Breaking Pid() does not make them fail: the pid reaches
// preparePluginSandbox, which refuses a non-positive one, and the plugin start then fails in a way
// that leaves the higher-level tests waiting rather than reporting — a ten-minute hang instead of a
// failure with a name on it. This turns the same mutation into a one-second red.
func TestExecChildReportsTheRealPidAndExitStatus(t *testing.T) {
	child := newExecChild(startHelperChild(t, 0))

	want := child.cmd.Process.Pid
	if want <= 0 {
		t.Fatalf("the OS reported pid %d for a process it just started", want)
	}
	if got := child.Pid(); got != want {
		t.Errorf("Pid() = %d, want %d; every OS-level bound on a plugin is keyed by this number", got, want)
	}

	if err := child.Wait(); err != nil {
		t.Errorf("Wait() = %v, want nil for a child that exits 0; the reaper reports this as a crash", err)
	}
}

// TestExecChildWaitReportsANonZeroExit is the half that makes the test above load-bearing. A Wait
// that swallowed the status and returned nil would pass every assertion on a child that exits 0,
// and the host would then read a crashed plugin as a clean shutdown and never restart it.
func TestExecChildWaitReportsANonZeroExit(t *testing.T) {
	child := newExecChild(startHelperChild(t, 3))

	err := child.Wait()
	if err == nil {
		t.Fatal("Wait() = nil for a child that exited 3; a crash would be reported as a clean exit")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Wait() = %v, want an *exec.ExitError carrying the status", err)
	}
	if got := exitErr.ExitCode(); got != 3 {
		t.Errorf("exit code = %d, want 3", got)
	}
}

// TestExecChildKillEndsTheProcess uses a child that will not exit on its own, so the assertion is
// about Kill and nothing else. With a child that exits immediately the test passes whether or not
// Kill does anything at all, which is how the first version of it was worthless.
func TestExecChildKillEndsTheProcess(t *testing.T) {
	child := newExecChild(startHelperChildMode(t, helperBlock))

	if err := child.Kill(); err != nil {
		t.Fatalf("Kill() = %v, want nil", err)
	}

	// The deadline is the assertion: a Kill that never reached the process leaves Wait blocked on a
	// child that blocks forever, and without a bound here that is a hung suite instead of a failure
	// with a name on it.
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Wait did not return within 10s of Kill; the kill did not reach the process")
	}
}
