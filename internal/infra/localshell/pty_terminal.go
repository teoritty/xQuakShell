package localshell

import (
	"fmt"
	"sync"

	"github.com/aymanbagabas/go-pty"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/safego"
)

// readBufSize matches the SSH terminal's read buffer so both producers hand the UI chunks of the
// same rough size and the batching downstream behaves the same for either.
const readBufSize = 8 * 1024

// outputQueueDepth bounds how far the reader may run ahead of the consumer. It exists so a shell
// printing faster than the UI repaints blocks in the read loop rather than growing a queue until
// the process runs out of memory.
const outputQueueDepth = 64

var _ domain.LocalTerminalPTY = (*terminal)(nil)

// terminal is one running shell and the goroutines pumping it.
type terminal struct {
	pty pty.Pty
	cmd *pty.Cmd
	out chan []byte

	mu     sync.Mutex
	closed bool

	closeOut sync.Once
	// job carries the platform handle that owns the process tree, where the platform has one.
	job processGroup
}

func newTerminal(handle pty.Pty, cmd *pty.Cmd) *terminal {
	return &terminal{
		pty: handle,
		cmd: cmd,
		out: make(chan []byte, outputQueueDepth),
	}
}

// pump starts the two goroutines a running shell needs: one moving its output, one noticing that
// it exited on its own.
func (t *terminal) pump() {
	safego.GoNamed("localshell.read", t.readLoop)
	safego.GoNamed("localshell.wait", t.waitLoop)
}

// Output returns the channel carrying the shell's bytes. It closes once, when the shell is gone.
func (t *terminal) Output() <-chan []byte {
	return t.out
}

// Write sends keystrokes to the shell.
func (t *terminal) Write(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return fmt.Errorf("write to local terminal: %w", domain.ErrLocalTerminalNotFound)
	}
	if _, err := t.pty.Write(data); err != nil {
		return fmt.Errorf("write to local terminal: %w", err)
	}
	return nil
}

// Resize changes the shell's window size.
func (t *terminal) Resize(cols, rows uint16) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return fmt.Errorf("resize local terminal: %w", domain.ErrLocalTerminalNotFound)
	}
	if err := t.pty.Resize(int(cols), int(rows)); err != nil {
		return fmt.Errorf("resize local terminal: %w", err)
	}
	return nil
}

// Close kills the shell and everything it started, then releases the pseudo-terminal.
//
// Safe to call twice: the tab closing and the shell exiting on its own both arrive here, and
// which one wins is a race the caller should not have to think about.
func (t *terminal) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()

	// Order matters. Killing the tree first means the read loop sees the pseudo-terminal end
	// naturally; closing the pty first would leave orphans writing into a closed handle.
	t.killTree()
	if err := t.pty.Close(); err != nil {
		return fmt.Errorf("close local terminal: %w", err)
	}
	return nil
}

// readLoop moves bytes from the shell to the output channel until the shell is gone.
func (t *terminal) readLoop() {
	defer t.closeOutput()

	buf := make([]byte, readBufSize)
	for {
		n, err := t.pty.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			t.out <- chunk
		}
		if err != nil {
			return
		}
	}
}

// waitLoop reaps the shell and tears the rest down when it exits by itself - the user typing
// `exit`, or the process dying. Without it a shell that ended would leave its tab looking alive.
func (t *terminal) waitLoop() {
	_ = t.cmd.Wait()
	_ = t.Close()
}

func (t *terminal) closeOutput() {
	t.closeOut.Do(func() { close(t.out) })
}
