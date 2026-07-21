package usecase

import (
	"context"

	"xquakshell/internal/domain"
)

// TransferPlanner enumerates a drag-and-drop into a TransferPlan, detecting
// destination conflicts, without moving any bytes. It is the read-only sibling of
// TransferService: the planner decides *what* would happen, the executor does it.
type TransferPlanner struct {
	sessions remoteOpSessionPort
	hostFS   domain.HostFileSystem
}

// NewTransferPlanner creates a planner over the session registry and host FS.
func NewTransferPlanner(sessions remoteOpSessionPort, hostFS domain.HostFileSystem) *TransferPlanner {
	return &TransferPlanner{sessions: sessions, hostFS: hostFS}
}

// PlanUpload plans uploading local paths into a remote directory: source walked
// on the host FS, targets probed over SFTP.
func (p *TransferPlanner) PlanUpload(sessionID string, localPaths []string, remoteDir string) (*TransferPlan, error) {
	fs, ctx, err := p.remote(sessionID)
	if err != nil {
		return nil, err
	}
	ports := planPorts{
		walkRoot:      func(root string) ([]sourceEntry, error) { return walkLocalSource(p.hostFS, root) },
		listTargetDir: func(dir string) map[string]domain.FileStat { return listRemoteTargetDir(ctx, fs, dir) },
		ops:           remotePathOps{},
	}
	return buildPlan(transferKindUpload, remoteDir, localPaths, ports)
}

// PlanDownload plans downloading remote paths into a local directory: source
// walked over SFTP, targets probed on the host FS.
func (p *TransferPlanner) PlanDownload(sessionID string, remotePaths []string, localDir string) (*TransferPlan, error) {
	fs, ctx, err := p.remote(sessionID)
	if err != nil {
		return nil, err
	}
	ports := planPorts{
		walkRoot:      func(root string) ([]sourceEntry, error) { return walkRemoteSource(ctx, fs, root) },
		listTargetDir: func(dir string) map[string]domain.FileStat { return listLocalTargetDir(p.hostFS, dir) },
		ops:           localPathOps{},
	}
	return buildPlan(transferKindDownload, localDir, remotePaths, ports)
}

// PlanLocalCopy plans copying local paths into a local directory (OS Explorer
// drop). Both sides are the host FS.
func (p *TransferPlanner) PlanLocalCopy(srcPaths []string, destDir string) (*TransferPlan, error) {
	if p.hostFS == nil {
		return nil, errHostFSUnavailable
	}
	ports := planPorts{
		walkRoot:      func(root string) ([]sourceEntry, error) { return walkLocalSource(p.hostFS, root) },
		listTargetDir: func(dir string) map[string]domain.FileStat { return listLocalTargetDir(p.hostFS, dir) },
		ops:           localPathOps{},
	}
	return buildPlan(transferKindLocalCopy, destDir, srcPaths, ports)
}

func (p *TransferPlanner) remote(sessionID string) (domain.RemoteFS, context.Context, error) {
	fs, err := p.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return nil, nil, err
	}
	ctx, err := p.sessions.GetSessionContext(sessionID)
	if err != nil {
		return nil, nil, err
	}
	return fs, ctx, nil
}
