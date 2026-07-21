package usecase

import (
	"context"
	"fmt"
	"path/filepath"

	"ssh-client/internal/domain"
	"ssh-client/internal/pkg/safego"
)

// TransferProgress describes the state of a long-running operation for UI
// callbacks. Despite the name it now covers not only byte transfers
// (upload/download) but also remote filesystem operations (delete, recursive
// chmod/chown) — see Kind. TECH DEBT: the "Transfer" naming (this type, the
// Wails event, and the frontend store) is kept for pragmatic reuse; a future
// refactor should rename these to a generic "Operation" vocabulary.
type TransferProgress struct {
	ID         string
	SessionID  string
	Kind       string // "upload" | "download" | "delete" | "chmod" | "chown"
	Direction  string
	LocalPath  string
	RemotePath string
	// RefreshDir is the destination directory the UI should reload when the
	// operation finishes. RemotePath may be a human label for batches ("3
	// items"), so it must never be parsed as a path; prefer this field.
	RefreshDir string
	Done       int64
	Total      int64
	State      string
}

// TransferProgressFunc reports transfer progress to the presentation layer.
type TransferProgressFunc func(TransferProgress)

// TransferService orchestrates SFTP uploads and downloads with concurrency limits.
type TransferService struct {
	sessions *SessionManager
	settings *SettingsService
	hostFS   domain.HostFileSystem
	limiter  domain.ConcurrencyLimiter
	cancels  *cancelRegistry
}

// NewTransferService creates a transfer orchestrator.
func NewTransferService(sessions *SessionManager, settings *SettingsService, hostFS domain.HostFileSystem, limiter domain.ConcurrencyLimiter) *TransferService {
	if limiter == nil {
		panic("usecase: TransferService requires ConcurrencyLimiter")
	}
	return &TransferService{
		sessions: sessions,
		settings: settings,
		hostFS:   hostFS,
		limiter:  limiter,
		cancels:  newCancelRegistry(),
	}
}

// Upload copies a local file or directory to the remote path.
func (s *TransferService) Upload(ctx context.Context, sessionID, localPath, remotePath string, onProgress TransferProgressFunc) error {
	if s.hostFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	resolved, err := s.hostFS.ResolvePath(localPath)
	if err != nil {
		return err
	}
	info, err := s.hostFS.Stat(localPath)
	if err != nil {
		return err
	}
	if info.IsDir {
		return s.uploadRecursive(ctx, sessionID, resolved, remotePath, onProgress)
	}
	return s.uploadFile(ctx, sessionID, resolved, remotePath, onProgress)
}

// Download copies a remote file or directory to a local directory.
func (s *TransferService) Download(ctx context.Context, sessionID, remotePath, localDir string, onProgress TransferProgressFunc) error {
	fs, err := s.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	sessionCtx, err := s.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	resolvedDir, err := s.hostFS.ResolvePath(localDir)
	if err != nil {
		return err
	}
	if _, listErr := fs.List(sessionCtx, remotePath); listErr == nil {
		localTarget := filepath.Join(resolvedDir, filepath.Base(remotePath))
		if err := s.hostFS.Mkdir(localTarget); err != nil {
			return err
		}
		return s.downloadRecursive(ctx, sessionID, remotePath, localTarget, onProgress)
	}
	return s.downloadFile(ctx, sessionID, remotePath, resolvedDir, onProgress)
}

// Cancel aborts an active transfer by ID. Returns true if a transfer was found.
func (s *TransferService) Cancel(transferID string) bool {
	return s.cancels.Cancel(transferID)
}

func (s *TransferService) maxConcurrent() int {
	limit := 4
	if s.settings != nil {
		if settings, err := s.settings.GetSettings(); err == nil && settings.Transfer.MaxConcurrent > 0 {
			limit = settings.Transfer.MaxConcurrent
		}
	}
	return limit
}

func (s *TransferService) acquireSlot(ctx context.Context) error {
	s.limiter.SetLimit(s.maxConcurrent())
	return s.limiter.Acquire(ctx)
}

func (s *TransferService) releaseSlot() {
	s.limiter.Release()
}

func (s *TransferService) uploadFile(parentCtx context.Context, sessionID, localPath, remotePath string, onProgress TransferProgressFunc) error {
	fs, err := s.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	if err := s.acquireSlot(parentCtx); err != nil {
		return err
	}
	defer s.releaseSlot()
	ctx, cancel := context.WithCancel(parentCtx)
	// defer cancel() releases this child context's resources as soon as this
	// function returns, on every path (success, failure, or cancellation).
	// Without it, every transfer created a child of the session's long-lived
	// context (parentCtx, from SessionManager.GetSessionContext) and never
	// released it: context.WithCancel registers the child in its parent's
	// internal children map, and that registration is only removed when
	// cancel() actually runs — never automatically when this function
	// returns. Each leaked entry is small on its own, but a long session
	// with many uploads/downloads accumulates them for as long as the
	// session stays open, which is a real (if slow) resource leak. This is a
	// pre-existing bug, fixed here; the same pattern is fixed in
	// uploadRecursive, downloadRecursive, and downloadFile below.
	defer cancel()
	transferID := fmt.Sprintf("upload-%s-%s", sessionID, filepath.Base(localPath))
	s.cancels.Register(transferID, cancel)
	defer s.cancels.Unregister(transferID)

	progress := func(done, total int64) {
		if onProgress != nil {
			onProgress(TransferProgress{
				ID: transferID, SessionID: sessionID, Kind: "upload", Direction: "upload",
				LocalPath: localPath, RemotePath: remotePath,
				Done: done, Total: total, State: "active",
			})
		}
	}

	doneCh := make(chan error, 1)
	safego.GoNamed("transfer.upload", func() {
		doneCh <- fs.Upload(ctx, localPath, remotePath, progress)
	})
	err = <-doneCh

	state := "completed"
	if err != nil {
		if ctx.Err() == context.Canceled {
			state = "cancelled"
			_ = fs.Remove(context.Background(), remotePath)
		} else {
			state = "failed"
		}
	}
	if onProgress != nil {
		onProgress(TransferProgress{
			ID: transferID, SessionID: sessionID, Kind: "upload", Direction: "upload",
			LocalPath: localPath, RemotePath: remotePath,
			Done: 0, Total: 0, State: state,
		})
	}
	return err
}

