package usecase

import (
	"sync"
	"time"
)

// emitInterval throttles progress emissions so a huge tree does not flood the
// event bus. An emission also fires every emitEvery items regardless, so short
// bursts still advance the bar.
const (
	emitInterval = 100 * time.Millisecond
	emitEvery    = 64
)

// operationReporter turns raw operation facts into TransferProgress events. It
// owns the emission policy — rate limiting and terminal-state latching — so the
// planner, the executor and RemoteOpService only report facts.
//
// Lifecycle contract: an operation that emitted any non-terminal event MUST emit
// exactly one terminal event. A Finish() in a defer guarantees the "must"; the
// done latch guarantees the "exactly one".
type operationReporter struct {
	opID      string
	sessionID string
	kind      string
	// direction mirrors kind by default. It is a transfer-only notion
	// (upload/download); operations that have none clear it via withDirection.
	direction string
	// label fills TransferProgress.RemotePath — a human-readable caption that
	// must never be parsed as a path. It defaults to the target directory;
	// batch callers replace it via withLabel.
	label string
	// refreshDir fills TransferProgress.RefreshDir — the directory the UI
	// reloads when the operation finishes. Empty leaves the choice to the UI.
	refreshDir string
	emit       TransferProgressFunc

	mu        sync.Mutex
	throttle  *throttler
	scanned   int64
	lastDone  int64
	lastTotal int64
	done      bool
}

// newOperationReporter builds a reporter for one operation. targetDir seeds both
// the display label and the refresh directory; pass "" when the operation
// declares neither.
func newOperationReporter(opID, sessionID, kind, targetDir string, emit TransferProgressFunc) *operationReporter {
	return &operationReporter{
		opID:       opID,
		sessionID:  sessionID,
		kind:       kind,
		direction:  kind,
		label:      targetDir,
		refreshDir: targetDir,
		emit:       emit,
		throttle:   newThrottler(),
	}
}

// withLabel overrides the human-readable caption (TransferProgress.RemotePath)
// without touching the refresh directory. Used by batch operations, whose
// caption is a count ("3 items") rather than a path.
func (r *operationReporter) withLabel(label string) *operationReporter {
	r.label = label
	return r
}

// withDirection overrides TransferProgress.Direction. Used by operations that
// move no bytes and therefore have no direction.
func (r *operationReporter) withDirection(direction string) *operationReporter {
	r.direction = direction
	return r
}

// terminalStates close an operation. The first one wins.
func isTerminalState(s string) bool {
	return s == "completed" || s == "failed" || s == "cancelled"
}

// Report emits one progress event. A terminal state closes the reporter, after
// which every further call — including a deferred Finish — is a no-op.
func (r *operationReporter) Report(done, total int64, state string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reportLocked(done, total, state)
}

// Started emits the initial indeterminate "active" event.
func (r *operationReporter) Started() { r.Report(0, 0, "active") }

// Scanned ticks the indeterminate scan counter during enumeration.
func (r *operationReporter) Scanned() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scanned++
	r.reportLocked(r.scanned, 0, "active")
}

// Finish emits a terminal event carrying the last reported figures, unless the
// reporter is already closed. Safe — and intended — to call from a defer on
// every exit path.
func (r *operationReporter) Finish(state string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reportLocked(r.lastDone, r.lastTotal, state)
}

// reportLocked is the emission policy itself. The caller holds r.mu.
func (r *operationReporter) reportLocked(done, total int64, state string) {
	if r.done {
		return
	}
	// Recorded before the throttle check, so Finish carries the last *reported*
	// figures rather than the last *emitted* ones: on a determinate transfer the
	// panel renders a ratio, and a safety-net Finish must not rewind the bar to
	// whatever the throttle happened to let through.
	r.lastDone, r.lastTotal = done, total
	if isTerminalState(state) {
		r.done = true
	} else if !r.throttle.ready(done) {
		return // throttling applies to progress only, never to a terminal event
	}
	r.emitLocked(done, total, state)
}

// emitLocked hands one fully-formed event to the progress callback. The caller
// holds r.mu, which keeps the emitted order identical to the reported order.
func (r *operationReporter) emitLocked(done, total int64, state string) {
	if r.emit == nil {
		return
	}
	r.emit(TransferProgress{
		ID: r.opID, SessionID: r.sessionID,
		Kind: r.kind, Direction: r.direction,
		RemotePath: r.label, RefreshDir: r.refreshDir,
		Done: done, Total: total, State: state,
	})
}

// throttler rate-limits progress emissions by both time and item count.
type throttler struct {
	last time.Time
}

func newThrottler() *throttler {
	return &throttler{}
}

// ready reports whether a progress emission should fire for the given item
// counter, at most every emitInterval or every emitEvery items.
func (t *throttler) ready(count int64) bool {
	now := time.Now()
	if count%emitEvery == 0 || now.Sub(t.last) >= emitInterval {
		t.last = now
		return true
	}
	return false
}
