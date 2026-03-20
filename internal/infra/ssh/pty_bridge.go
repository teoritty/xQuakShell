package ssh

import (
	"context"
	"fmt"
	"io"
	"sync"

	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
)

const (
	defaultTerm = "xterm-256color"
	readBufSize = 8 * 1024
)

// PTYBridge implements domain.TerminalPTYBridge over an SSH session.
type PTYBridge struct {
	mu      sync.Mutex
	session *gossh.Session
	stdin   io.WriteCloser
	closed  bool
}

// NewPTYBridge creates a new PTYBridge (not yet started).
func NewPTYBridge() *PTYBridge {
	return &PTYBridge{}
}

// Start opens a PTY on the remote via the SSH client and starts reading stdout.
// Returns a channel of output byte slices that the caller must drain.
// The channel is closed when the session ends or the context is cancelled.
func (b *PTYBridge) Start(ctx context.Context, sshClient domain.SSHClient, opts domain.PTYOptions) (<-chan []byte, error) {
	session, err := sshClient.NewSession()
	if err != nil {
		return nil, fmt.Errorf("pty new session: %w", err)
	}

	term := opts.Term
	if term == "" {
		term = defaultTerm
	}
	cols := opts.Cols
	if cols == 0 {
		cols = 80
	}
	rows := opts.Rows
	if rows == 0 {
		rows = 24
	}

	modes := gossh.TerminalModes{
		gossh.ECHO:          1,
		gossh.TTY_OP_ISPEED: 14400,
		gossh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty(term, int(rows), int(cols), modes); err != nil {
		session.Close()
		return nil, fmt.Errorf("pty request: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("pty stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("pty stdout pipe: %w", err)
	}

	if err := session.Shell(); err != nil {
		session.Close()
		return nil, fmt.Errorf("pty shell: %w", err)
	}

	b.mu.Lock()
	b.session = session
	b.stdin = stdin
	b.closed = false
	b.mu.Unlock()

	outputCh := make(chan []byte, 64)

	go b.readLoop(ctx, stdout, outputCh)

	go func() {
		session.Wait()
		b.mu.Lock()
		b.closed = true
		b.mu.Unlock()
	}()

	return outputCh, nil
}

// Write sends input bytes to the remote PTY stdin.
func (b *PTYBridge) Write(data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed || b.stdin == nil {
		return fmt.Errorf("pty write: session closed")
	}

	_, err := b.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("pty write: %w", err)
	}
	return nil
}

// Resize changes the PTY window size.
func (b *PTYBridge) Resize(cols, rows uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed || b.session == nil {
		return fmt.Errorf("pty resize: session closed")
	}

	if err := b.session.WindowChange(int(rows), int(cols)); err != nil {
		return fmt.Errorf("pty resize: %w", err)
	}
	return nil
}

// Close terminates the PTY session and releases resources.
func (b *PTYBridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}
	b.closed = true

	if b.stdin != nil {
		b.stdin.Close()
	}
	if b.session != nil {
		b.session.Close()
	}
	return nil
}

// readLoop reads from stdout and sends chunks to the output channel.
// Closes the channel when done.
func (b *PTYBridge) readLoop(ctx context.Context, r io.Reader, ch chan<- []byte) {
	defer close(ch)

	buf := make([]byte, readBufSize)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := r.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			select {
			case ch <- data:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			return
		}
	}
}
