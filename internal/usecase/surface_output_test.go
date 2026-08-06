package usecase

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

// recordingEmitter stands in for the presenter on the pump's far side.
type recordingEmitter struct {
	mu      sync.Mutex
	batches []string
	done    chan struct{}
	once    sync.Once
}

func newRecordingEmitter() *recordingEmitter {
	return &recordingEmitter{done: make(chan struct{})}
}

func (e *recordingEmitter) emit(surfaceID, dataBase64, stream string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.batches = append(e.batches, stream+":"+dataBase64)
}

func (e *recordingEmitter) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.batches...)
}

func (e *recordingEmitter) decoded(t *testing.T, stream string) string {
	t.Helper()
	var b strings.Builder
	for _, entry := range e.snapshot() {
		parts := strings.SplitN(entry, ":", 2)
		if parts[0] != stream {
			continue
		}
		data, err := decodeSurfaceOutput(parts[1])
		if err != nil {
			t.Fatalf("emitted payload is not base64: %v", err)
		}
		b.Write(data)
	}
	return b.String()
}

func (e *recordingEmitter) waitFor(t *testing.T, want string, stream string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if e.decoded(t, stream) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %s = %q, want %q", stream, e.decoded(t, stream), want)
		}
		time.Sleep(surfaceOutputBatchInterval / 5)
	}
}

