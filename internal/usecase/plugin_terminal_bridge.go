package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"xquakshell/internal/domain"
)

const (
	terminalBatchMaxBytes = 512
	terminalBatchMaxDelay = 8 * time.Millisecond
)

// pluginTerminalBridge forwards keyboard input to a plugin session.
//
// mu and sendMu are never held simultaneously. mu guards only the in-memory
// batching state (buf/timer/closed) and is held briefly for pure in-memory
// work; sendMu serializes the outbound host->plugin RPCs so terminal bytes
// keep their write order even though the RPC runs outside mu. Every call
// site takes a batch under mu, unlocks, and only then sends it — see
// takeBatchLocked and sendBatch.
type pluginTerminalBridge struct {
	notify func(ctx context.Context, method string, params json.RawMessage) error

	mu     sync.Mutex // guards buf/timer/closed only; never held during I/O
	buf    []byte
	timer  *time.Timer
	closed bool

	sendMu sync.Mutex // serializes outbound RPCs so batches arrive in write order
}

func (b *pluginTerminalBridge) Start(_ context.Context, _ domain.SSHClient, _ domain.PTYOptions) (<-chan []byte, error) {
	return nil, fmt.Errorf("plugin terminal bridge does not use Start")
}

func (b *pluginTerminalBridge) Write(p []byte) error {
	if b == nil || len(p) == 0 {
		return nil
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}

	b.buf = append(b.buf, p...)
	var payload []byte
	if len(b.buf) >= terminalBatchMaxBytes {
		payload = b.takeBatchLocked()
	} else if b.timer == nil {
		b.timer = time.AfterFunc(terminalBatchMaxDelay, func() {
			b.mu.Lock()
			batch := b.takeBatchLocked()
			b.mu.Unlock()
			_ = b.sendBatch(batch)
		})
	}
	b.mu.Unlock()

	return b.sendBatch(payload)
}

// takeBatchLocked detaches the pending batch and disarms the timer. It
// performs no I/O: callers send the payload after releasing b.mu, so a slow
// or stuck plugin can never block the next keystroke. Caller must hold b.mu.
func (b *pluginTerminalBridge) takeBatchLocked() []byte {
	if len(b.buf) == 0 {
		return nil
	}
	payload := append([]byte(nil), b.buf...)
	b.buf = b.buf[:0]
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	return payload
}

// sendBatch performs the host->plugin RPC for a previously-taken batch. It
// must be called WITHOUT b.mu held. sendMu serializes concurrent sends so
// that batches reach the plugin in the order they were taken.
func (b *pluginTerminalBridge) sendBatch(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	params, err := json.Marshal(map[string]string{
		"dataBase64": base64.StdEncoding.EncodeToString(payload),
	})
	if err != nil {
		return err
	}

	b.sendMu.Lock()
	defer b.sendMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return b.notify(ctx, "session.writeInput", params)
}

func (b *pluginTerminalBridge) Resize(cols, rows uint32) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	payload := b.takeBatchLocked()
	b.mu.Unlock()
	_ = b.sendBatch(payload)

	params, err := json.Marshal(map[string]uint32{"cols": cols, "rows": rows})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return b.notify(ctx, "session.resize", params)
}

func (b *pluginTerminalBridge) Close() error {
	b.mu.Lock()
	b.closed = true
	payload := b.takeBatchLocked()
	b.mu.Unlock()

	return b.sendBatch(payload)
}

var _ domain.TerminalPTYBridge = (*pluginTerminalBridge)(nil)
