package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain"
)

// The lifecycle invariant under test: ExecutePlan takes over a panel item the
// planner already published under plan.OpID, so *every* exit path — including
// the ones that never reach the transfer loop — must emit exactly one terminal
// event. Without that, the item reads "Scanning…" until the app restarts.

// execEvents collects ExecutePlan's progress stream. ExecutePlan may emit from
// the mover's progress callback, so the sink locks.
type execEvents struct {
	mu     sync.Mutex
	events []TransferProgress
}

func (e *execEvents) fn(p TransferProgress) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, p)
}

func (e *execEvents) terminals() []TransferProgress {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []TransferProgress
	for _, ev := range e.events {
		if isTerminalState(ev.State) {
			out = append(out, ev)
		}
	}
	return out
}

// execService builds a TransferService with no session registry: the paths
// exercised here either fail before touching it (unknown kind) or use the
// host-FS-only local-copy mover.
func execService(hostFS domain.HostFileSystem, cancels *CancelRegistry) *TransferService {
	return NewTransferService(nil, nil, hostFS, newStubConcurrencyLimiter(1), cancels)
}

func plannedPlan(kind, opID string) *TransferPlan {
	return &TransferPlan{
		Kind:    kind,
		OpID:    opID,
		DestDir: "/dst",
		Files: []PlannedFile{
			{Source: "/src/a.txt", Target: "/dst/a.txt", Size: 7, SrcModTime: time.Unix(100, 0)},
		},
	}
}

// Path 1: the mover cannot be built — the session dropped while the user sat in
// the conflict dialog, or the plan carries a kind nothing can move. The item is
// already on screen, so this must close it as failed rather than return silently.
func TestExecutePlanUnbuildableMoverClosesItemAsFailed(t *testing.T) {
	cancels := NewCancelRegistry()
	svc := execService(&mockHostFS{}, cancels)
	sink := &execEvents{}

	err := svc.ExecutePlan(context.Background(), "s1", plannedPlan("bogus-kind", "op-1"), nil, sink.fn)
	if err == nil {
		t.Fatal("want an error for a plan whose kind has no mover")
	}

	terminals := sink.terminals()
	if len(terminals) != 1 {
		t.Fatalf("terminal events = %d, want exactly 1: %+v", len(terminals), terminals)
	}
	if terminals[0].State != "failed" {
		t.Fatalf("terminal state = %q, want failed", terminals[0].State)
	}
	if terminals[0].ID != "op-1" {
		t.Fatalf("terminal event id = %q, want the plan's op id: a different id closes a different item", terminals[0].ID)
	}
}

