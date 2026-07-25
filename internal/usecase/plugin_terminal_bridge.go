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
// Two properties must hold at once:
//
//   - Latency: Write must never block on the network. The host->plugin RPC
//     therefore runs outside mu, which guards only the in-memory batching
//     state (buf/timer/closed).
//   - Ordering: terminal bytes must reach the plugin in write order.
//     Serializing the sends alone is not enough — two goroutines can take
//     batches under mu in one order and then race to acquire the send slot in
//     the other order. Instead a batch is detached from buf *only* by the
//     goroutine that has already claimed sendMu via TryLock, so at most one
//     batch is ever in flight and take-order equals send-order by
//     construction. A goroutine that cannot claim the slot leaves its bytes in
//     buf and (re-)arms the timer, which retries the flush later.
//
// Lock discipline: mu is never held while *blocking* on sendMu. Under mu the
// send slot is only ever claimed with TryLock, which cannot block, so no
// lock-order cycle exists. The two call sites that must not drop their batch
// (Resize, Close) acquire sendMu first without holding mu, then take mu
// briefly — a strict sendMu -> mu order that TryLock can never deadlock with.
type pluginTerminalBridge struct {
	notify func(ctx context.Context, method string, params json.RawMessage) error

	mu     sync.Mutex // guards buf/timer/closed only; never held during I/O
	buf    []byte
	timer  *time.Timer
	closed bool

	sendMu sync.Mutex // the send slot: held for the whole take+send of one batch
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
		// Claims sendMu on success; re-arms the timer on failure so the
		// bytes are never stranded in buf.
		payload = b.tryTakeBatchLocked()
	} else {
		b.armTimerLocked()
	}
	b.mu.Unlock()

	if payload == nil {
		return nil
	}
	defer b.sendMu.Unlock()
	return b.sendBatchLocked(payload)
}

// onBatchTimer is the batching timer's callback. It retries the flush: if the
// send slot is busy the bytes stay buffered and tryTakeBatchLocked arms a
// fresh timer, so a parked send delays delivery but never loses it.
func (b *pluginTerminalBridge) onBatchTimer() {
	b.mu.Lock()
	b.timer = nil // this firing is spent; tryTakeBatchLocked may arm a new one
	payload := b.tryTakeBatchLocked()
	b.mu.Unlock()

	if payload == nil {
		return
	}
	defer b.sendMu.Unlock()
	_ = b.sendBatchLocked(payload)
}

// armTimerLocked ensures a flush is scheduled for the currently buffered
// bytes. Caller must hold b.mu.
func (b *pluginTerminalBridge) armTimerLocked() {
	if b.closed || b.timer != nil {
		return
	}
	b.timer = time.AfterFunc(terminalBatchMaxDelay, b.onBatchTimer)
}

// tryTakeBatchLocked detaches the pending batch for immediate sending, but
// only if it can also claim the send slot. A non-nil return means the caller
// now holds sendMu and must send the payload and then unlock sendMu. A nil
// return means nothing is to be sent: either the buffer was empty, or a send
// is already in flight and the bytes were left buffered with the timer armed
// to retry. It performs no I/O. Caller must hold b.mu.
func (b *pluginTerminalBridge) tryTakeBatchLocked() []byte {
	if len(b.buf) == 0 {
		return nil
	}
	if !b.sendMu.TryLock() {
		b.armTimerLocked()
		return nil
	}
	return b.takeBufferLocked()
}

// takeBufferLocked detaches the pending batch and disarms the timer without
// touching the send slot. Caller must hold b.mu.
func (b *pluginTerminalBridge) takeBufferLocked() []byte {
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

// sendBatch performs the host->plugin RPC for a batch, acquiring and
// releasing the send slot itself. Must be called WITHOUT b.mu held.
func (b *pluginTerminalBridge) sendBatch(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	b.sendMu.Lock()
	defer b.sendMu.Unlock()
	return b.sendBatchLocked(payload)
}

// sendBatchLocked performs the host->plugin RPC for a batch. Caller must hold
// sendMu and must NOT hold b.mu.
func (b *pluginTerminalBridge) sendBatchLocked(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	params, err := json.Marshal(map[string]string{
		"dataBase64": base64.StdEncoding.EncodeToString(payload),
	})
	if err != nil {
		return err
	}
	return b.notifyWithTimeout("session.writeInput", params)
}

func (b *pluginTerminalBridge) notifyWithTimeout(method string, params json.RawMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return b.notify(ctx, method, params)
}

func (b *pluginTerminalBridge) Resize(cols, rows uint32) error {
	if b == nil {
		return nil
	}
	params, err := json.Marshal(map[string]uint32{"cols": cols, "rows": rows})
	if err != nil {
		return err
	}

	// Resize is not on the keystroke hot path, so it waits for the send slot
	// rather than declining it: that both guarantees the pending input is not
	// dropped and keeps session.resize behind the keystrokes typed before it.
	b.sendMu.Lock()
	defer b.sendMu.Unlock()

	b.mu.Lock()
	payload := b.takeBufferLocked()
	b.mu.Unlock()
	_ = b.sendBatchLocked(payload)

	return b.notifyWithTimeout("session.resize", params)
}

func (b *pluginTerminalBridge) Close() error {
	b.mu.Lock()
	b.closed = true
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.mu.Unlock()

	// Wait for any in-flight send so the final batch is neither lost nor
	// delivered ahead of earlier bytes.
	b.sendMu.Lock()
	defer b.sendMu.Unlock()

	b.mu.Lock()
	payload := b.takeBufferLocked()
	b.mu.Unlock()

	return b.sendBatchLocked(payload)
}

var _ domain.TerminalPTYBridge = (*pluginTerminalBridge)(nil)
