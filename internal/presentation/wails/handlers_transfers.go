package wails

import (
	"context"

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
		Direction:  p.Direction,
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

// PlanUpload enumerates uploading localPaths into remoteDir, detecting conflicts
// at the destination without transferring anything.
func (a *AppAPI) PlanUpload(sessionID string, localPaths []string, remoteDir string) (TransferPlanDTO, error) {
	if a.transferPlanner == nil {
		return TransferPlanDTO{}, nil
	}
	plan, err := a.transferPlanner.PlanUpload(sessionID, localPaths, remoteDir, a.emitTransferProgress)
	if err != nil {
		return TransferPlanDTO{}, err
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
		return TransferPlanDTO{}, err
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
		return TransferPlanDTO{}, err
	}
	return transferPlanToDTO(plan), nil
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
	return a.transferSvc.ExecutePlan(parentCtx, sessionID, dtoToTransferPlan(req.Plan), dtoToResolutions(req.Resolutions), a.emitTransferProgress)
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
	return a.transferSvc.ExecutePlan(parentCtx, sessionID, dtoToTransferPlan(req.Plan), dtoToResolutions(req.Resolutions), a.emitTransferProgress)
}

// ExecuteLocalCopy runs a resolved local-copy plan (OS Explorer drop). It is not
// tied to a session, so it uses a background context.
func (a *AppAPI) ExecuteLocalCopy(req ExecutePlanDTO) error {
	if a.transferSvc == nil {
		return nil
	}
	return a.transferSvc.ExecutePlan(context.Background(), "", dtoToTransferPlan(req.Plan), dtoToResolutions(req.Resolutions), a.emitTransferProgress)
}

// CancelTransfer cancels an active transfer, a scanning drop or a remote
// operation by ID. Every phase of every cancellable operation registers in the
// one shared registry, so a single lookup covers them all.
func (a *AppAPI) CancelTransfer(operationID string) {
	if a.cancels != nil {
		a.cancels.Cancel(operationID)
	}
}
