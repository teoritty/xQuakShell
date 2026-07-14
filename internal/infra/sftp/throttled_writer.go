package sftp

import (
	"context"
	"io"

	"golang.org/x/time/rate"
)

// throttledWriter wraps io.Writer and limits write throughput. It is the
// symmetric counterpart to throttledReader, used for downloads where the
// destination writer is the stream under our control (sftp.File.WriteTo owns
// the read side).
// rateLimitKbps: kilobits per second (0 = unlimited).
type throttledWriter struct {
	w       io.Writer
	limiter *rate.Limiter
	ctx     context.Context
}

// newThrottledWriter creates a writer that limits throughput to rateLimitKbps (0 = passthrough).
func newThrottledWriter(ctx context.Context, w io.Writer, rateLimitKbps int) io.Writer {
	if rateLimitKbps <= 0 {
		return w
	}
	// Kbps -> bytes/sec: 1 Kbps = 128 bytes/sec
	bytesPerSec := rateLimitKbps * 128
	if bytesPerSec < 1 {
		bytesPerSec = 1
	}
	return &throttledWriter{
		w:       w,
		limiter: rate.NewLimiter(rate.Limit(bytesPerSec), bytesPerSec*2),
		ctx:     ctx,
	}
}

func (t *throttledWriter) Write(p []byte) (n int, err error) {
	n, err = t.w.Write(p)
	if n > 0 && t.limiter != nil {
		if werr := t.limiter.WaitN(t.ctx, n); werr != nil {
			return n, werr
		}
	}
	return n, err
}
