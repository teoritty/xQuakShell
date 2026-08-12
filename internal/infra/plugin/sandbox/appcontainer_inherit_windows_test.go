//go:build windows

package sandbox_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"xquakshell/internal/infra/plugin/sandbox"
)

// eofWait bounds how long the parent waits for its own pipe to report end of file.
//
// The passing answer arrives immediately — closing the only write handle ends the pipe — so this
// number is not a guess about how slow a correct run can be. It is how long a FAILING run takes to
// prove itself: a child holding an inherited write handle never lets that read return, and the test
// has to call that a failure rather than hang the suite.
const eofWait = 10 * time.Second

// TestASpawnedContainerInheritsNoHandleBeyondItsOwnPipes fails if the child receives any inheritable
// handle other than the three it is given.
//
// The extra pipe here stands in for a concurrent spawn's child ends, which is the real source of
// such a handle: plugin starts are not serialised, and every one of them leaves two inheritable
// handles open for the length of its own CreateProcess call. The test asks the question the way the
// operating system answers it — a pipe reports end of file when the last write handle closes, so a
// read that never ends means somebody else is holding one.
func TestASpawnedContainerInheritsNoHandleBeyondItsOwnPipes(t *testing.T) {
	container := newTestContainer(t)

	read, write := newInheritablePipe(t)
	proc := spawnLingeringProcess(t, container)

	// The parent's own write handle has to go first, or the pipe would stay open on its account and
	// the read below could not distinguish that from the child holding one.
	if err := windows.CloseHandle(write); err != nil {
		t.Fatalf("close the parent's write end: %v", err)
	}

	readFile := os.NewFile(uintptr(read), "inheritance-probe")
	t.Cleanup(func() { _ = readFile.Close() })

	result := make(chan error, 1)
	go func() {
		_, err := readFile.Read(make([]byte, 1))
		result <- err
	}()

	select {
	case err := <-result:
		if !errors.Is(err, io.EOF) {
			t.Errorf("read from the probe pipe = %v, want EOF", err)
		}
	case <-time.After(eofWait):
		t.Errorf("the probe pipe never reported EOF while the plugin process (pid %d) was alive; "+
			"the child inherited a write handle it was never given, and a handle that arrives by "+
			"inheritance is checked by nobody — not the container SID, not an ACL", proc.Pid())
	}
}

// newInheritablePipe creates a pipe with BOTH ends inheritable, exactly as newStdioPipes does before
// it clears the flag on the end the host keeps. Neither end is cleared here: this pipe is standing in
// for another spawn's handles, and the whole question is whether they travel.
func newInheritablePipe(t *testing.T) (read, write windows.Handle) {
	t.Helper()
	sa := windows.SecurityAttributes{InheritHandle: 1}
	sa.Length = uint32(unsafe.Sizeof(sa))
	if err := windows.CreatePipe(&read, &write, &sa, 0); err != nil {
		t.Fatalf("CreatePipe: %v", err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(read) })
	return read, write
}

// spawnLingeringProcess starts a process inside the container that stays alive until the test ends.
//
// cmd.exe with no arguments waits on the standard input the spawn just handed it, which is the
// simplest process that outlives its own start without a fixture binary to build. System32 is
// readable and executable by every AppContainer, so no grant is needed to reach it — and that is
// worth knowing on its own: "confined to its own directories" has always meant its own USER data,
// never the parts of Windows that ship open to all application packages.
func spawnLingeringProcess(t *testing.T, container *sandbox.Container) *sandbox.Process {
	t.Helper()
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatalf("locate the system directory: %v", err)
	}
	dataDir := t.TempDir()
	proc, err := sandbox.Spawn(container, sandbox.SpawnRequest{
		Exe: filepath.Join(system, "cmd.exe"),
		Env: sandbox.ContainerEnv([]string{"SYSTEMROOT=" + filepath.Dir(system)}, dataDir),
	})
	if err != nil {
		t.Fatalf("spawn a process in the container: %v", err)
	}
	t.Cleanup(func() {
		_ = proc.Stdin.Close()
		_ = proc.Kill()
		_ = proc.Stdout.Close()
	})
	return proc
}
