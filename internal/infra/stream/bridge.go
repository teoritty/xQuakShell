package stream

import (
	"context"
	"fmt"
	"io"
	"sync"

	"ssh-client/internal/domain"
)

const readBufSize = 8 * 1024

// Bridge implements domain.TerminalPTYBridge for raw stream connections (Telnet, Serial).
// Create with NewBridge(conn), then Start(ctx, nil, opts) uses the pre-set connection.
type Bridge struct {
	mu     sync.Mutex
	conn   io.ReadWriteCloser
	stdin  io.Writer
	closed bool
}

// NewBridge creates a new stream bridge. Call SetConnection before Start.
func NewBridge() *Bridge {
	return &Bridge{}
}

// SetConnection sets the underlying connection. Must be called before Start.
func (b *Bridge) SetConnection(conn io.ReadWriteCloser) {
	b.mu.Lock()
	b.conn = conn
	b.stdin = conn
	b.mu.Unlock()
}

// Start implements domain.TerminalPTYBridge. For stream bridges, sshClient is ignored.
func (b *Bridge) Start(ctx context.Context, _ domain.SSHClient, _ domain.PTYOptions) (<-chan []byte, error) {
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	if conn == nil {
		return nil, fmt.Errorf("stream bridge: no connection set")
	}

	outputCh := make(chan []byte, 64)
	go b.readLoop(ctx, conn, outputCh)
	return outputCh, nil
}

// StartFromConn starts reading from the connection and returns the output channel.
// Use this when the connector has the connection and wants to avoid the SSHClient parameter.
func (b *Bridge) StartFromConn(ctx context.Context, conn io.ReadWriteCloser) (<-chan []byte, error) {
	b.SetConnection(conn)
	return b.Start(ctx, nil, domain.PTYOptions{})
}

// Write implements domain.TerminalPTYBridge.
func (b *Bridge) Write(data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.stdin == nil {
		return fmt.Errorf("stream write: closed")
	}
	_, err := b.stdin.Write(data)
	return err
}

// Resize implements domain.TerminalPTYBridge. No-op for stream (no PTY window).
func (b *Bridge) Resize(_, _ uint32) error {
	return nil
}

// Close implements domain.TerminalPTYBridge.
func (b *Bridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
		b.stdin = nil
	}
	return nil
}

func (b *Bridge) readLoop(ctx context.Context, r io.Reader, ch chan<- []byte) {
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