// Path 2: the slot cannot be acquired. acquireSlot only fails on a cancelled
// context, so the user must see "Cancelled", not "Error".
func TestExecutePlanCancelledBeforeSlotClosesItemAsCancelled(t *testing.T) {
	cancels := NewCancelRegistry()
	svc := execService(&mockHostFS{}, cancels)
	sink := &execEvents{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.ExecutePlan(ctx, "", plannedPlan(transferKindLocalCopy, "op-2"), nil, sink.fn)
	if err == nil {
		t.Fatal("want an error when the context is already cancelled")
	}

	terminals := sink.terminals()
	if len(terminals) != 1 {
		t.Fatalf("terminal events = %d, want exactly 1: %+v", len(terminals), terminals)
	}
	if terminals[0].State != "cancelled" {
		t.Fatalf("terminal state = %q, want cancelled: a cancelled context is not an error", terminals[0].State)
	}
}

// The regression the done latch exists to prevent: the deferred safety net must
// not repaint a finished transfer as failed. If it did, the main path would
// break — worse than the bug being fixed.
func TestExecutePlanSuccessStaysCompletedUnderSafetyNet(t *testing.T) {
	cancels := NewCancelRegistry()
	svc := execService(&mockHostFS{}, cancels)
	sink := &execEvents{}

	if err := svc.ExecutePlan(context.Background(), "", plannedPlan(transferKindLocalCopy, "op-3"), nil, sink.fn); err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	terminals := sink.terminals()
	if len(terminals) != 1 {
		t.Fatalf("terminal events = %d, want exactly 1: %+v", len(terminals), terminals)
	}
	if terminals[0].State != "completed" {
		t.Fatalf("terminal state = %q, want completed: the deferred net overwrote a successful transfer", terminals[0].State)
	}
	if terminals[0].Done != 7 || terminals[0].Total != 7 {
		t.Fatalf("terminal figures = %d/%d, want 7/7", terminals[0].Done, terminals[0].Total)
	}
}

// Closing the item is only half of it: the planner parked a "close the item"
// action under this id, and an early return must not leave that action behind.
// A stale entry means a later cancel click emits a second terminal event for an
// operation that already ended.
func TestExecutePlanEarlyReturnReleasesCancelRegistration(t *testing.T) {
	cancels := NewCancelRegistry()
	svc := execService(&mockHostFS{}, cancels)
	sink := &execEvents{}

	// Stand in for the planner's parked closer.
	plannerClosed := false
	cancels.Register("op-4", func() { plannerClosed = true })

	if err := svc.ExecutePlan(context.Background(), "s1", plannedPlan("bogus-kind", "op-4"), nil, sink.fn); err == nil {
		t.Fatal("want an error")
	}
	if cancels.Cancel("op-4") {
		t.Fatal("op id still cancellable after a closed operation: the planner's closer leaked")
	}
	if plannerClosed {
		t.Fatal("the planner's closer ran: ownership was not taken over before the first fallible step")
	}
}

// copyCountingHostFS is a host FS that counts the file copies a local-copy
// transfer actually performs. "Did any bytes move?" is the assertion that
// matters below; a terminal-event count alone would not catch a transfer that
// wrote every file and merely mislabelled the result.
type copyCountingHostFS struct {
	*mockHostFS
	copies int
}

func (c *copyCountingHostFS) CopyTo(_, _ string) error {
	c.copies++
	return nil
}

// The whole point of the phase handoff: a cancel landing in the conflict-dialog
// gap must actually stop the transfer, not just be repainted over.
//
// The planner parks a "close the item" closer under the op id and returns. The
// user clicks the panel item's ✕ — the closer runs, emitting the id's one
// terminal event — but nothing interrupts the conflict dialog, so the frontend
// resolves it and calls ExecutePlan as if nothing happened. ExecutePlan builds
// its own reporter, with its own done latch, which knows nothing about the
// terminal already emitted: before the fix it re-registered the id, copied every
// file (overwriting the conflicting target the user was asked about) and emitted
// "completed", flipping the panel row cancelled -> active -> completed. The
// cancellation was not double-reported, it was discarded.
func TestExecutePlanRefusesAnIDCancelledDuringTheConflictDialog(t *testing.T) {
	hostFS := &copyCountingHostFS{mockHostFS: &mockHostFS{
		statFn: func(string) (domain.HostFileInfo, error) {
			return domain.HostFileInfo{IsDir: false, Size: 7}, nil
		},
		// The destination already holds a.txt, so the plan carries a conflict and
		// the dialog is what the user is looking at when they hit cancel.
		listFn: func(string, bool, func(string, string) bool) ([]domain.LocalFileEntry, error) {
			return []domain.LocalFileEntry{{Name: "a.txt", Path: "/dst/a.txt", Size: 3}}, nil
		},
	}}
	cancels := NewCancelRegistry()

	// One sink across both phases: the invariant is one terminal event per op id,
	// not one per phase.
	sink := &execEvents{}

	plan, err := NewTransferPlanner(nil, hostFS, cancels).PlanLocalCopy([]string{"/src/a.txt"}, "/dst", sink.fn)
	if err != nil {
		t.Fatalf("PlanLocalCopy: %v", err)
	}
	if plan.OpID == "" {
		t.Fatal("a plan awaiting execution must carry its op id")
	}
	if len(plan.Files) != 1 || !plan.Files[0].HasConflict() {
		t.Fatalf("want one conflicting file in the plan, got %+v", plan.Files)
	}

	// The user clicks the panel item's cancel while the dialog is open.
	if !cancels.Cancel(plan.OpID) {
		t.Fatal("the parked op id was not cancellable: the T5 handoff regressed")
	}

	// The dialog completes anyway and the frontend executes the resolved plan.
	err = execService(hostFS, cancels).ExecutePlan(
		context.Background(), "", plan,
		// Keyed by the plan's own target so the overwrite really is chosen: a
		// mismatched key would silently degrade to Skip and copy nothing anyway,
		// making the "moved nothing" assertion vacuous.
		map[string]ResolvedAction{plan.Files[0].Target: {Action: domain.ConflictOverwrite}},
		sink.fn,
	)
	if !errors.Is(err, ErrOperationCancelled) {
		t.Fatalf("err = %v, want ErrOperationCancelled: a cancel is not a failure to surface", err)
	}

	if hostFS.copies != 0 {
		t.Fatalf("copies = %d, want 0: files were written after the user asked to stop", hostFS.copies)
	}
	terminals := sink.terminals()
	if len(terminals) != 1 {
		t.Fatalf("terminal events = %d, want exactly 1 for the op id: %+v", len(terminals), terminals)
	}
	if terminals[0].State != "cancelled" {
		t.Fatalf("terminal state = %q, want cancelled", terminals[0].State)
	}
	if cancels.Cancel(plan.OpID) {
		t.Fatal("declining the id must not leave it registered: the row would be cancellable after it closed")
	}
}

// The other half of the same handoff: a plan the user did NOT cancel must be
// taken over exactly as before, and must stay cancellable through execution.
// Without this, "refuse the cancelled id" could be implemented by refusing
// everything.
func TestExecutePlanStillTakesOverAnUncancelledPlannedID(t *testing.T) {
	hostFS := &copyCountingHostFS{mockHostFS: &mockHostFS{
		statFn: func(string) (domain.HostFileInfo, error) {
			return domain.HostFileInfo{IsDir: false, Size: 7}, nil
		},
	}}
	cancels := NewCancelRegistry()
	sink := &execEvents{}

	plan, err := NewTransferPlanner(nil, hostFS, cancels).PlanLocalCopy([]string{"/src/a.txt"}, "/dst", sink.fn)
	if err != nil {
		t.Fatalf("PlanLocalCopy: %v", err)
	}
	if err := execService(hostFS, cancels).ExecutePlan(context.Background(), "", plan, nil, sink.fn); err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}
	if hostFS.copies != 1 {
		t.Fatalf("copies = %d, want 1: an uncancelled plan must still transfer", hostFS.copies)
	}
	terminals := sink.terminals()
	if len(terminals) != 1 || terminals[0].State != "completed" {
		t.Fatalf("terminals = %+v, want exactly one completed event", terminals)
	}
}