func (s *TransferService) uploadRecursive(parentCtx context.Context, sessionID, localDir, remoteDir string, onProgress TransferProgressFunc) error {
	fs, err := s.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	if err := s.acquireSlot(parentCtx); err != nil {
		return err
	}
	defer s.releaseSlot()
	ctx, cancel := context.WithCancel(parentCtx)
	// defer cancel() — see the comment in uploadFile above for why this
	// matters; same context-leak pattern fixed here.
	defer cancel()
	transferID := fmt.Sprintf("upload-%s-%s", sessionID, filepath.Base(localDir))
	s.cancels.Register(transferID, cancel)
	defer s.cancels.Unregister(transferID)

	progress := func(done, total int64) {
		if onProgress != nil {
			onProgress(TransferProgress{
				ID: transferID, SessionID: sessionID, Kind: "upload", Direction: "upload",
				LocalPath: localDir, RemotePath: remoteDir,
				Done: done, Total: total, State: "active",
			})
		}
	}

	doneCh := make(chan error, 1)
	safego.GoNamed("transfer.uploadRecursive", func() {
		doneCh <- fs.UploadRecursive(ctx, localDir, remoteDir, progress)
	})
	err = <-doneCh
	state := "completed"
	if err != nil {
		if ctx.Err() == context.Canceled {
			state = "cancelled"
		} else {
			state = "failed"
		}
	}
	if onProgress != nil {
		onProgress(TransferProgress{
			ID: transferID, SessionID: sessionID, Kind: "upload", Direction: "upload",
			LocalPath: localDir, RemotePath: remoteDir,
			Done: 0, Total: 0, State: state,
		})
	}
	return err
}

func (s *TransferService) downloadRecursive(parentCtx context.Context, sessionID, remoteDir, localDir string, onProgress TransferProgressFunc) error {
	fs, err := s.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	if err := s.acquireSlot(parentCtx); err != nil {
		return err
	}
	defer s.releaseSlot()
	ctx, cancel := context.WithCancel(parentCtx)
	// defer cancel() — see the comment in uploadFile above for why this
	// matters; same context-leak pattern fixed here.
	defer cancel()
	transferID := fmt.Sprintf("download-%s-%s", sessionID, filepath.Base(remoteDir))
	s.cancels.Register(transferID, cancel)
	defer s.cancels.Unregister(transferID)

	progress := func(done, total int64) {
		if onProgress != nil {
			onProgress(TransferProgress{
				ID: transferID, SessionID: sessionID, Kind: "download", Direction: "download",
				LocalPath: localDir, RemotePath: remoteDir,
				Done: done, Total: total, State: "active",
			})
		}
	}

	doneCh := make(chan error, 1)
	safego.GoNamed("transfer.downloadRecursive", func() {
		doneCh <- fs.DownloadRecursive(ctx, remoteDir, localDir, progress)
	})
	err = <-doneCh
	state := "completed"
	if err != nil {
		if ctx.Err() == context.Canceled {
			state = "cancelled"
		} else {
			state = "failed"
		}
	}
	if onProgress != nil {
		onProgress(TransferProgress{
			ID: transferID, SessionID: sessionID, Kind: "download", Direction: "download",
			LocalPath: localDir, RemotePath: remoteDir,
			Done: 0, Total: 0, State: state,
		})
	}
	return err
}

func (s *TransferService) downloadFile(parentCtx context.Context, sessionID, remotePath, localDir string, onProgress TransferProgressFunc) error {
	fs, err := s.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	if err := s.acquireSlot(parentCtx); err != nil {
		return err
	}
	defer s.releaseSlot()
	localPath := filepath.Join(localDir, filepath.Base(remotePath))
	if resolved, err := s.hostFS.ResolvePath(localPath); err != nil {
		return err
	} else {
		localPath = resolved
	}
	ctx, cancel := context.WithCancel(parentCtx)
	// defer cancel() — see the comment in uploadFile above for why this
	// matters; same context-leak pattern fixed here.
	defer cancel()
	transferID := fmt.Sprintf("download-%s-%s", sessionID, filepath.Base(remotePath))
	s.cancels.Register(transferID, cancel)
	defer s.cancels.Unregister(transferID)

	progress := func(done, total int64) {
		if onProgress != nil {
			onProgress(TransferProgress{
				ID: transferID, SessionID: sessionID, Kind: "download", Direction: "download",
				LocalPath: localPath, RemotePath: remotePath,
				Done: done, Total: total, State: "active",
			})
		}
	}

	doneCh := make(chan error, 1)
	safego.GoNamed("transfer.download", func() {
		doneCh <- fs.Download(ctx, remotePath, localPath, progress)
	})
	err = <-doneCh
	state := "completed"
	if err != nil {
		if ctx.Err() == context.Canceled {
			state = "cancelled"
			_ = s.hostFS.Remove(localPath)
		} else {
			state = "failed"
		}
	}
	if onProgress != nil {
		onProgress(TransferProgress{
			ID: transferID, SessionID: sessionID, Kind: "download", Direction: "download",
			LocalPath: localPath, RemotePath: remotePath,
			Done: 0, Total: 0, State: state,
		})
	}
	return err
}
