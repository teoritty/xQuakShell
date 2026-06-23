package usecase

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"ssh-client/internal/domain"
)

// TransferProgress describes transfer state for UI callbacks.
type TransferProgress struct {
	ID         string
	SessionID  string
	Direction  string
	LocalPath  string
	RemotePath string
	Done       int64
	Total      int64
	State      string
}

// TransferProgressFunc reports transfer progress to the presentation layer.
type TransferProgressFunc func(TransferProgress)

// TransferService orchestrates SFTP uploads and downloads with concurrency limits.
type TransferService struct {
	sessions   *SessionManager
	settings   *SettingsService
	localFS    domain.LocalFileSystem
	mu         sync.Mutex
	cond       *sync.Cond
	active     int
	cancelsMu  sync.Mutex
	cancels    map[string]context.CancelFunc
}

// NewTransferService creates a transfer orchestrator.
func NewTransferService(sessions *SessionManager, settings *SettingsService, localFS domain.LocalFileSystem) *TransferService {
	s := &TransferService{
		sessions: sessions,
		settings: settings,
		localFS:  localFS,
		cancels:    make(map[string]context.CancelFunc),
	}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// Upload copies a local file or directory to the remote path.
func (s *TransferService) Upload(ctx context.Context, sessionID, localPath, remotePath string, onProgress TransferProgressFunc) error {
	if s.localFS == nil {
		return fmt.Errorf("local file service unavailable")
	}
	resolved, err := s.localFS.ResolvePath(localPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if info.IsDir() {
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
	resolvedDir, err := s.localFS.ResolvePath(localDir)
	if err != nil {
		return err
	}
	if _, listErr := fs.List(sessionCtx, remotePath); listErr == nil {
		localTarget := filepath.Join(resolvedDir, filepath.Base(remotePath))
		if err := s.localFS.Mkdir(localTarget); err != nil {
			return err
		}
		return s.downloadRecursive(ctx, sessionID, remotePath, localTarget, onProgress)
	}
	return s.downloadFile(ctx, sessionID, remotePath, resolvedDir, onProgress)
}

// Cancel aborts an active transfer by ID.
func (s *TransferService) Cancel(transferID string) {
	s.cancelsMu.Lock()
	cancel, ok := s.cancels[transferID]
	delete(s.cancels, transferID)
	s.cancelsMu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
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
	limit := s.maxConcurrent()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			s.cond.Broadcast()
		case <-done:
		}
	}()
	s.mu.Lock()
	for s.active >= limit {
		s.cond.Wait()
		if ctx.Err() != nil {
			s.mu.Unlock()
			return ctx.Err()
		}
	}
	s.active++
	s.mu.Unlock()
	return nil
}

func (s *TransferService) releaseSlot() {
	s.mu.Lock()
	s.active--
	s.cond.Signal()
	s.mu.Unlock()
}

func (s *TransferService) registerCancel(transferID string, cancel context.CancelFunc) {
	s.cancelsMu.Lock()
	s.cancels[transferID] = cancel
	s.cancelsMu.Unlock()
}

func (s *TransferService) unregisterCancel(transferID string) {
	s.cancelsMu.Lock()
	delete(s.cancels, transferID)
	s.cancelsMu.Unlock()
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
	transferID := fmt.Sprintf("upload-%s-%s", sessionID, filepath.Base(localPath))
	s.registerCancel(transferID, cancel)
	defer s.unregisterCancel(transferID)

	progress := func(done, total int64) {
		if onProgress != nil {
			onProgress(TransferProgress{
				ID: transferID, SessionID: sessionID, Direction: "upload",
				LocalPath: localPath, RemotePath: remotePath,
				Done: done, Total: total, State: "active",
			})
		}
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- fs.Upload(ctx, localPath, remotePath, progress)
	}()
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
			ID: transferID, SessionID: sessionID, Direction: "upload",
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
	transferID := fmt.Sprintf("upload-%s-%s", sessionID, filepath.Base(localDir))
	s.registerCancel(transferID, cancel)
	defer s.unregisterCancel(transferID)

	progress := func(done, total int64) {
		if onProgress != nil {
			onProgress(TransferProgress{
				ID: transferID, SessionID: sessionID, Direction: "upload",
				LocalPath: localDir, RemotePath: remoteDir,
				Done: done, Total: total, State: "active",
			})
		}
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- fs.UploadRecursive(ctx, localDir, remoteDir, progress)
	}()
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
			ID: transferID, SessionID: sessionID, Direction: "upload",
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
	transferID := fmt.Sprintf("download-%s-%s", sessionID, filepath.Base(remoteDir))
	s.registerCancel(transferID, cancel)
	defer s.unregisterCancel(transferID)

	progress := func(done, total int64) {
		if onProgress != nil {
			onProgress(TransferProgress{
				ID: transferID, SessionID: sessionID, Direction: "download",
				LocalPath: localDir, RemotePath: remoteDir,
				Done: done, Total: total, State: "active",
			})
		}
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- fs.DownloadRecursive(ctx, remoteDir, localDir, progress)
	}()
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
			ID: transferID, SessionID: sessionID, Direction: "download",
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
	ctx, cancel := context.WithCancel(parentCtx)
	localPath := filepath.Join(localDir, filepath.Base(remotePath))
	if resolved, err := s.localFS.ResolvePath(localPath); err != nil {
		return err
	} else {
		localPath = resolved
	}
	transferID := fmt.Sprintf("download-%s-%s", sessionID, filepath.Base(remotePath))
	s.registerCancel(transferID, cancel)
	defer s.unregisterCancel(transferID)

	progress := func(done, total int64) {
		if onProgress != nil {
			onProgress(TransferProgress{
				ID: transferID, SessionID: sessionID, Direction: "download",
				LocalPath: localPath, RemotePath: remotePath,
				Done: done, Total: total, State: "active",
			})
		}
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- fs.Download(ctx, remotePath, localPath, progress)
	}()
	err = <-doneCh
	state := "completed"
	if err != nil {
		if ctx.Err() == context.Canceled {
			state = "cancelled"
			_ = os.Remove(localPath)
		} else {
			state = "failed"
		}
	}
	if onProgress != nil {
		onProgress(TransferProgress{
			ID: transferID, SessionID: sessionID, Direction: "download",
			LocalPath: localPath, RemotePath: remotePath,
			Done: 0, Total: 0, State: state,
		})
	}
	return err
}
