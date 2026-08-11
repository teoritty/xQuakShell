//go:build windows

package sandbox

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// stdioPipes are the three anonymous pipes a plugin process talks through: the JSON-RPC transport
// on stdin and stdout, and the log stream on stderr.
//
// Each pipe is created with both ends inheritable and the parent's end then marked non-inheritable.
// That order is not decoration: CreatePipe cannot make one end inheritable on its own, and a
// parent-side handle that leaked into the child would keep the pipe open after the parent closed
// it, so a read that should end at the child's exit would block forever instead.
type stdioPipes struct {
	stdinRead, stdinWrite   windows.Handle
	stdoutRead, stdoutWrite windows.Handle
	stderrRead, stderrWrite windows.Handle
}

func newStdioPipes() (*stdioPipes, error) {
	p := &stdioPipes{}
	for _, pipe := range []struct {
		name             string
		read, write      *windows.Handle
		parentEndIsWrite bool
	}{
		{"stdin", &p.stdinRead, &p.stdinWrite, true},
		{"stdout", &p.stdoutRead, &p.stdoutWrite, false},
		{"stderr", &p.stderrRead, &p.stderrWrite, false},
	} {
		if err := createInheritablePipe(pipe.read, pipe.write, pipe.parentEndIsWrite); err != nil {
			p.closeParentEnds()
			p.closeChildEnds()
			return nil, fmt.Errorf("create plugin %s pipe: %w", pipe.name, err)
		}
	}
	return p, nil
}

func createInheritablePipe(read, write *windows.Handle, parentEndIsWrite bool) error {
	sa := windows.SecurityAttributes{InheritHandle: 1}
	sa.Length = uint32(unsafe.Sizeof(sa))
	if err := windows.CreatePipe(read, write, &sa, 0); err != nil {
		return err
	}
	parentEnd := *read
	if parentEndIsWrite {
		parentEnd = *write
	}
	if err := windows.SetHandleInformation(parentEnd, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return fmt.Errorf("clear inherit flag: %w", err)
	}
	return nil
}

// closeChildEnds releases the handles the child now owns. The parent must do this once
// CreateProcess has duplicated them, or the child's exit never closes the pipe and the reader on
// this side hangs.
func (p *stdioPipes) closeChildEnds() {
	for _, h := range []*windows.Handle{&p.stdinRead, &p.stdoutWrite, &p.stderrWrite} {
		closeHandle(h)
	}
}

// closeParentEnds is the failure path: nothing was started, so nothing owns these.
func (p *stdioPipes) closeParentEnds() {
	for _, h := range []*windows.Handle{&p.stdinWrite, &p.stdoutRead, &p.stderrRead} {
		closeHandle(h)
	}
}

func closeHandle(h *windows.Handle) {
	if *h != 0 && *h != windows.InvalidHandle {
		_ = windows.CloseHandle(*h)
		*h = 0
	}
}
