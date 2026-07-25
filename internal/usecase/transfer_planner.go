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
	cancels  *CancelRegistry
}

// NewTransferPlanner creates a planner over the session registry, the host FS
// and the application-wide cancel registry. The planner is the first owner of a
// drop's op id: it registers the id, and hands ownership on to the executor.
func NewTransferPlanner(sessions remoteOpSessionPort, hostFS domain.HostFileSystem, cancels *CancelRegistry) *TransferPlanner {
	if cancels == nil {
		panic("usecase: TransferPlanner requires CancelRegistry")
	}
	return &TransferPlanner{sessions: sessions, hostFS: hostFS, cancels: cancels}
}

// PlanUpload plans uploading local paths into a remote directory: source walked
// on the host FS, targets probed over SFTP. onProgress streams a live "scanning"
// counter during enumeration under the plan's OpID.
func (p *TransferPlanner) PlanUpload(sessionID string, localPaths []string, remoteDir string, onProgress TransferProgressFunc) (*TransferPlan, error) {
	fs, sessCtx, err := p.remote(sessionID)
	if err != nil {
		return nil, err
	}
	ctx, cancel, rep := p.beginScan(sessCtx, transferKindUpload, sessionID, remoteDir, onProgress)
	defer cancel()
	ports := planPorts{
		walkRoot:      func(root string) ([]sourceEntry, error) { return walkLocalSource(ctx, p.hostFS, root, rep.Scanned) },
		listTargetDir: func(dir string) map[string]domain.FileStat { return listRemoteTargetDir(ctx, fs, dir) },
		ops:           remotePathOps{},
	}
	plan, err := buildPlan(transferKindUpload, remoteDir, localPaths, ports)
	return finishPlan(ctx, plan, err, rep, p.cancels)
}

// PlanDownload plans downloading remote paths into a local directory: source
// walked over SFTP, targets probed on the host FS.
func (p *TransferPlanner) PlanDownload(sessionID string, remotePaths []string, localDir string, onProgress TransferProgressFunc) (*TransferPlan, error) {
	fs, sessCtx, err := p.remote(sessionID)
	if err != nil {
		return nil, err
	}
	ctx, cancel, rep := p.beginScan(sessCtx, transferKindDownload, sessionID, localDir, onProgress)
	defer cancel()
	ports := planPorts{
		walkRoot:      func(root string) ([]sourceEntry, error) { return walkRemoteSource(ctx, fs, root, rep.Scanned) },
		listTargetDir: func(dir string) map[string]domain.FileStat { return listLocalTargetDir(p.hostFS, dir) },
		ops:           localPathOps{},
	}
	plan, err := buildPlan(transferKindDownload, localDir, remotePaths, ports)
	return finishPlan(ctx, plan, err, rep, p.cancels)
}

// PlanLocalCopy plans copying local paths into a local directory (OS Explorer
// drop). Both sides are the host FS. It is not tied to a session, so the scan
// counter carries an empty session id (shown in every panel).
func (p *TransferPlanner) PlanLocalCopy(srcPaths []string, destDir string, onProgress TransferProgressFunc) (*TransferPlan, error) {
	if p.hostFS == nil {
		return nil, errHostFSUnavailable
	}
	// A local copy is not tied to a session, so the scan hangs off the process
	// context; only an explicit cancel stops it.
	ctx, cancel, rep := p.beginScan(context.Background(), transferKindLocalCopy, "", destDir, onProgress)
	defer cancel()
	ports := planPorts{
		walkRoot:      func(root string) ([]sourceEntry, error) { return walkLocalSource(ctx, p.hostFS, root, rep.Scanned) },
		listTargetDir: func(dir string) map[string]domain.FileStat { return listLocalTargetDir(p.hostFS, dir) },
		ops:           localPathOps{},
	}
	plan, err := buildPlan(transferKindLocalCopy, destDir, srcPaths, ports)
	return finishPlan(ctx, plan, err, rep, p.cancels)
}

