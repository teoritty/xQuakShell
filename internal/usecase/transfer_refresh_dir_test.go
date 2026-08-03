package usecase

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain"
)

// localTestDir is a host-shaped absolute directory: the legacy download paths
// build their refresh directory with filepath, so the expectation must use the
// platform separator too.
var localTestDir = filepath.FromSlash("/tmp/dl")

// The field-semantics invariant this file pins down:
//
//	RefreshDir — always a path; the single machine-readable answer to "which
//	             directory must the UI reload". Never a caption.
//	RemotePath — always a human-readable caption; never parsed as a path.
//
// The frontend used to guess the refresh directory by stripping the last
// segment off RemotePath whenever RefreshDir was empty, which turned a batch
// caption like "3 items" into the nonsense path "". That fallback is gone, so
// an emitter that leaves RefreshDir empty now means a tree that never
// refreshes. Every emitter is therefore enumerated here rather than tested
// one-by-one: the table is the audit.

// progressingRemoteFS is a fakeRemoteFS whose byte-moving methods tick the
// progress callback once, so the legacy TransferService paths emit their
// "active" event as well as their terminal one.
type progressingRemoteFS struct{ *fakeRemoteFS }

func (f progressingRemoteFS) Upload(_ context.Context, _, _ string, p domain.ProgressFunc) error {
	if p != nil {
		p(1, 2)
	}
	return nil
}

func (f progressingRemoteFS) Download(_ context.Context, _, _ string, p domain.ProgressFunc) error {
	if p != nil {
		p(1, 2)
	}
	return nil
}

func (f progressingRemoteFS) UploadRecursive(_ context.Context, _, _ string, p domain.ProgressFunc) error {
	if p != nil {
		p(1, 2)
	}
	return nil
}

func (f progressingRemoteFS) DownloadRecursive(_ context.Context, _, _ string, p domain.ProgressFunc) error {
	if p != nil {
		p(1, 2)
	}
	return nil
}

// refreshSink collects every emission. Some emitters report from a worker
// goroutine, so it locks.
type refreshSink struct {
	mu     sync.Mutex
	events []TransferProgress
	done   chan struct{}
	closed bool
}

func newRefreshSink() *refreshSink { return &refreshSink{done: make(chan struct{})} }

func (s *refreshSink) fn(p TransferProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, p)
	if isTerminalState(p.State) && !s.closed {
		s.closed = true
		close(s.done)
	}
}

func (s *refreshSink) all() []TransferProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]TransferProgress(nil), s.events...)
}

// waitDone blocks until a terminal event lands. Synchronous emitters have
// already produced one by the time this is called.
func (s *refreshSink) waitDone(t *testing.T) {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for a terminal event; got %+v", s.all())
	}
}

func refreshTestService(sessions remoteOpSessionPort, hostFS domain.HostFileSystem) *TransferService {
	return NewTransferService(sessions, nil, hostFS, newStubConcurrencyLimiter(1), NewCancelRegistry())
}

