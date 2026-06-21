package wails

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// acquireTransferSlot blocks until a transfer slot is available. Call releaseTransferSlot when done.
func (a *AppAPI) acquireTransferSlot(ctx context.Context) error {
	limit := 4
	if data, err := a.vaultRepo.GetData(); err == nil && data.Settings != nil && data.Settings.Transfer.MaxConcurrent > 0 {
		limit = data.Settings.Transfer.MaxConcurrent
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			a.transferCond.Broadcast()
		case <-done:
		}
	}()
	a.transferCond.L.Lock()
	for a.transferActive >= limit {
		a.transferCond.Wait()
		if ctx.Err() != nil {
			a.transferCond.L.Unlock()
			return ctx.Err()
		}
	}
	a.transferActive++
	a.transferCond.L.Unlock()
	return nil
}

func (a *AppAPI) releaseTransferSlot() {
	a.transferCond.L.Lock()
	a.transferActive--
	a.transferCond.Signal()
	a.transferCond.L.Unlock()
}

// Upload copies a local file or directory to the remote path (recursive for directories).
func (a *AppAPI) Upload(sessionID, localPath, remotePath string) error {
	localPath = sanitizeLocalPath(localPath)
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return a.uploadRecursive(sessionID, localPath, remotePath)
	}
	return a.uploadFile(sessionID, localPath, remotePath)
}

func (a *AppAPI) uploadFile(sessionID, localPath, remotePath string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	parentCtx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	if err := a.acquireTransferSlot(parentCtx); err != nil {
		return err
	}
	defer a.releaseTransferSlot()
	ctx, cancel := context.WithCancel(parentCtx)
	transferID := fmt.Sprintf("upload-%s-%s", sessionID, filepath.Base(localPath))
	a.transferCancelsMu.Lock()
	a.transferCancels[transferID] = cancel
	a.transferCancelsMu.Unlock()
	defer func() {
		a.transferCancelsMu.Lock()
		delete(a.transferCancels, transferID)
		a.transferCancelsMu.Unlock()
	}()

	progress := func(done, total int64) {
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
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
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
			ID: transferID, SessionID: sessionID, Direction: "upload",
			LocalPath: localPath, RemotePath: remotePath,
			Done: 0, Total: 0, State: state,
		})
	}
	return err
}

func (a *AppAPI) uploadRecursive(sessionID, localDir, remoteDir string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	parentCtx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	if err := a.acquireTransferSlot(parentCtx); err != nil {
		return err
	}
	defer a.releaseTransferSlot()
	ctx, cancel := context.WithCancel(parentCtx)
	transferID := fmt.Sprintf("upload-%s-%s", sessionID, filepath.Base(localDir))
	a.transferCancelsMu.Lock()
	a.transferCancels[transferID] = cancel
	a.transferCancelsMu.Unlock()
	defer func() {
		a.transferCancelsMu.Lock()
		delete(a.transferCancels, transferID)
		a.transferCancelsMu.Unlock()
	}()

	progress := func(done, total int64) {
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
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
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
			ID: transferID, SessionID: sessionID, Direction: "upload",
			LocalPath: localDir, RemotePath: remoteDir,
			Done: 0, Total: 0, State: state,
		})
	}
	return err
}

// Download copies a remote file or directory to the local path (recursive for directories).
func (a *AppAPI) Download(sessionID, remotePath, localDir string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	ctx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	_, listErr := fs.List(ctx, remotePath)
	if listErr == nil {
		localTarget := filepath.Join(localDir, filepath.Base(remotePath))
		if err := os.MkdirAll(localTarget, 0755); err != nil {
			return err
		}
		return a.downloadRecursive(sessionID, remotePath, localTarget)
	}
	return a.downloadFile(sessionID, remotePath, localDir)
}

func (a *AppAPI) downloadRecursive(sessionID, remoteDir, localDir string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	parentCtx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	if err := a.acquireTransferSlot(parentCtx); err != nil {
		return err
	}
	defer a.releaseTransferSlot()
	ctx, cancel := context.WithCancel(parentCtx)
	transferID := fmt.Sprintf("download-%s-%s", sessionID, filepath.Base(remoteDir))
	a.transferCancelsMu.Lock()
	a.transferCancels[transferID] = cancel
	a.transferCancelsMu.Unlock()
	defer func() {
		a.transferCancelsMu.Lock()
		delete(a.transferCancels, transferID)
		a.transferCancelsMu.Unlock()
	}()

	progress := func(done, total int64) {
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
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
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
			ID: transferID, SessionID: sessionID, Direction: "download",
			LocalPath: localDir, RemotePath: remoteDir,
			Done: 0, Total: 0, State: state,
		})
	}
	return err
}

func (a *AppAPI) downloadFile(sessionID, remotePath, localDir string) error {
	fs, err := a.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return err
	}
	parentCtx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	if err := a.acquireTransferSlot(parentCtx); err != nil {
		return err
	}
	defer a.releaseTransferSlot()
	ctx, cancel := context.WithCancel(parentCtx)
	localPath := filepath.Join(localDir, filepath.Base(remotePath))
	transferID := fmt.Sprintf("download-%s-%s", sessionID, filepath.Base(remotePath))
	a.transferCancelsMu.Lock()
	a.transferCancels[transferID] = cancel
	a.transferCancelsMu.Unlock()
	defer func() {
		a.transferCancelsMu.Lock()
		delete(a.transferCancels, transferID)
		a.transferCancelsMu.Unlock()
	}()

	progress := func(done, total int64) {
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
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
	if a.ctx != nil {
		wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
			ID: transferID, SessionID: sessionID, Direction: "download",
			LocalPath: localPath, RemotePath: remotePath,
			Done: 0, Total: 0, State: state,
		})
	}
	return err
}

// CancelTransfer cancels an active transfer by ID.
func (a *AppAPI) CancelTransfer(transferID string) {
	a.transferCancelsMu.Lock()
	cancel, ok := a.transferCancels[transferID]
	delete(a.transferCancels, transferID)
	a.transferCancelsMu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
}
