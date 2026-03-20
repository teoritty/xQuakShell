package sftp

import (
	"context"
	"io"

	"golang.org/x/time/rate"
)

// throttledReader wraps io.Reader and limits read throughput.
// rateLimitKbps: kilobits per second (0 = unlimited).
type throttledReader struct {
	r       io.Reader
	limiter *rate.Limiter
	ctx     context.Context
}

// newThrottledReader creates a reader that limits throughput to rateLimitKbps (0 = passthrough).
func newThrottledReader(ctx context.Context, r io.Reader, rateLimitKbps int) io.Reader {
	if rateLimitKbps <= 0 {
		return r
	}
	// Kbps -> bytes/sec: 1 Kbps = 128 bytes/sec
	bytesPerSec := rateLimitKbps * 128
	if bytesPerSec < 1 {
		bytesPerSec = 1
	}
	return &throttledReader{
		r:        r,
		limiter:  rate.NewLimiter(rate.Limit(bytesPerSec), bytesPerSec*2),
		ctx:      ctx,
	}
}

func (t *throttledReader) Read(p []byte) (n int, err error) {
	n, err = t.r.Read(p)
	if n > 0 && t.limiter != nil {
		if err := t.limiter.WaitN(t.ctx, n); err != nil {
			return n, err
		}
	}
	return n, err
}
