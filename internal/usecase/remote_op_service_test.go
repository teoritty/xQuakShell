package usecase

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain"
)

// fakeRemoteFS is a minimal domain.RemoteFS for exercising RemoteOpService.
// It models a flat tree of `count` entries: CountTree reports `count` (ticking
// onEach per entry) and RemoveAll deletes `count` entries (ticking onEach),
// honouring context cancellation between entries.
type fakeRemoteFS struct {
	count       int
	perItemWait time.Duration // optional delay per removed entry (for cancel tests)
}

func (f *fakeRemoteFS) CountTree(ctx context.Context, _ string, _ domain.ApplyTarget, onEach func()) (int64, error) {
	for i := 0; i < f.count; i++ {
		select {
		case <-ctx.Done():
			return int64(i), ctx.Err()
		default:
		}
		if onEach != nil {
			onEach()
		}
	}
	return int64(f.count), nil
}

func (f *fakeRemoteFS) RemoveAll(ctx context.Context, _ string, onEach func()) error {
	for i := 0; i < f.count; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if f.perItemWait > 0 {
			time.Sleep(f.perItemWait)
		}
		if onEach != nil {
			onEach()
		}
	}
	return nil
}

func (f *fakeRemoteFS) ChmodRecursive(ctx context.Context, _ string, _ os.FileMode, _ domain.ApplyTarget, onEach func()) error {
	return f.RemoveAll(ctx, "", onEach)
}
func (f *fakeRemoteFS) ChownRecursive(ctx context.Context, _ string, _, _ int, _ domain.ApplyTarget, onEach func()) error {
	return f.RemoveAll(ctx, "", onEach)
}

// Unused interface methods.
func (f *fakeRemoteFS) GetWorkingDirectory(context.Context) (string, error)          { return "", nil }
func (f *fakeRemoteFS) List(context.Context, string) ([]domain.RemoteNode, error)     { return nil, nil }
func (f *fakeRemoteFS) Upload(context.Context, string, string, domain.ProgressFunc) error {
	return nil
}
func (f *fakeRemoteFS) Download(context.Context, string, string, domain.ProgressFunc) error {
	return nil
}
func (f *fakeRemoteFS) UploadRecursive(context.Context, string, string, domain.ProgressFunc) error {
	return nil
}
func (f *fakeRemoteFS) DownloadRecursive(context.Context, string, string, domain.ProgressFunc) error {
	return nil
}
func (f *fakeRemoteFS) Mkdir(context.Context, string) error              { return nil }
func (f *fakeRemoteFS) CreateFile(context.Context, string) error         { return nil }
func (f *fakeRemoteFS) Remove(context.Context, string) error             { return nil }
func (f *fakeRemoteFS) Rename(context.Context, string, string) error     { return nil }
func (f *fakeRemoteFS) Chmod(context.Context, string, os.FileMode) error { return nil }
func (f *fakeRemoteFS) Chown(context.Context, string, int, int) error    { return nil }
func (f *fakeRemoteFS) Close() error                                     { return nil }

type fakeOpSessions struct {
	fs  domain.RemoteFS
	ctx context.Context
}

func (s *fakeOpSessions) GetRemoteFS(string) (domain.RemoteFS, error) { return s.fs, nil }
func (s *fakeOpSessions) GetSessionContext(string) (context.Context, error) {
	if s.ctx != nil {
		return s.ctx, nil
	}
	return context.Background(), nil
}

// collector gathers progress events and signals once a terminal state arrives.
type collector struct {
	mu     sync.Mutex
	events []TransferProgress
	done   chan TransferProgress
}

func newCollector() *collector { return &collector{done: make(chan TransferProgress, 1)} }

func (c *collector) fn(p TransferProgress) {
	c.mu.Lock()
	c.events = append(c.events, p)
	c.mu.Unlock()
	if p.State == "completed" || p.State == "failed" || p.State == "cancelled" {
		select {
		case c.done <- p:
		default:
		}
	}
}

func (c *collector) all() []TransferProgress {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]TransferProgress(nil), c.events...)
}

func waitTerminal(t *testing.T, c *collector) TransferProgress {
	t.Helper()
	select {
	case p := <-c.done:
		return p
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for terminal operation state")
		return TransferProgress{}
	}
}

func TestRemoteOpDeleteReportsAccurateTotal(t *testing.T) {
	svc := NewRemoteOpService(&fakeOpSessions{fs: &fakeRemoteFS{count: 10}})
	c := newCollector()

	if err := svc.Delete("s1", "/tmp/big", c.fn); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	final := waitTerminal(t, c)
	if final.State != "completed" {
		t.Fatalf("expected completed, got %q", final.State)
	}
	if final.Kind != "delete" {
		t.Fatalf("expected kind delete, got %q", final.Kind)
	}
	if final.Done != 10 || final.Total != 10 {
		t.Fatalf("expected done/total 10/10, got %d/%d", final.Done, final.Total)
	}

	// The very first event must appear immediately as active (panel responsiveness).
	events := c.all()
	if events[0].State != "active" {
		t.Fatalf("expected first event active, got %q", events[0].State)
	}
}

func TestRemoteOpDeleteCancellation(t *testing.T) {
	svc := NewRemoteOpService(&fakeOpSessions{fs: &fakeRemoteFS{count: 100000, perItemWait: 200 * time.Microsecond}})
	c := newCollector()

	if err := svc.Delete("s1", "/tmp/huge", c.fn); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	// Grab the operation ID from the first event and cancel mid-flight.
	var opID string
	deadline := time.After(2 * time.Second)
	for opID == "" {
		select {
		case <-deadline:
			t.Fatal("no events to derive op ID")
		default:
		}
		if ev := c.all(); len(ev) > 0 {
			opID = ev[0].ID
		}
	}
	if !svc.Cancel(opID) {
		// Operation may have a brief window before registration; retry once.
		time.Sleep(5 * time.Millisecond)
		svc.Cancel(opID)
	}

	final := waitTerminal(t, c)
	if final.State != "cancelled" {
		t.Fatalf("expected cancelled, got %q", final.State)
	}
}

func TestRemoteOpDeletePropagatesResolveError(t *testing.T) {
	// A session that fails to resolve returns the error synchronously.
	failing := &failingOpSessions{err: errors.New("no session")}
	svc := NewRemoteOpService(failing)
	if err := svc.Delete("s1", "/x", func(TransferProgress) {}); err == nil {
		t.Fatal("expected synchronous error when session cannot be resolved")
	}
}

type failingOpSessions struct{ err error }

func (s *failingOpSessions) GetRemoteFS(string) (domain.RemoteFS, error) { return nil, s.err }
func (s *failingOpSessions) GetSessionContext(string) (context.Context, error) {
	return context.Background(), nil
}