// Every emitter of TransferProgress fills RefreshDir with the directory the UI
// must reload — never with a caption, never empty.
func TestEveryEmitterFillsRefreshDirWithAPath(t *testing.T) {
	remoteFS := progressingRemoteFS{&fakeRemoteFS{count: 3}}
	sessions := &fakeOpSessions{fs: remoteFS}
	hostFS := &mockHostFS{
		statFn: func(string) (domain.HostFileInfo, error) {
			return domain.HostFileInfo{IsDir: false, Size: 1}, nil
		},
	}
	planner := NewTransferPlanner(sessions, hostFS, NewCancelRegistry())
	remoteOps := NewRemoteOpService(sessions, NewCancelRegistry())
	svc := refreshTestService(sessions, hostFS)

	cases := []struct {
		name string
		// run drives one emitter to completion.
		run func(t *testing.T, onProgress TransferProgressFunc)
		// wantRefreshDir is the directory every event of that emitter must carry.
		wantRefreshDir string
		// async marks emitters that report from a background goroutine, so the
		// assertions must wait for the terminal event. The synchronous ones have
		// finished emitting by the time run returns — and a successful plan
		// deliberately ends with no terminal event at all, because the executor
		// inherits the still-open panel item.
		async bool
	}{
		{
			name: "planner scan: upload into a remote directory",
			run: func(_ *testing.T, on TransferProgressFunc) {
				_, _ = planner.PlanUpload("s1", []string{"/src/a.txt"}, "/var/www", on)
			},
			wantRefreshDir: "/var/www",
		},
		{
			name: "planner scan: download into a local directory",
			run: func(_ *testing.T, on TransferProgressFunc) {
				_, _ = planner.PlanDownload("s1", []string{"/remote/a.txt"}, "/dst", on)
			},
			wantRefreshDir: "/dst",
		},
		{
			name: "planner scan: local copy into a local directory",
			run: func(_ *testing.T, on TransferProgressFunc) {
				_, _ = planner.PlanLocalCopy([]string{"/src/a.txt"}, "/dst", on)
			},
			wantRefreshDir: "/dst",
		},
		{
			name: "executor: batch whose caption is a count, not a path",
			run: func(_ *testing.T, on TransferProgressFunc) {
				plan := &TransferPlan{
					Kind: transferKindLocalCopy, OpID: "op-exec", DestDir: "/dst",
					Files: []PlannedFile{
						{Source: "/src/a.txt", Target: "/dst/a.txt", Size: 1},
						{Source: "/src/b.txt", Target: "/dst/b.txt", Size: 1},
					},
				}
				_ = svc.ExecutePlan(context.Background(), "", plan, nil, on)
			},
			wantRefreshDir: "/dst",
		},
		{
			name:           "remote op: delete removes the path, so its parent reloads",
			run:            func(_ *testing.T, on TransferProgressFunc) { _ = remoteOps.Delete("s1", "/srv/logs", on) },
			wantRefreshDir: "/srv",
			async:          true,
		},
		{
			name: "remote op: recursive chmod changes the operated directory",
			run: func(_ *testing.T, on TransferProgressFunc) {
				_ = remoteOps.ChmodRecursive("s1", "/srv/logs", os.FileMode(0o755), domain.ApplyBoth, on)
			},
			wantRefreshDir: "/srv/logs",
			async:          true,
		},
		{
			name: "remote op: recursive chown changes the operated directory",
			run: func(_ *testing.T, on TransferProgressFunc) {
				_ = remoteOps.ChownRecursive("s1", "/srv/logs", 0, 0, domain.ApplyBoth, on)
			},
			wantRefreshDir: "/srv/logs",
			async:          true,
		},
		{
			name: "legacy single-file upload lands in the remote file's parent",
			run: func(t *testing.T, on TransferProgressFunc) {
				if err := svc.uploadFile(context.Background(), "s1", "/src/a.txt", "/var/www/a.txt", on); err != nil {
					t.Fatal(err)
				}
			},
			wantRefreshDir: "/var/www",
		},
		{
			name: "legacy recursive upload creates the remote dir, so its parent reloads",
			run: func(t *testing.T, on TransferProgressFunc) {
				if err := svc.uploadRecursive(context.Background(), "s1", "/src/tree", "/var/www/tree", on); err != nil {
					t.Fatal(err)
				}
			},
			wantRefreshDir: "/var/www",
		},
		{
			name: "legacy single-file download lands in the chosen local directory",
			run: func(t *testing.T, on TransferProgressFunc) {
				if err := svc.downloadFile(context.Background(), "s1", "/remote/a.txt", localTestDir, on); err != nil {
					t.Fatal(err)
				}
			},
			wantRefreshDir: localTestDir,
		},
		{
			name: "legacy recursive download creates the local dir, so its parent reloads",
			run: func(t *testing.T, on TransferProgressFunc) {
				if err := svc.downloadRecursive(context.Background(), "s1", "/remote/tree", filepath.Join(localTestDir, "tree"), on); err != nil {
					t.Fatal(err)
				}
			},
			wantRefreshDir: localTestDir,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := newRefreshSink()
			tc.run(t, sink.fn)
			if tc.async {
				sink.waitDone(t)
			}

			events := sink.all()
			if len(events) == 0 {
				t.Fatal("emitter produced no events at all")
			}
			for _, e := range events {
				if e.RefreshDir == "" {
					t.Fatalf("RefreshDir is empty: %+v — the frontend has no fallback any more", e)
				}
				if e.RefreshDir != tc.wantRefreshDir {
					t.Fatalf("RefreshDir = %q, want %q (event %+v)", e.RefreshDir, tc.wantRefreshDir, e)
				}
				if !looksLikeAPath(e.RefreshDir) {
					t.Fatalf("RefreshDir = %q is not a path", e.RefreshDir)
				}
			}
		})
	}
}

// looksLikeAPath rejects the captions RemotePath is allowed to carry ("3
// items", "Scanning …") while accepting both POSIX remote paths and host paths.
func looksLikeAPath(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t") {
		return false
	}
	return strings.ContainsRune(s, '/') || strings.ContainsRune(s, '\\')
}

// The two fields are independent: a batch's caption is a count while its
// RefreshDir stays the real destination. This is the exact pair the old
// frontend regex conflated.
func TestBatchCaptionNeverLeaksIntoRefreshDir(t *testing.T) {
	sink := newRefreshSink()
	svc := refreshTestService(nil, &mockHostFS{})
	plan := &TransferPlan{
		Kind: transferKindLocalCopy, OpID: "op-batch", DestDir: "/dst",
		Files: []PlannedFile{
			{Source: "/src/a.txt", Target: "/dst/a.txt", Size: 1},
			{Source: "/src/b.txt", Target: "/dst/b.txt", Size: 1},
			{Source: "/src/c.txt", Target: "/dst/c.txt", Size: 1},
		},
	}
	if err := svc.ExecutePlan(context.Background(), "", plan, nil, sink.fn); err != nil {
		t.Fatal(err)
	}

	for _, e := range sink.all() {
		if e.RemotePath != "3 items" {
			t.Fatalf("RemotePath = %q, want the batch caption %q", e.RemotePath, "3 items")
		}
		if e.RefreshDir != "/dst" {
			t.Fatalf("RefreshDir = %q, want /dst", e.RefreshDir)
		}
	}
}
