package sftp

import (
	"context"
	"io"

	"ssh-client/internal/domain"
)

// progressReader wraps an io.Reader to report cumulative bytes read and to
// abort promptly when ctx is cancelled. It is read by a single goroutine
// (sftp's ReadFrom feeds from one goroutine even with concurrent writes), so
// it needs no synchronization.
//
// It exposes Size() so that sftp.File.ReadFrom can size its concurrency: the
// library only pipelines writes when the source reader advertises its length
// via Len()/Size()/Stat(); otherwise it silently falls back to a slow,
// serialized upload.
type progressReader struct {
	r        io.Reader
	ctx      context.Context
	total    int64
	done     int64
	progress domain.ProgressFunc
}

func (pr *progressReader) Read(p []byte) (int, error) {
	select {
	case <-pr.ctx.Done():
		return 0, pr.ctx.Err()
	default:
	}
	n, err := pr.r.Read(p)
	if n > 0 {
		pr.done += int64(n)
		if pr.progress != nil {
			pr.progress(pr.done, pr.total)
		}
	}
	return n, err
}

// Size reports the total number of bytes to be read, enabling ReadFrom's
// concurrent upload path.
func (pr *progressReader) Size() int64 { return pr.total }

// progressWriter wraps an io.Writer to report cumulative bytes written and to
// abort promptly when ctx is cancelled. sftp.File.WriteTo writes to it
// sequentially from a single goroutine, so it needs no synchronization.
type progressWriter struct {
	w        io.Writer
	ctx      context.Context
	total    int64
	done     int64
	progress domain.ProgressFunc
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	select {
	case <-pw.ctx.Done():
		return 0, pw.ctx.Err()
	default:
	}
	n, err := pw.w.Write(p)
	if n > 0 {
		pw.done += int64(n)
		if pw.progress != nil {
			pw.progress(pw.done, pw.total)
		}
	}
	return n, err
}