// A surface's bytes reach the emitter in order, however they were chunked on the way in.
func TestSurfaceOutputPreservesOrderWithinAStream(t *testing.T) {
	broker := NewSurfaceOutputBroker()
	emitter := newRecordingEmitter()
	q := broker.Open("srf-1")
	startSurfaceOutputPump("srf-1", q, emitter.emit)
	defer broker.Close("srf-1")

	for _, part := range []string{"alpha ", "beta ", "gamma"} {
		if _, err := broker.Enqueue("srf-1", surfaceChunk{data: []byte(part), stream: surfaceStreamStdout}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	emitter.waitFor(t, "alpha beta gamma", surfaceStreamStdout)
}

// The two streams are never merged into one batch: a log surface colours them apart, and splicing
// them would interleave two half-written lines.
func TestSurfaceOutputKeepsStreamsApart(t *testing.T) {
	broker := NewSurfaceOutputBroker()
	emitter := newRecordingEmitter()
	startSurfaceOutputPump("srf-1", broker.Open("srf-1"), emitter.emit)
	defer broker.Close("srf-1")

	for i := 0; i < 3; i++ {
		if _, err := broker.Enqueue("srf-1", surfaceChunk{data: []byte("out"), stream: surfaceStreamStdout}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
		if _, err := broker.Enqueue("srf-1", surfaceChunk{data: []byte("err"), stream: surfaceStreamStderr}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	emitter.waitFor(t, "outoutout", surfaceStreamStdout)
	emitter.waitFor(t, "errerrerr", surfaceStreamStderr)
}

// The promise ADR-015 makes and the reason the queue exists: a plugin that outruns the consumer is
// told to slow down instead of growing the host's memory. With no pump draining it, the budget
// fills and the next write is refused as ErrRateLimited (-32003 on the wire).
func TestSurfaceOutputRefusesAWriterThatOutrunsTheConsumer(t *testing.T) {
	broker := NewSurfaceOutputBroker()
	broker.Open("srf-1") // no pump: the consumer never takes anything

	chunk := make([]byte, 64<<10)
	var lastErr error
	for i := 0; i < (surfaceOutputBudgetBytes/len(chunk))+2; i++ {
		if _, err := broker.Enqueue("srf-1", surfaceChunk{data: chunk, stream: surfaceStreamStdout}); err != nil {
			lastErr = err
			break
		}
	}
	if !errors.Is(lastErr, domainplugin.ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited once the budget is full", lastErr)
	}
}

// A single chunk larger than the whole budget still goes through on an empty queue: refusing it
// would make the budget a second, quieter frame limit that no plugin author knows about.
func TestSurfaceOutputAcceptsOneOversizedChunk(t *testing.T) {
	broker := NewSurfaceOutputBroker()
	emitter := newRecordingEmitter()
	startSurfaceOutputPump("srf-1", broker.Open("srf-1"), emitter.emit)
	defer broker.Close("srf-1")

	big := strings.Repeat("x", surfaceOutputBudgetBytes+1024)
	if _, err := broker.Enqueue("srf-1", surfaceChunk{data: []byte(big), stream: surfaceStreamStdout}); err != nil {
		t.Fatalf("an oversized chunk on an empty queue must be accepted: %v", err)
	}
	emitter.waitFor(t, big, surfaceStreamStdout)
}

// Once the pump drains, a writer that was over budget can continue: backpressure is a pause, not a
// permanent refusal.
func TestSurfaceOutputRecoversAfterTheConsumerCatchesUp(t *testing.T) {
	broker := NewSurfaceOutputBroker()
	emitter := newRecordingEmitter()
	q := broker.Open("srf-1")
	defer broker.Close("srf-1")

	chunk := make([]byte, 64<<10)
	for i := 0; i < (surfaceOutputBudgetBytes/len(chunk))+2; i++ {
		if _, err := broker.Enqueue("srf-1", surfaceChunk{data: chunk, stream: surfaceStreamStdout}); err != nil {
			break
		}
	}

	startSurfaceOutputPump("srf-1", q, emitter.emit)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := broker.Enqueue("srf-1", surfaceChunk{data: []byte("after"), stream: surfaceStreamStdout}); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("a drained queue must accept writes again")
		}
	}
}

// Closing a surface ends its pump. Without that the goroutine outlives the tab it was feeding,
// which is exactly the leak §2.2 forbids: every goroutine needs a path to termination.
func TestClosingASurfaceStopsItsPump(t *testing.T) {
	broker := NewSurfaceOutputBroker()
	emitter := newRecordingEmitter()
	q := broker.Open("srf-1")

	stopped := make(chan struct{})
	go func() {
		pumpSurfaceOutput("srf-1", q, emitter.emit)
		close(stopped)
	}()

	if _, err := broker.Enqueue("srf-1", surfaceChunk{data: []byte("tail"), stream: surfaceStreamStdout}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	broker.Close("srf-1")

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the pump did not stop when its surface closed")
	}
	// What was already queued is still delivered: the last lines of a log are the ones worth having.
	if got := emitter.decoded(t, surfaceStreamStdout); got != "tail" {
		t.Fatalf("output queued before the close was dropped: %q", got)
	}
}

// Close is reached from the user, the session and the plugin process at once; a second one must
// not panic on an already closed channel.
func TestClosingASurfaceQueueTwiceIsSafe(t *testing.T) {
	broker := NewSurfaceOutputBroker()
	broker.Open("srf-1")
	broker.Close("srf-1")
	broker.Close("srf-1")
}

// A write naming a surface with no queue is not an error: it is the ordinary race where the tab
// closed a moment ago, which the caller turns into a no-op.
func TestEnqueueToAnUnknownSurfaceIsNotAnError(t *testing.T) {
	broker := NewSurfaceOutputBroker()
	queued, err := broker.Enqueue("srf-gone", surfaceChunk{data: []byte("x"), stream: surfaceStreamStdout})
	if err != nil {
		t.Fatalf("got %v, want no error", err)
	}
	if queued {
		t.Fatal("nothing can be queued for a surface with no queue")
	}
}

func TestDecodeSurfaceOutputRejectsGarbage(t *testing.T) {
	if _, err := decodeSurfaceOutput("this is not base64!!"); err == nil {
		t.Fatal("expected an undecodable payload to be refused")
	}
	data, err := decodeSurfaceOutput("")
	if err != nil || len(data) != 0 {
		t.Fatalf("an empty payload is empty, not an error: %v %v", data, err)
	}
}
