package usecase

import (
	"context"
	"fmt"

	"xquakshell/internal/domain"
)

// ResolvedAction is the caller's decision for one conflicting target. NewName is
// an optional explicit rename target (from the dialog's editable name field);
// when empty a Rename outcome auto-numbers.
type ResolvedAction struct {
	Action  domain.ConflictAction
	NewName string
}

// ExecutePlan runs a resolved TransferPlan: it applies each file's conflict
// resolution and moves bytes through the kind-appropriate mover, reporting
// aggregated progress and honouring cancellation. It is the conflict-aware
// counterpart to Upload/Download.
func (s *TransferService) ExecutePlan(parentCtx context.Context, sessionID string, plan *TransferPlan, resolutions map[string]ResolvedAction, onProgress TransferProgressFunc) error {
	// Everything below the first fallible step inherits a live panel item: the
	// planner published one under plan.OpID and deliberately did not close it, so
	// the lifecycle invariant makes every exit path from here responsible for
	// emitting exactly one terminal event. Ownership of both the item and its
	// cancel registration is therefore taken over *before* anything can fail.
	//
	// Reuse the OpID assigned during planning so the scanning phase and this
	// byte-transfer phase are one continuous Transfers-panel item. Fall back to a
	// fresh id for plans built without one.
	transferID := plan.OpID
	if transferID == "" {
		transferID = newOpID(plan.Kind)
	}
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()
	// Replace, not Register: for a planned drop the planner already owns this id
	// and parked a "close the item" action under it. This phase takes over that
	// ownership with a real cancellation, unbroken — the id is never absent from
	// the registry while the panel item is active. Unregistering on the way out
	// also means an early return cannot leave the planner's parked closer behind
	// to emit a second terminal event for an operation that already ended.
	s.cancels.Replace(transferID, cancel)
	defer s.cancels.Unregister(transferID)

	// The batch's caption is a count ("3 items"), not a path, so it replaces the
	// destination directory in the label while RefreshDir keeps the real path.
	rep := newOperationReporter(transferID, sessionID, plan.Kind, plan.DestDir, onProgress).
		withLabel(planLabel(plan))
	// Structural safety net, not the normal path. The reporter's done latch makes
	// this a no-op once any terminal state has been reported, so it fires only on
	// a return that closed nothing itself — including one added here in future.
	// This is what `defer s.releaseSlot()` does for the limiter slot, applied to
	// the UI item.
	defer rep.Finish("failed")

	mover, err := s.moverFor(plan.Kind, sessionID)
	if err != nil {
		return err // closed as "failed" by the deferred net above
	}
	if err := s.acquireSlot(ctx); err != nil {
		// acquireSlot fails only on a cancelled context, so this is "Cancelled",
		// not "Error" — hence an explicit Finish rather than the net's "failed".
		rep.Finish(terminalState(ctx, err))
		return err
	}
	defer s.releaseSlot()

	return executePlanCore(ctx, plan, resolutions, mover, rep.Report)
}

func (s *TransferService) moverFor(kind, sessionID string) (fileMover, error) {
	switch kind {
	case transferKindUpload:
		fs, err := s.sessions.GetRemoteFS(sessionID)
		if err != nil {
			return nil, err
		}
		return &uploadMover{fs: fs}, nil
	case transferKindDownload:
		fs, err := s.sessions.GetRemoteFS(sessionID)
		if err != nil {
			return nil, err
		}
		if s.hostFS == nil {
			return nil, errHostFSUnavailable
		}
		return &downloadMover{fs: fs, hostFS: s.hostFS}, nil
	case transferKindLocalCopy:
		if s.hostFS == nil {
			return nil, errHostFSUnavailable
		}
		return &localCopyMover{hostFS: s.hostFS}, nil
	default:
		return nil, fmt.Errorf("unknown transfer kind %q", kind)
	}
}

// plannedJob is one file the executor will actually write, after conflict
// resolution has chosen its final target (possibly renamed) and whether the
// existing target must be removed first (type mismatch).
type plannedJob struct {
	source      string
	target      string
	size        int64
	removeFirst bool
}

