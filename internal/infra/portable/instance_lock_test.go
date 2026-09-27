package portable

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// holderEnv turns the test binary into a process that holds a data root's lock until its stdin
// closes. A second process is the only honest way to test the lock: an in-process test can pass
// for reasons that have nothing to do with the kernel arbitrating between two programs.
const holderEnv = "XQS_INSTANCE_LOCK_HOLDER"

func TestMain(m *testing.M) {
	if dataRoot := os.Getenv(holderEnv); dataRoot != "" {
		os.Exit(runLockHolder(dataRoot))
	}
	os.Exit(m.Run())
}

func runLockHolder(dataRoot string) int {
	lock, err := AcquireInstanceLock(dataRoot)
	if err != nil {
		fmt.Println("error:", err)
		return 2
	}
	fmt.Println("locked")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	if err := lock.Release(); err != nil {
		return 3
	}
	return 0
}

type lockHolder struct {
	cmd   *exec.Cmd
	stdin interface{ Close() error }
}

func startLockHolder(t *testing.T, dataRoot string) *lockHolder {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), holderEnv+"="+dataRoot)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start holder: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatalf("holder did not take the lock: %q, %v", line, err)
	}
	return &lockHolder{cmd: cmd, stdin: stdin}
}

func mustAcquire(t *testing.T, dataRoot string) *InstanceLock {
	t.Helper()
	lock, err := AcquireInstanceLock(dataRoot)
	if err != nil {
		t.Fatalf("acquire on a free data root: %v", err)
	}
	t.Cleanup(func() { _ = lock.Release() })
	return lock
}

func TestSecondClaimInTheSameProcessIsRefused(t *testing.T) {
	dataRoot := t.TempDir()
	mustAcquire(t, dataRoot)

	_, err := AcquireInstanceLock(dataRoot)
	if !errors.Is(err, ErrInstanceRunning) {
		t.Fatalf("second acquire = %v, want ErrInstanceRunning; two handles on one data root "+
			"would be two vault copies overwriting each other", err)
	}
}

func TestReleaseLetsTheDataRootBeClaimedAgain(t *testing.T) {
	dataRoot := t.TempDir()
	first, err := AcquireInstanceLock(dataRoot)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Errorf("second release = %v; shutdown paths may release twice and must not fail", err)
	}

	mustAcquire(t, dataRoot)
}

func TestAnotherProcessHoldingTheLockIsReportedAsRunning(t *testing.T) {
	dataRoot := t.TempDir()
	holder := startLockHolder(t, dataRoot)

	_, err := AcquireInstanceLock(dataRoot)
	if !errors.Is(err, ErrInstanceRunning) {
		t.Fatalf("acquire while another process holds the lock = %v, want ErrInstanceRunning", err)
	}

	pid, ok := InstanceLockOwner(dataRoot)
	if !ok || pid != holder.cmd.Process.Pid {
		t.Errorf("owner = %d, %v; want the holder's pid %d, the one the loser must find a window for",
			pid, ok, holder.cmd.Process.Pid)
	}
}

// The point of an OS lock over a PID file: a process that dies without cleaning up - a crash, a
// kill from Task Manager, a stick pulled out - must not leave the data root claimed forever.
func TestKilledHolderLeavesNoStaleClaim(t *testing.T) {
	dataRoot := t.TempDir()
	holder := startLockHolder(t, dataRoot)

	if err := holder.cmd.Process.Kill(); err != nil {
		t.Fatalf("kill holder: %v", err)
	}
	_ = holder.cmd.Wait()

	mustAcquire(t, dataRoot)
}

func TestHolderReleasingHandsTheDataRootOver(t *testing.T) {
	dataRoot := t.TempDir()
	holder := startLockHolder(t, dataRoot)

	if err := holder.stdin.Close(); err != nil {
		t.Fatalf("close holder stdin: %v", err)
	}
	if err := holder.cmd.Wait(); err != nil {
		t.Fatalf("holder exited with %v; its Release failed", err)
	}

	mustAcquire(t, dataRoot)
}

// A losing process must not overwrite the PID the winner recorded, or the window search that
// follows looks for the wrong process.
func TestLosingClaimLeavesTheOwnerRecordAlone(t *testing.T) {
	dataRoot := t.TempDir()
	holder := startLockHolder(t, dataRoot)

	for range 3 {
		if _, err := AcquireInstanceLock(dataRoot); !errors.Is(err, ErrInstanceRunning) {
			t.Fatalf("acquire = %v, want ErrInstanceRunning", err)
		}
	}

	if pid, _ := InstanceLockOwner(dataRoot); pid != holder.cmd.Process.Pid {
		t.Errorf("owner after losing claims = %d, want %d", pid, holder.cmd.Process.Pid)
	}
}

func TestOwnerIsThisProcessAfterAcquire(t *testing.T) {
	dataRoot := t.TempDir()
	if err := os.WriteFile(InstanceLockPath(dataRoot), []byte("999999999\nleftover"), 0o600); err != nil {
		t.Fatalf("seed stale lock file: %v", err)
	}
	mustAcquire(t, dataRoot)

	pid, ok := InstanceLockOwner(dataRoot)
	if !ok || pid != os.Getpid() {
		t.Errorf("owner = %d, %v; want %d with the previous run's contents replaced, not appended to",
			pid, ok, os.Getpid())
	}
}

func TestReleaseKeepsTheLockFile(t *testing.T) {
	dataRoot := t.TempDir()
	lock := mustAcquire(t, dataRoot)
	if err := lock.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := os.Stat(InstanceLockPath(dataRoot)); err != nil {
		t.Errorf("lock file gone after release (%v); unlinking it lets two processes lock two "+
			"different inodes under one name", err)
	}
}

func TestMissingDataRootIsCreated(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "fresh", "data")
	mustAcquire(t, dataRoot)
	if _, err := os.Stat(InstanceLockPath(dataRoot)); err != nil {
		t.Errorf("lock file not created under a new data root: %v", err)
	}
}

// A data root that cannot hold a lock file is a reason to start unprotected, not proof that
// another instance is running; the two must be distinguishable.
func TestUnusableDataRootIsNotMistakenForARunningInstance(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatalf("create blocker file: %v", err)
	}

	_, err := AcquireInstanceLock(blocker)
	if err == nil {
		t.Fatal("acquire under a regular file succeeded")
	}
	if errors.Is(err, ErrInstanceRunning) {
		t.Errorf("error %v claims a running instance where there is only an unusable path", err)
	}
}

func TestOwnerIsUnknownWithoutAUsableRecord(t *testing.T) {
	cases := map[string]string{"empty": "", "garbage": "abc", "zero": "0", "negative": "-12"}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			dataRoot := t.TempDir()
			if err := os.WriteFile(InstanceLockPath(dataRoot), []byte(content), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			if pid, ok := InstanceLockOwner(dataRoot); ok {
				t.Errorf("owner of %q = %d, true; want unknown", content, pid)
			}
		})
	}
	if _, ok := InstanceLockOwner(t.TempDir()); ok {
		t.Error("owner reported for a data root with no lock file")
	}
}