// beginScan opens the enumeration phase of a drop: it mints the operation id,
// derives a cancellable context from the session's, and builds the reporter that
// streams an initial indeterminate "active" event followed by a throttled scan
// counter (Total=0).
//
// The cancel func is registered BEFORE the first event is emitted. The panel
// draws a cancel button the moment the item appears, and from that instant the
// button must do something — scanning a large tree over SFTP takes minutes.
//
// NOTE: there is deliberately no `defer p.cancels.Unregister(opID)` at any call
// site. That is not a forgotten line: on the success path the registration is
// handed to the next phase instead of being dropped, because the item stays
// visible and active while the user resolves conflicts. All four exit paths
// (branches 1, 1b, 2 and 3) are spelled out in finishPlan below — do not "fix"
// this by adding a defer.
func (p *TransferPlanner) beginScan(parent context.Context, kind, sessionID, targetDir string, onProgress TransferProgressFunc) (context.Context, context.CancelFunc, *operationReporter) {
	opID := newOpID(kind)
	rep := newOperationReporter(opID, sessionID, kind, targetDir, onProgress)
	ctx, cancel := context.WithCancel(parent)
	p.cancels.Register(opID, cancel)
	rep.Started()
	return ctx, cancel, rep
}

// finishPlan closes the enumeration phase. Cancellability is an invariant, not a
// courtesy: while the panel shows the item as active, the registry must hold a
// way to close it. So the registration is released only together with the
// terminal event, and handed on when there is no terminal event yet.
func finishPlan(ctx context.Context, plan *TransferPlan, err error, rep *operationReporter, cancels *CancelRegistry) (*TransferPlan, error) {
	opID := rep.opID
	// Branch 1: enumeration broke off — cancelled by the user or genuinely
	// failed. Either way the operation ends here.
	if err != nil {
		rep.Finish(terminalState(ctx, err))
		cancels.Unregister(opID)
		return nil, err
	}
	// Branch 1b: the walk finished, but the user cancelled before we got here.
	// The conflict probes that follow the walk have no cancellation point of
	// their own — a failed probe just means "no conflicts" — so without this
	// check a cancel landing during the probe phase would be swallowed and the
	// item re-registered as if nothing happened.
	if cerr := ctx.Err(); cerr != nil {
		rep.Finish(terminalState(ctx, cerr))
		cancels.Unregister(opID)
		return nil, cerr
	}
	// Branch 2: nothing to transfer. The operation happened and ends here; the
	// executor is never called, so the op id is not stamped onto the plan.
	if len(plan.Files) == 0 {
		rep.Finish("completed")
		cancels.Unregister(opID)
		return plan, nil
	}
	// Branch 3: the plan awaits execution. No work is running — the user may be
	// staring at the conflict dialog for minutes — but the item is active, so it
	// must stay cancellable, and here "cancel" means "close the item".
	//
	// This closer emits the id's one terminal event, and it does so while nothing
	// downstream is watching: the frontend is blocked on the conflict dialog and
	// will call ExecutePlan regardless of what happened out here. So the fact that
	// it ran has to survive in something that outlives this reporter — the
	// registry records it (Cancel -> terminated), and ExecutePlan asks with
	// takeOver instead of assuming the id is still live. A second reporter with a
	// second done latch cannot enforce one-terminal-per-id on its own.
	//
	// NOTE: there is a narrow, accepted race here. A cancel click landing between
	// the ctx.Err() check above and this Replace call finds the entry already
	// removed (branch 1b's check happened, and produced no cancellation because
	// ctx.Err() was still nil): the click is a silent no-op, and Replace then
	// re-registers the id as if nothing happened, leaving the conflict dialog to
	// open normally. A second click works. The window is a few instructions
	// wide; do not close it with extra locking — that would introduce the very
	// ordering dependency this handoff design avoids.
	cancels.Replace(opID, func() { rep.Finish("cancelled") })
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