// executePlanCore is the pure sequencing core: it turns a plan plus resolutions
// into concrete jobs, ensures directories, and moves each file, reporting
// aggregated byte progress. It has no session/limiter/cancel-registry
// dependencies so it can be tested against a fake mover.
func executePlanCore(ctx context.Context, plan *TransferPlan, resolutions map[string]ResolvedAction, mover fileMover, report func(done, total int64, state string)) error {
	ops := targetPathOps(plan.Kind)
	jobs, total := planJobs(ctx, plan, resolutions, mover, ops)

	// Ensure declared directories first so nested files have somewhere to land.
	for _, dir := range plan.Dirs {
		if err := mover.ensureDir(ctx, dir); err != nil {
			report(0, total, terminalState(ctx, err))
			return err
		}
	}

	report(0, total, "active")
	var done int64
	for _, j := range jobs {
		select {
		case <-ctx.Done():
			report(done, total, "cancelled")
			return ctx.Err()
		default:
		}
		parent, _ := ops.Split(j.target)
		if err := mover.ensureDir(ctx, parent); err != nil {
			report(done, total, terminalState(ctx, err))
			return err
		}
		if j.removeFirst {
			if err := mover.removeTarget(ctx, j.target); err != nil {
				report(done, total, terminalState(ctx, err))
				return err
			}
		}
		fileBase := done
		if err := mover.moveFile(ctx, j.source, j.target, func(fileDone, _ int64) {
			report(fileBase+fileDone, total, "active")
		}); err != nil {
			report(done, total, terminalState(ctx, err))
			return err
		}
		done += j.size
		report(done, total, "active")
	}
	report(total, total, "completed")
	return nil
}

// planJobs resolves every file's conflict outcome into a concrete job list and
// the total byte count to transfer. Skipped files are dropped; renamed files get
// a fresh, batch-unique target name.
func planJobs(ctx context.Context, plan *TransferPlan, resolutions map[string]ResolvedAction, mover fileMover, ops pathOps) ([]plannedJob, int64) {
	var jobs []plannedJob
	var total int64
	// Reserved names per parent directory prevent two renamed files colliding
	// within the same batch.
	reserved := map[string]map[string]bool{}

	for _, f := range plan.Files {
		action := domain.ConflictOverwrite
		var tgt domain.FileStat
		if f.HasConflict() {
			tgt = *f.Conflict
			if r, ok := resolutions[f.Target]; ok {
				action = r.Action
			} else {
				action = domain.ConflictSkip // unresolved conflict: never silently overwrite
			}
		}
		src := domain.FileStat{Exists: true, Size: f.Size, ModTime: f.SrcModTime}

		switch domain.ResolveConflict(action, src, tgt) {
		case domain.OutcomeSkip:
			continue
		case domain.OutcomeWrite:
			jobs = append(jobs, plannedJob{
				source: f.Source, target: f.Target, size: f.Size,
				removeFirst: f.HasConflict() && f.Conflict.IsDir,
			})
			total += f.Size
		case domain.OutcomeRename:
			parent, base := ops.Split(f.Target)
			used := reservedNames(ctx, reserved, mover, parent)
			desired := base
			if r, ok := resolutions[f.Target]; ok && r.NewName != "" {
				desired = r.NewName
			}
			unique := domain.NextAvailableName(desired, func(n string) bool { return used[n] })
			used[unique] = true
			jobs = append(jobs, plannedJob{source: f.Source, target: ops.Join(parent, unique), size: f.Size})
			total += f.Size
		}
	}
	return jobs, total
}

// reservedNames returns the mutable name set for a target directory, seeding it
// once from the directory's current contents.
func reservedNames(ctx context.Context, reserved map[string]map[string]bool, mover fileMover, dir string) map[string]bool {
	if names, ok := reserved[dir]; ok {
		return names
	}
	names := mover.existingNames(ctx, dir)
	if names == nil {
		names = map[string]bool{}
	}
	reserved[dir] = names
	return names
}

// planLabel is a short human label for the batch, shown in the transfers panel.
func planLabel(plan *TransferPlan) string {
	n := len(plan.Files)
	if n == 1 {
		return plan.Files[0].Target
	}
	return fmt.Sprintf("%d items", n)
}
