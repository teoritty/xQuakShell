package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"ssh-client/internal/domain"
)

const (
	terminalBatchMaxBytes = 512
	terminalBatchMaxDelay = 8 * time.Millisecond
)

// pluginTerminalBridge forwards keyboard input to a plugin session.
type pluginTerminalBridge struct {
	notify func(ctx context.Context, method string, params json.RawMessage) error

	mu     sync.Mutex
	buf    []byte
	timer  *time.Timer
	closed bool
}

func (b *pluginTerminalBridge) Start(_ context.Context, _ domain.SSHClient, _ domain.PTYOptions) (<-chan []byte, error) {
	return nil, fmt.Errorf("plugin terminal bridge does not use Start")
}

func (b *pluginTerminalBridge) Write(p []byte) error {
	if b == nil || len(p) == 0 {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}

	b.buf = append(b.buf, p...)
	if len(b.buf) >= terminalBatchMaxBytes {
		return b.flushLocked()
	}
	if b.timer == nil {
		b.timer = time.AfterFunc(terminalBatchMaxDelay, func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			_ = b.flushLocked()
		})
	}
	return nil
}

func (b *pluginTerminalBridge) flushLocked() error {
	if len(b.buf) == 0 {
		return nil
	}
	payload := append([]byte(nil), b.buf...)
	b.buf = b.buf[:0]
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}

	params, err := json.Marshal(map[string]string{
		"dataBase64": base64.StdEncoding.EncodeToString(payload),
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return b.notify(ctx, "session.writeInput", params)
}

func (b *pluginTerminalBridge) Resize(cols, rows uint32) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	_ = b.flushLocked()
	b.mu.Unlock()

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
	defer b.mu.Unlock()
	b.closed = true
	return b.flushLocked()
}

var _ domain.TerminalPTYBridge = (*pluginTerminalBridge)(nil)
