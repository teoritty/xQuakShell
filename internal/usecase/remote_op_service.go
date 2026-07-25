package usecase

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/safego"
)

// remoteOpSessionPort is the slice of SessionManager RemoteOpService needs:
// resolving a session's remote filesystem and its long-lived context. Depending
// on this abstraction (rather than *SessionManager) keeps the service testable.
type remoteOpSessionPort interface {
	GetRemoteFS(sessionID string) (domain.RemoteFS, error)
	GetSessionContext(sessionID string) (context.Context, error)
}

// RemoteOpService orchestrates long-running remote filesystem operations
// (recursive delete / chmod / chown) as cancellable background jobs that report
// progress through a TransferProgressFunc. It is the operation-level sibling of
// TransferService: TransferService moves bytes, RemoteOpService mutates the
// remote tree. Both share the application-wide CancelRegistry.
type RemoteOpService struct {
	sessions remoteOpSessionPort
	cancels  *CancelRegistry
}

// NewRemoteOpService creates a RemoteOpService over the shared cancel registry.
func NewRemoteOpService(sessions remoteOpSessionPort, cancels *CancelRegistry) *RemoteOpService {
	if cancels == nil {
		panic("usecase: RemoteOpService requires CancelRegistry")
	}
	return &RemoteOpService{sessions: sessions, cancels: cancels}
}

// Delete recursively deletes remotePath in the background, reporting progress.
func (s *RemoteOpService) Delete(sessionID, remotePath string, onProgress TransferProgressFunc) error {
	return s.run(sessionID, "delete", remotePath, domain.ApplyBoth, onProgress,
		func(ctx context.Context, fs domain.RemoteFS, onEach func()) error {
			return fs.RemoveAll(ctx, remotePath, onEach)
		})
}

// ChmodRecursive applies mode under remotePath in the background, reporting progress.
func (s *RemoteOpService) ChmodRecursive(sessionID, remotePath string, mode os.FileMode, applyTo domain.ApplyTarget, onProgress TransferProgressFunc) error {
	return s.run(sessionID, "chmod", remotePath, applyTo, onProgress,
		func(ctx context.Context, fs domain.RemoteFS, onEach func()) error {
			return fs.ChmodRecursive(ctx, remotePath, mode, applyTo, onEach)
		})
}

// ChownRecursive applies uid/gid under remotePath in the background, reporting progress.
func (s *RemoteOpService) ChownRecursive(sessionID, remotePath string, uid, gid int, applyTo domain.ApplyTarget, onProgress TransferProgressFunc) error {
	return s.run(sessionID, "chown", remotePath, applyTo, onProgress,
		func(ctx context.Context, fs domain.RemoteFS, onEach func()) error {
			return fs.ChownRecursive(ctx, remotePath, uid, gid, applyTo, onEach)
		})
}

// run resolves the session, then executes act in a cancellable goroutine in two
// phases: a read-only scan that computes the total (live counter, indeterminate
// bar), followed by the mutating action (accurate percentage). Errors before the
// goroutine starts are returned synchronously; everything after is reported via
// onProgress. The API method returns immediately so the UI stays responsive.
func (s *RemoteOpService) run(
	sessionID, kind, remotePath string,
	applyTo domain.ApplyTarget,
	onProgress TransferProgressFunc,
	act func(ctx context.Context, fs domain.RemoteFS, onEach func()) error,
) error {
	fs, err := s.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	parentCtx, err := s.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}

	opID := fmt.Sprintf("%s-%s-%s-%d", kind, sessionID, path.Base(remotePath), time.Now().UnixNano())
	// A remote-tree mutation declares no target directory: it has no
	// upload/download direction, and the UI derives the directory to refresh
	// from the operated-on path itself.
	rep := newOperationReporter(opID, sessionID, kind, "", onProgress).
		withLabel(remotePath).
		withDirection("")

	ctx, cancel := context.WithCancel(parentCtx)
	s.cancels.Register(opID, cancel)
	// Show the operation immediately (indeterminate "scanning" state) before the
	// scan even starts, so the panel is responsive on very large trees.
	rep.Started()

	safego.GoNamed("remoteop."+kind, func() {
		defer cancel()
		defer s.cancels.Unregister(opID)

		// Phase 1 — scan: count the entries to act on, streaming a live counter
		// with an indeterminate total.
		var scanned int64
		total, err := fs.CountTree(ctx, remotePath, applyTo, func() {
			scanned++
			rep.Report(scanned, 0, "active")
		})
		if err != nil {
			rep.Report(scanned, 0, terminalState(ctx, err))
			return
		}

		// Phase 2 — act: perform the operation, streaming an accurate percentage.
		var done int64
		if actErr := act(ctx, fs, func() {
			done++
			rep.Report(done, total, "active")
		}); actErr != nil {
			rep.Report(done, total, terminalState(ctx, actErr))
			return
		}
		rep.Report(total, total, "completed")
	})

	return nil
}

// terminalState maps an operation error to a UI state, distinguishing an
// explicit cancellation from a genuine failure.
func terminalState(ctx context.Context, err error) string {
	if err != nil && ctx.Err() == context.Canceled {
		return "cancelled"
	}
	return "failed"
}
