package loghub

import (
	"bytes"
	"sync"
)

// LineWriter adapts io.Writer for log.Printf output into the hub.
type LineWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// NewLineWriter creates a writer that publishes complete lines to the hub.
func NewLineWriter() *LineWriter {
	return &LineWriter{}
}

func (w *LineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	for {
		line, ok := w.nextLine()
		if !ok {
			break
		}
		PublishStdLog(line)
	}
	return n, err
}

func (w *LineWriter) nextLine() (string, bool) {
	data := w.buf.Bytes()
	idx := bytes.IndexByte(data, '\n')
	if idx < 0 {
		return "", false
	}
	line := string(data[:idx])
	w.buf.Next(idx + 1)
	return line, true
}
