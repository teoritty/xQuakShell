package usecase

import (
	"sync"
	"time"

	"xquakshell/internal/pkg/safego"
)

// Limits on one surface's output queue (ADR-015 §1).
const (
	// surfaceOutputBudgetBytes is how much unsent output one surface may hold. Past it a write
	// waits, and then fails, rather than growing the host's memory on the plugin's schedule. One
	// MiB is four full frames: enough to ride out a repaint, far short of a leak.
	surfaceOutputBudgetBytes = 1 << 20

	// surfaceOutputQueueDepth bounds the number of queued chunks independently of their size, so a
	// plugin writing a byte at a time cannot make the pump walk a million-element queue.
	surfaceOutputQueueDepth = 256

	// surfaceWriteTimeout is how long a write waits for room. The same allowance
	// session.writeTerminal gives, for the same reason: a consumer that is briefly behind is
	// normal, and one that is behind for two seconds is not going to catch up by being sent more.
	surfaceWriteTimeout = 2 * time.Second

	// surfaceOutputBatchInterval is how often the pump flushes. It matches the session terminal
	// pump: one event per stream per interval instead of one per write, which is what keeps a
	// chatty producer from turning into a repaint per chunk in the UI.
	surfaceOutputBatchInterval = 50 * time.Millisecond
)

// surfaceChunk is one piece of a surface's output, tagged with the stream it came from.
type surfaceChunk struct {
	data   []byte
	stream string
}

// surfaceOutputQueue is one surface's bounded queue and the accounting behind its budget.
type surfaceOutputQueue struct {
	chunks chan surfaceChunk

	mu     sync.Mutex
	queued int
	closed bool
	// room is signalled by the pump after it drains, so a blocked writer wakes as soon as there is
	// space rather than after a poll interval.
	room chan struct{}
}

// SurfaceOutputBroker owns every open surface's output queue.
//
// It follows SurfaceRegistry's discipline for the same reason: it holds a mutex, and it makes no
// outbound call of any kind — no presenter, no plugin, no audit. The pump goroutine reads from a
// queue it was handed and calls out from there, never while this lock is held.
type SurfaceOutputBroker struct {
	mu     sync.Mutex
	queues map[string]*surfaceOutputQueue
}

// NewSurfaceOutputBroker creates an empty broker.
func NewSurfaceOutputBroker() *SurfaceOutputBroker {
	return &SurfaceOutputBroker{queues: make(map[string]*surfaceOutputQueue)}
}

// Open creates a surface's queue and returns it for a pump to drain. Opening twice is a
// programming error and returns nil, which the caller treats as "already streaming".
func (b *SurfaceOutputBroker) Open(surfaceID string) *surfaceOutputQueue {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.queues[surfaceID]; exists {
		return nil
	}
	q := &surfaceOutputQueue{
		chunks: make(chan surfaceChunk, surfaceOutputQueueDepth),
		room:   make(chan struct{}, 1),
	}
	b.queues[surfaceID] = q
	return q
}

// Close ends a surface's stream. The pump sees the closed channel and returns, which is the whole
// termination path: no context, no stop flag, nothing that can be forgotten on one branch.
// Idempotent, because a surface can be closed by the user and by its session at the same moment.
func (b *SurfaceOutputBroker) Close(surfaceID string) {
	b.mu.Lock()
	q := b.queues[surfaceID]
	delete(b.queues, surfaceID)
	b.mu.Unlock()
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	close(q.chunks)
}

// Enqueue queues one chunk, waiting up to surfaceWriteTimeout for room.
//
// Returns false when the surface has no queue (it is gone — the caller turns that into a no-op)
// and ErrSurfaceConsumerBehind when the budget stayed full for the whole allowance.
func (b *SurfaceOutputBroker) Enqueue(surfaceID string, chunk surfaceChunk) (queued bool, err error) {
	b.mu.Lock()
	q := b.queues[surfaceID]
	b.mu.Unlock()
	if q == nil {
		return false, nil
	}
	return q.enqueue(chunk)
}

func (q *surfaceOutputQueue) enqueue(chunk surfaceChunk) (bool, error) {
	deadline := time.NewTimer(surfaceWriteTimeout)
	defer deadline.Stop()

	for {
		accepted, closed := q.tryReserve(len(chunk.data))
		if closed {
			return false, nil
		}
		if accepted {
			break
		}
		select {
		case <-q.room:
			// The pump drained something; try again.
		case <-deadline.C:
			return false, errSurfaceConsumerBehind
		}
	}

	// The send cannot block: the reservation above is what bounds the queue, and the channel is
	// deeper than the number of reservations the budget allows.
	select {
	case q.chunks <- chunk:
		return true, nil
	default:
		q.release(len(chunk.data))
		return false, errSurfaceConsumerBehind
	}
}

// tryReserve takes budget for a chunk. Reports whether it succeeded, and whether the queue is
// already closed — the second is not a failure, it is a surface that has gone away.
func (q *surfaceOutputQueue) tryReserve(n int) (accepted, closed bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false, true
	}
	// A single chunk larger than the whole budget is still accepted when the queue is empty:
	// refusing it would make the budget a second, quieter frame limit.
	if q.queued > 0 && q.queued+n > surfaceOutputBudgetBytes {
		return false, false
	}
	q.queued += n
	return true, false
}

// release returns budget after the pump has taken a chunk, and nudges any waiting writer.
func (q *surfaceOutputQueue) release(n int) {
	q.mu.Lock()
	q.queued -= n
	if q.queued < 0 {
		q.queued = 0
	}
	q.mu.Unlock()
	select {
	case q.room <- struct{}{}:
	default:
	}
}

// pumpSurfaceOutput drains one surface's queue into the presenter, batching per stream.
//
// Batching is per stream and not across them: a log surface colours stdout and stderr apart, and
// merging them would either lose that or interleave two lines into one. Ordering within a stream
// is preserved, which is the only ordering a reader of one stream can observe anyway.
func pumpSurfaceOutput(surfaceID string, q *surfaceOutputQueue, emit func(surfaceID, dataBase64, stream string)) {
	ticker := time.NewTicker(surfaceOutputBatchInterval)
	defer ticker.Stop()

	batch := newSurfaceBatch()
	flush := func() {
		for _, stream := range surfaceStreamOrder {
			data := batch.take(stream)
			if len(data) == 0 {
				continue
			}
			emit(surfaceID, encodeSurfaceOutput(data), stream)
		}
	}

	for {
		select {
		case chunk, ok := <-q.chunks:
			if !ok {
				// The surface closed. Whatever is already batched still belongs to the user: the
				// last lines of a log are exactly the ones worth keeping.
				flush()
				return
			}
			batch.add(chunk)
			q.release(len(chunk.data))
		case <-ticker.C:
			flush()
		}
	}
}

// startSurfaceOutputPump runs the pump for a surface in the background.
func startSurfaceOutputPump(surfaceID string, q *surfaceOutputQueue, emit func(surfaceID, dataBase64, stream string)) {
	if q == nil {
		return
	}
	safego.GoNamed("surface.streamOutput", func() { pumpSurfaceOutput(surfaceID, q, emit) })
}
