package sftp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestProgressReaderCountsAndReportsProgress(t *testing.T) {
	const data = "hello, sftp world"
	var reports [][2]int64
	pr := &progressReader{
		r:     strings.NewReader(data),
		ctx:   context.Background(),
		total: int64(len(data)),
		progress: func(done, total int64) {
			reports = append(reports, [2]int64{done, total})
		},
	}

	got, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != data {
		t.Fatalf("content mismatch: got %q want %q", got, data)
	}
	if len(reports) == 0 {
		t.Fatal("expected at least one progress report")
	}
	last := reports[len(reports)-1]
	if last[0] != int64(len(data)) || last[1] != int64(len(data)) {
		t.Fatalf("final progress = %v, want done=total=%d", last, len(data))
	}
	// done must be monotonically non-decreasing.
	for i := 1; i < len(reports); i++ {
		if reports[i][0] < reports[i-1][0] {
			t.Fatalf("progress went backwards: %v then %v", reports[i-1], reports[i])
		}
	}
}

func TestProgressReaderSizeEnablesConcurrency(t *testing.T) {
	pr := &progressReader{r: strings.NewReader("x"), ctx: context.Background(), total: 42}
	// ReadFrom's concurrency path type-asserts exactly this interface.
	sizer, ok := interface{}(pr).(interface{ Size() int64 })
	if !ok {
		t.Fatal("progressReader must implement Size() int64 to enable concurrent uploads")
	}
	if sizer.Size() != 42 {
		t.Fatalf("Size() = %d, want 42", sizer.Size())
	}
}

func TestProgressReaderCancelStopsRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pr := &progressReader{r: strings.NewReader("data"), ctx: ctx, total: 4}
	n, err := pr.Read(make([]byte, 4))
	if n != 0 {
		t.Fatalf("expected 0 bytes on cancelled ctx, got %d", n)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestProgressWriterCountsAndReportsProgress(t *testing.T) {
	const data = "download payload chunked"
	var buf bytes.Buffer
	var lastDone int64
	pw := &progressWriter{
		w:        &buf,
		ctx:      context.Background(),
		total:    int64(len(data)),
		progress: func(done, total int64) { lastDone = done },
	}

	// Write in two chunks to confirm cumulative counting.
	if _, err := pw.Write([]byte(data[:10])); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	if _, err := pw.Write([]byte(data[10:])); err != nil {
		t.Fatalf("write 2: %v", err)
	}
	if buf.String() != data {
		t.Fatalf("content mismatch: got %q want %q", buf.String(), data)
	}
	if lastDone != int64(len(data)) {
		t.Fatalf("final done = %d, want %d", lastDone, len(data))
	}
}

func TestProgressWriterCancelStopsWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	pw := &progressWriter{w: &buf, ctx: ctx, total: 4}
	n, err := pw.Write([]byte("data"))
	if n != 0 {
		t.Fatalf("expected 0 bytes on cancelled ctx, got %d", n)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected nothing written, got %q", buf.String())
	}
}
