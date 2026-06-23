package wails

import (
	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"ssh-client/internal/usecase"
)

func (a *AppAPI) emitTransferProgress(p usecase.TransferProgress) {
	if a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
		ID:         p.ID,
		SessionID:  p.SessionID,
		Direction:  p.Direction,
		LocalPath:  p.LocalPath,
		RemotePath: p.RemotePath,
		Done:       p.Done,
		Total:      p.Total,
		State:      p.State,
	})
}

// Upload copies a local file or directory to the remote path (recursive for directories).
func (a *AppAPI) Upload(sessionID, localPath, remotePath string) error {
	if a.transferSvc == nil {
		return nil
	}
	parentCtx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	return a.transferSvc.Upload(parentCtx, sessionID, localPath, remotePath, a.emitTransferProgress)
}

// Download copies a remote file or directory to the local path (recursive for directories).
func (a *AppAPI) Download(sessionID, remotePath, localDir string) error {
	if a.transferSvc == nil {
		return nil
	}
	parentCtx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	return a.transferSvc.Download(parentCtx, sessionID, remotePath, localDir, a.emitTransferProgress)
}

// CancelTransfer cancels an active transfer by ID.
func (a *AppAPI) CancelTransfer(transferID string) {
	if a.transferSvc != nil {
		a.transferSvc.Cancel(transferID)
	}
}
