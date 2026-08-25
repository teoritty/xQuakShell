package usecase

import "encoding/base64"

// maxLocalTerminalBatchBytes caps how much output goes into one event.
//
// The cap matters more than it looks. A shell printing a large file hands over chunks far faster
// than a WebView repaints, and without coalescing that is one event, one IPC crossing and one
// repaint per chunk - the shape that made plugin surfaces need a broker. Batching turns a burst
// into a handful of events; the cap stops a single event growing without bound while the reader
// is still producing.
const maxLocalTerminalBatchBytes = 64 * 1024

// pumpLocalTerminalOutput moves a shell's bytes to the UI, coalescing bursts.
//
// No queue and no rate limiting here, unlike the plugin surface path. That broker exists because
// its producer is a separate process that has to be told when the consumer is behind; this
// producer is a read loop on the other side of a bounded channel, so it simply blocks and the
// backpressure is already correct. What is left to do is batching.
//
// emit is called on this goroutine and must not block for long. done runs once the shell's output
// channel closes, which is the signal that the process is gone.
func pumpLocalTerminalOutput(id string, out <-chan []byte, emit func(id, dataBase64 string), done func()) {
	defer done()

	for first := range out {
		batch := append([]byte(nil), first...)
		batch = drainAvailable(batch, out)
		emit(id, base64.StdEncoding.EncodeToString(batch))
	}
}

// drainAvailable takes whatever is already waiting without blocking, so a burst that has already
// arrived travels as one event instead of one per chunk. It stops at the cap and lets the next
// iteration send the rest.
func drainAvailable(batch []byte, out <-chan []byte) []byte {
	for len(batch) < maxLocalTerminalBatchBytes {
		select {
		case next, ok := <-out:
			if !ok {
				return batch
			}
			batch = append(batch, next...)
		default:
			return batch
		}
	}
	return batch
}
