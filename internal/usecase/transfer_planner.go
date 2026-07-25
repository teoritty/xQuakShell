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
// on the host FS, targets probed over SFTP. onProgress streams a live "scanning"
// counter during enumeration under the plan's OpID.
func (p *TransferPlanner) PlanUpload(sessionID string, localPaths []string, remoteDir string, onProgress TransferProgressFunc) (*TransferPlan, error) {
	fs, ctx, err := p.remote(sessionID)
	if err != nil {
		return nil, err
	}
	rep := newScanReporter(transferKindUpload, sessionID, remoteDir, onProgress)
	rep.Started()
	ports := planPorts{
		walkRoot:      func(root string) ([]sourceEntry, error) { return walkLocalSource(p.hostFS, root, rep.Scanned) },
		listTargetDir: func(dir string) map[string]domain.FileStat { return listRemoteTargetDir(ctx, fs, dir) },
		ops:           remotePathOps{},
	}
	plan, err := buildPlan(transferKindUpload, remoteDir, localPaths, ports)
	return finishPlan(plan, err, rep)
}

// PlanDownload plans downloading remote paths into a local directory: source
// walked over SFTP, targets probed on the host FS.
func (p *TransferPlanner) PlanDownload(sessionID string, remotePaths []string, localDir string, onProgress TransferProgressFunc) (*TransferPlan, error) {
	fs, ctx, err := p.remote(sessionID)
	if err != nil {
		return nil, err
	}
	rep := newScanReporter(transferKindDownload, sessionID, localDir, onProgress)
	rep.Started()
	ports := planPorts{
		walkRoot:      func(root string) ([]sourceEntry, error) { return walkRemoteSource(ctx, fs, root, rep.Scanned) },
		listTargetDir: func(dir string) map[string]domain.FileStat { return listLocalTargetDir(p.hostFS, dir) },
		ops:           localPathOps{},
	}
	plan, err := buildPlan(transferKindDownload, localDir, remotePaths, ports)
	return finishPlan(plan, err, rep)
}

// PlanLocalCopy plans copying local paths into a local directory (OS Explorer
// drop). Both sides are the host FS. It is not tied to a session, so the scan
// counter carries an empty session id (shown in every panel).
func (p *TransferPlanner) PlanLocalCopy(srcPaths []string, destDir string, onProgress TransferProgressFunc) (*TransferPlan, error) {
	if p.hostFS == nil {
		return nil, errHostFSUnavailable
	}
	rep := newScanReporter(transferKindLocalCopy, "", destDir, onProgress)
	rep.Started()
	ports := planPorts{
		walkRoot:      func(root string) ([]sourceEntry, error) { return walkLocalSource(p.hostFS, root, rep.Scanned) },
		listTargetDir: func(dir string) map[string]domain.FileStat { return listLocalTargetDir(p.hostFS, dir) },
		ops:           localPathOps{},
	}
	plan, err := buildPlan(transferKindLocalCopy, destDir, srcPaths, ports)
	return finishPlan(plan, err, rep)
}

// newScanReporter mints the operation id for a drop and builds the reporter
// that streams the enumeration phase: an initial indeterminate "active" event
// from Started, then a throttled scan counter (Total=0) from Scanned.
func newScanReporter(kind, sessionID, targetDir string, onProgress TransferProgressFunc) *operationReporter {
	return newOperationReporter(newOpID(kind), sessionID, kind, targetDir, onProgress)
}

// finishPlan stamps the OpID on a successful plan, or tears down the scanning
// item with a terminal "failed" event when enumeration errored. A successful
// plan deliberately emits no terminal event: the executor reuses the same op id
// and closes the panel item when the bytes are done.
func finishPlan(plan *TransferPlan, err error, rep *operationReporter) (*TransferPlan, error) {
	if err != nil {
		rep.Report(0, 0, "failed")
		return nil, err
	}
	plan.OpID = rep.opID
	return plan, nil
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
