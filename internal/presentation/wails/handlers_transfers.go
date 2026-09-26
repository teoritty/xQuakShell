package wails

import (
	"context"
	"errors"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"xquakshell/internal/usecase"
)

func (a *AppAPI) emitTransferProgress(p usecase.TransferProgress) {
	if a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventTransferProgress, TransferProgressPayload{
		ID:         p.ID,
		SessionID:  p.SessionID,
		Kind:       p.Kind,
		LocalPath:  p.LocalPath,
		RemotePath: p.RemotePath,
		RefreshDir: p.RefreshDir,
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
	return quietCancel(a.transferSvc.Upload(parentCtx, sessionID, localPath, remotePath, a.emitTransferProgress))
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
	return quietCancel(a.transferSvc.Download(parentCtx, sessionID, remotePath, localDir, a.emitTransferProgress))
}

// PlanUpload enumerates uploading localPaths into remoteDir, detecting conflicts
// at the destination without transferring anything.
func (a *AppAPI) PlanUpload(sessionID string, localPaths []string, remoteDir string) (TransferPlanDTO, error) {
	if a.transferPlanner == nil {
		return TransferPlanDTO{}, nil
	}
	plan, err := a.transferPlanner.PlanUpload(sessionID, localPaths, remoteDir, a.emitTransferProgress)
	if err != nil {
		return TransferPlanDTO{}, quietCancel(err)
	}
	return transferPlanToDTO(plan), nil
}

// PlanDownload enumerates downloading remotePaths into localDir, detecting
// conflicts at the destination.
func (a *AppAPI) PlanDownload(sessionID string, remotePaths []string, localDir string) (TransferPlanDTO, error) {
	if a.transferPlanner == nil {
		return TransferPlanDTO{}, nil
	}
	plan, err := a.transferPlanner.PlanDownload(sessionID, remotePaths, localDir, a.emitTransferProgress)
	if err != nil {
		return TransferPlanDTO{}, quietCancel(err)
	}
	return transferPlanToDTO(plan), nil
}

// PlanLocalCopy enumerates copying srcPaths into destDir on the local
// filesystem (OS Explorer drop), detecting conflicts.
func (a *AppAPI) PlanLocalCopy(srcPaths []string, destDir string) (TransferPlanDTO, error) {
	if a.transferPlanner == nil {
		return TransferPlanDTO{}, nil
	}
	plan, err := a.transferPlanner.PlanLocalCopy(srcPaths, destDir, a.emitTransferProgress)
	if err != nil {
		return TransferPlanDTO{}, quietCancel(err)
	}
	return transferPlanToDTO(plan), nil
}

// quietCancel absorbs the outcomes that are not failures: the user cancelled the
// panel item, either while the conflict dialog was open (ErrOperationCancelled -
// nothing ran) or while the scan or the transfer itself was running
// (ErrTransferCancelled). Either way the item already shows "cancelled". The
// frontend turns every rejected RPC into an error banner, and "you cancelled
// this, here is an error" is the wrong thing to tell someone who cancelled.
// Every other error still propagates.
func quietCancel(err error) error {
	if errors.Is(err, usecase.ErrOperationCancelled) || errors.Is(err, usecase.ErrTransferCancelled) {
		return nil
	}
	return err
}

// execPlan runs a resolved plan; a cancellation is not reported as an error.
func (a *AppAPI) execPlan(parentCtx context.Context, sessionID string, req ExecutePlanDTO) error {
	return quietCancel(a.transferSvc.ExecutePlan(parentCtx, sessionID, dtoToTransferPlan(req.Plan), dtoToResolutions(req.Resolutions), a.emitTransferProgress))
}

// ExecuteUpload runs a resolved upload plan.
func (a *AppAPI) ExecuteUpload(sessionID string, req ExecutePlanDTO) error {
	if a.transferSvc == nil {
		return nil
	}
	parentCtx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	return a.execPlan(parentCtx, sessionID, req)
}

// ExecuteDownload runs a resolved download plan.
func (a *AppAPI) ExecuteDownload(sessionID string, req ExecutePlanDTO) error {
	if a.transferSvc == nil {
		return nil
	}
	parentCtx, err := a.sessions.GetSessionContext(sessionID)
	if err != nil {
		return err
	}
	return a.execPlan(parentCtx, sessionID, req)
}

// ExecuteLocalCopy runs a resolved local-copy plan (OS Explorer drop). It is not
// tied to a session, so it uses a background context.
func (a *AppAPI) ExecuteLocalCopy(req ExecutePlanDTO) error {
	if a.transferSvc == nil {
		return nil
	}
	return a.execPlan(a.reqCtx(), "", req)
}

// CancelTransfer cancels an active transfer, a scanning drop or a remote
// operation by ID. Every phase of every cancellable operation registers in the
// one shared registry, so a single lookup covers them all.
func (a *AppAPI) CancelTransfer(operationID string) {
	if a.cancels != nil {
		a.cancels.Cancel(operationID)
	}
}
