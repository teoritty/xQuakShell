package usecase

import (
	"context"
	"fmt"
	"time"

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
	opID := newOpID(transferKindUpload, sessionID)
	onScan, emit := newScanReporter(opID, sessionID, transferKindUpload, remoteDir, onProgress)
	emit(0, "active")
	ports := planPorts{
		walkRoot:      func(root string) ([]sourceEntry, error) { return walkLocalSource(p.hostFS, root, onScan) },
		listTargetDir: func(dir string) map[string]domain.FileStat { return listRemoteTargetDir(ctx, fs, dir) },
		ops:           remotePathOps{},
	}
	plan, err := buildPlan(transferKindUpload, remoteDir, localPaths, ports)
	return finishPlan(plan, err, opID, emit)
}

// PlanDownload plans downloading remote paths into a local directory: source
// walked over SFTP, targets probed on the host FS.
func (p *TransferPlanner) PlanDownload(sessionID string, remotePaths []string, localDir string, onProgress TransferProgressFunc) (*TransferPlan, error) {
	fs, ctx, err := p.remote(sessionID)
	if err != nil {
		return nil, err
	}
	opID := newOpID(transferKindDownload, sessionID)
	onScan, emit := newScanReporter(opID, sessionID, transferKindDownload, localDir, onProgress)
	emit(0, "active")
	ports := planPorts{
		walkRoot:      func(root string) ([]sourceEntry, error) { return walkRemoteSource(ctx, fs, root, onScan) },
		listTargetDir: func(dir string) map[string]domain.FileStat { return listLocalTargetDir(p.hostFS, dir) },
		ops:           localPathOps{},
	}
	plan, err := buildPlan(transferKindDownload, localDir, remotePaths, ports)
	return finishPlan(plan, err, opID, emit)
}

// PlanLocalCopy plans copying local paths into a local directory (OS Explorer
// drop). Both sides are the host FS. It is not tied to a session, so the scan
// counter carries an empty session id (shown in every panel).
func (p *TransferPlanner) PlanLocalCopy(srcPaths []string, destDir string, onProgress TransferProgressFunc) (*TransferPlan, error) {
	if p.hostFS == nil {
		return nil, errHostFSUnavailable
	}
	opID := newOpID(transferKindLocalCopy, "")
	onScan, emit := newScanReporter(opID, "", transferKindLocalCopy, destDir, onProgress)
	emit(0, "active")
	ports := planPorts{
		walkRoot:      func(root string) ([]sourceEntry, error) { return walkLocalSource(p.hostFS, root, onScan) },
		listTargetDir: func(dir string) map[string]domain.FileStat { return listLocalTargetDir(p.hostFS, dir) },
		ops:           localPathOps{},
	}
	plan, err := buildPlan(transferKindLocalCopy, destDir, srcPaths, ports)
	return finishPlan(plan, err, opID, emit)
}

// newOpID mints a unique operation identifier for a transfer, shared by the
// planner's scan phase and the executor's byte phase.
func newOpID(kind, sessionID string) string {
	return fmt.Sprintf("%s-%s-%d", kind, sessionID, time.Now().UnixNano())
}

// newScanReporter builds the tick and emit closures for the enumeration phase:
// onScan streams a throttled, indeterminate "scanning" counter (Total=0), and
// emit lets the caller send the initial "active" event and a terminal state.
func newScanReporter(opID, sessionID, kind, targetDir string, onProgress TransferProgressFunc) (onScan func(), emit func(done int64, state string)) {
	displayKind := batchDisplayKind(kind)
	emit = func(done int64, state string) {
		if onProgress == nil {
			return
		}
		onProgress(TransferProgress{
			ID: opID, SessionID: sessionID,
			Kind: displayKind, Direction: displayKind,
			RemotePath: targetDir, RefreshDir: targetDir,
			Done: done, Total: 0, State: state,
		})
	}
	var scanned int64
	th := newThrottler()
	onScan = func() {
		scanned++
		if th.ready(scanned) {
			emit(scanned, "active")
		}
	}
	return
}

// finishPlan stamps the OpID on a successful plan, or tears down the scanning
// item with a terminal "failed" event when enumeration errored.
func finishPlan(plan *TransferPlan, err error, opID string, emit func(done int64, state string)) (*TransferPlan, error) {
	if err != nil {
		emit(0, "failed")
		return nil, err
	}
	plan.OpID = opID
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
