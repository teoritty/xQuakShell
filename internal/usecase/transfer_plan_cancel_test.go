package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"xquakshell/internal/domain"
)

// planEvents records the progress stream of one planning call. The planner runs
// synchronously on the calling goroutine, so no locking is needed here.
type planEvents struct {
	events []TransferProgress
	// onEvent fires for every emitted event, letting a test cancel *while* the
	// walk is running — exactly how the panel's cancel button behaves.
	onEvent func(TransferProgress)
}

func (p *planEvents) fn(ev TransferProgress) {
	p.events = append(p.events, ev)
	if p.onEvent != nil {
		p.onEvent(ev)
	}
}

func (p *planEvents) terminals() []TransferProgress {
	var out []TransferProgress
	for _, e := range p.events {
		if isTerminalState(e.State) {
			out = append(out, e)
		}
	}
	return out
}

// opID returns the operation id every event of a planning call carries.
func (p *planEvents) opID(t *testing.T) string {
	t.Helper()
	if len(p.events) == 0 {
		t.Fatal("no progress events emitted: the scan item never appeared")
	}
	return p.events[0].ID
}

// wideLocalTree models a source tree that is expensive to walk: every directory
// holds `width` files plus one subdirectory, nested `depth` levels deep. It
// counts List calls so a test can prove the walk stopped descending.
type wideLocalTree struct {
	width     int
	depth     int
	listCalls int
}

func (w *wideLocalTree) hostFS() *mockHostFS {
	return &mockHostFS{
		statFn: func(string) (domain.HostFileInfo, error) {
			return domain.HostFileInfo{IsDir: true}, nil
		},
		listFn: func(dir string, _ bool, _ func(string, string) bool) ([]domain.LocalFileEntry, error) {
			w.listCalls++
			out := make([]domain.LocalFileEntry, 0, w.width+1)
			for i := 0; i < w.width; i++ {
				name := fmt.Sprintf("f%d.txt", i)
				out = append(out, domain.LocalFileEntry{Name: name, Path: dir + "/" + name, Size: 1})
			}
			if w.listCalls < w.depth {
				out = append(out, domain.LocalFileEntry{Name: "deep", Path: dir + "/deep", IsDir: true})
			}
			return out, nil
		},
	}
}

// Cancelling while the source tree is being enumerated must actually abort the
// walk. Before the planner registered its op id (and before the walk checked the
// context), the panel's cancel button was a silent no-op: the enumeration ran to
// completion and the item kept spinning.
func TestPlanScanCancelAbortsLocalWalk(t *testing.T) {
	tree := &wideLocalTree{width: 200, depth: 50}
	cancels := NewCancelRegistry()
	p := NewTransferPlanner(nil, tree.hostFS(), cancels)

	sink := &planEvents{}
	sink.onEvent = func(ev TransferProgress) {
		// Cancel from inside the scan, once enumeration is well underway and the
		// walk is inside the first directory's entry loop.
		if ev.Done >= 64 && !isTerminalState(ev.State) {
			cancels.Cancel(ev.ID)
		}
	}

	plan, err := p.PlanLocalCopy([]string{"/src"}, "/dst", sink.fn)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if plan != nil {
		t.Fatalf("plan = %+v, want nil on a cancelled scan", plan)
	}
	if tree.listCalls != 1 {
		t.Fatalf("listCalls = %d, want 1: the walk kept descending after cancellation", tree.listCalls)
	}
	terminals := sink.terminals()
	if len(terminals) != 1 || terminals[0].State != "cancelled" {
		t.Fatalf("terminals = %+v, want exactly one cancelled event", terminals)
	}
}

// walkRemoteFS is a domain.RemoteFS whose List serves a nested remote tree, so
// the remote enumeration path can be cancelled mid-walk.
type walkRemoteFS struct {
	*fakeRemoteFS
	width     int
	depth     int
	listCalls int
}

func (w *walkRemoteFS) List(ctx context.Context, dir string) ([]domain.RemoteNode, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if dir == "/" {
		return []domain.RemoteNode{{Name: "src", Path: "/src", IsDir: true}}, nil
	}
	w.listCalls++
	out := make([]domain.RemoteNode, 0, w.width+1)
	for i := 0; i < w.width; i++ {
		name := fmt.Sprintf("f%d.txt", i)
		out = append(out, domain.RemoteNode{Name: name, Path: dir + "/" + name, Size: 1})
	}
	if w.listCalls < w.depth {
		out = append(out, domain.RemoteNode{Name: "deep", Path: dir + "/deep", IsDir: true})
	}
	return out, nil
}

// The same invariant on the remote side: SFTP enumeration of a huge tree must
// abort promptly, not only at the next directory boundary.
func TestPlanScanCancelAbortsRemoteWalk(t *testing.T) {
	fs := &walkRemoteFS{fakeRemoteFS: &fakeRemoteFS{}, width: 200, depth: 50}
	cancels := NewCancelRegistry()
	p := NewTransferPlanner(&fakeOpSessions{fs: fs}, &mockHostFS{}, cancels)

	sink := &planEvents{}
	sink.onEvent = func(ev TransferProgress) {
		if ev.Done >= 64 && !isTerminalState(ev.State) {
			cancels.Cancel(ev.ID)
		}
	}

	plan, err := p.PlanDownload("s1", []string{"/src"}, "/dst", sink.fn)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if plan != nil {
		t.Fatalf("plan = %+v, want nil on a cancelled scan", plan)
	}
	if fs.listCalls != 1 {
		t.Fatalf("listCalls = %d, want 1: the remote walk kept descending after cancellation", fs.listCalls)
	}
	terminals := sink.terminals()
	if len(terminals) != 1 || terminals[0].State != "cancelled" {
		t.Fatalf("terminals = %+v, want exactly one cancelled event", terminals)
	}
}

// singleFileRemoteFS is a domain.RemoteFS whose List is fully test-controlled,
// used to plant a cancel inside the source walk's single List call without
// going through the cancel registry.
type singleFileRemoteFS struct {
	*fakeRemoteFS
	listFn func(ctx context.Context, dir string) ([]domain.RemoteNode, error)
}

func (s *singleFileRemoteFS) List(ctx context.Context, dir string) ([]domain.RemoteNode, error) {
	return s.listFn(ctx, dir)
}

// The walk is not the whole scan: after it, every distinct target directory is
// probed for conflicts, and those probes swallow their own errors ("no
// conflicts here"). A cancel landing in that window must still close the item
// as cancelled rather than be swallowed into a plan that quietly found no
// conflicts.
//
// The cancellation here is delivered by cancelling the *session* context
// directly, not by calling cancels.Cancel(opID) — that models the
// terminalState path (finding 2: a session context with a deadline, or any
// cancellation that does not route through the cancel button) and, crucially,
// leaves opID's registry entry exactly as beginScan left it: registered. That
// makes it possible for this test to tell whether branch 1b's own
// cancels.Unregister(opID) actually ran: if it were removed, the entry would
// still be sitting in the registry after finishPlan returns, and the trailing
// cancels.Cancel(opID) below would report true instead of false. (Routing the
// cancel through cancels.Cancel, as an earlier version of this test did, would
// consume the entry itself and make that final assertion vacuously pass
// regardless of whether branch 1b's Unregister exists.)
func TestPlanCancelDuringConflictProbeIsHonoured(t *testing.T) {
	cancels := NewCancelRegistry()
	sessionCtx, cancelSession := context.WithCancel(context.Background())
	defer cancelSession()

	fs := &singleFileRemoteFS{
		fakeRemoteFS: &fakeRemoteFS{},
		listFn: func(context.Context, string) ([]domain.RemoteNode, error) {
			return []domain.RemoteNode{{Name: "a.txt", Path: "/src/a.txt", Size: 1}}, nil
		},
	}
	hostFS := &mockHostFS{
		listFn: func(string, bool, func(string, string) bool) ([]domain.LocalFileEntry, error) {
			// This is the conflict probe of the destination directory: the walk
			// is over by now. Cancel exactly here, bypassing the cancel registry.
			cancelSession()
			return nil, nil
		},
	}
	p := NewTransferPlanner(&fakeOpSessions{fs: fs, ctx: sessionCtx}, hostFS, cancels)

	sink := &planEvents{}
	var opID string
	sink.onEvent = func(ev TransferProgress) {
		if opID == "" {
			opID = ev.ID
		}
	}

	plan, err := p.PlanDownload("s1", []string{"/src/a.txt"}, "/dst", sink.fn)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if plan != nil {
		t.Fatalf("plan = %+v, want nil", plan)
	}
	terminals := sink.terminals()
	if len(terminals) != 1 || terminals[0].State != "cancelled" {
		t.Fatalf("terminals = %+v, want exactly one cancelled event", terminals)
	}
	if cancels.Cancel(opID) {
		t.Fatal("branch 1b did not release opID from the cancel registry: a closed operation must not stay cancellable")
	}
}

// The gap between "planning finished" and "execution started" is where the user
// resolves conflicts, and it can last minutes. The item is shown as active
// throughout, so its id must stay cancellable — ownership passes to the next
// phase rather than being dropped. Cancelling in the gap closes the item.
func TestPlannedOpStaysCancellableUntilExecuted(t *testing.T) {
	hostFS := &mockHostFS{
		statFn: func(string) (domain.HostFileInfo, error) {
			return domain.HostFileInfo{IsDir: false, Size: 7}, nil
		},
	}
	cancels := NewCancelRegistry()
	p := NewTransferPlanner(nil, hostFS, cancels)

	sink := &planEvents{}
	plan, err := p.PlanLocalCopy([]string{"/src/a.txt"}, "/dst", sink.fn)
	if err != nil {
		t.Fatal(err)
	}
	if plan.OpID == "" {
		t.Fatal("a plan awaiting execution must carry its op id")
	}
	if got := sink.terminals(); len(got) != 0 {
		t.Fatalf("terminals = %+v, want none: the item is still active", got)
	}

	if !cancels.Cancel(plan.OpID) {
		t.Fatal("plan id is no longer cancellable after planning: the panel item would hang forever")
	}
	terminals := sink.terminals()
	if len(terminals) != 1 || terminals[0].State != "cancelled" {
		t.Fatalf("terminals = %+v, want exactly one cancelled event", terminals)
	}
	if terminals[0].ID != plan.OpID {
		t.Fatalf("terminal event id = %q, want the plan's %q", terminals[0].ID, plan.OpID)
	}
}

// A drop with nothing to transfer (an empty directory) is an operation that
// happened and ended during planning. It closes as completed here — the
// executor is never called — and its id is dropped with the terminal event.
func TestPlanEmptyPlanCompletesAndReleasesID(t *testing.T) {
	hostFS := &mockHostFS{
		statFn: func(string) (domain.HostFileInfo, error) {
			return domain.HostFileInfo{IsDir: true}, nil
		},
		listFn: func(string, bool, func(string, string) bool) ([]domain.LocalFileEntry, error) {
			return nil, nil
		},
	}
	cancels := NewCancelRegistry()
	p := NewTransferPlanner(nil, hostFS, cancels)

	sink := &planEvents{}
	plan, err := p.PlanLocalCopy([]string{"/src"}, "/dst", sink.fn)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 0 {
		t.Fatalf("files = %d, want an empty plan", len(plan.Files))
	}
	terminals := sink.terminals()
	if len(terminals) != 1 || terminals[0].State != "completed" {
		t.Fatalf("terminals = %+v, want exactly one completed event", terminals)
	}
	if plan.OpID != "" {
		t.Fatalf("OpID = %q, want empty: the item is closed, there is nothing to execute", plan.OpID)
	}
	if cancels.Cancel(sink.opID(t)) {
		t.Fatal("a closed operation must not stay in the cancel registry")
	}
}

// A genuine enumeration failure — as opposed to a cancellation — closes the item
// as failed, and likewise releases the id.
func TestPlanEnumerationFailureReportsFailed(t *testing.T) {
	hostFS := &mockHostFS{
		statFn: func(string) (domain.HostFileInfo, error) {
			return domain.HostFileInfo{}, errors.New("boom")
		},
	}
	cancels := NewCancelRegistry()
	p := NewTransferPlanner(nil, hostFS, cancels)

	sink := &planEvents{}
	if _, err := p.PlanLocalCopy([]string{"/src"}, "/dst", sink.fn); err == nil {
		t.Fatal("want an error")
	}
	terminals := sink.terminals()
	if len(terminals) != 1 || terminals[0].State != "failed" {
		t.Fatalf("terminals = %+v, want exactly one failed event", terminals)
	}
	if cancels.Cancel(sink.opID(t)) {
		t.Fatal("a failed operation must not stay in the cancel registry")
	}
}

// Replace hands the abort action to the next phase without ever leaving the id
// unregistered — that gap is exactly what would make a panel item unclosable.
func TestCancelRegistryReplaceKeepsIDCancellable(t *testing.T) {
	r := NewCancelRegistry()
	var closed string
	r.Register("op-1", func() { closed = "first" })
	r.Replace("op-1", func() { closed = "second" })

	if !r.Cancel("op-1") {
		t.Fatal("id lost across Replace")
	}
	if closed != "second" {
		t.Fatalf("invoked action = %q, want the replacement", closed)
	}
	if r.Cancel("op-1") {
		t.Fatal("Cancel must consume the entry")
	}
}

// Replace must also work when the previous phase already dropped its entry:
// the receiving phase cannot know, and must not care.
func TestCancelRegistryReplaceOnMissingID(t *testing.T) {
	r := NewCancelRegistry()
	var called bool
	r.Replace("op-2", func() { called = true })
	if !r.Cancel("op-2") || !called {
		t.Fatal("Replace must register an id that was not present")
	}
}

// takeOver is Replace plus a veto. On a live id it behaves exactly like Replace,
// so the phase chain is unbroken; on an id Cancel already closed it declines and
// leaves the id unregistered, so the incoming phase cannot revive a closed item.
func TestCancelRegistryTakeOverDeclinesACancelledID(t *testing.T) {
	r := NewCancelRegistry()

	r.Register("live", func() {})
	if !r.takeOver("live", func() {}) {
		t.Fatal("takeOver declined a live id: the handoff would drop every planned transfer")
	}

	r.Register("closed", func() {})
	r.Cancel("closed")
	if r.takeOver("closed", func() {}) {
		t.Fatal("takeOver accepted an id whose terminal event is already out")
	}
	if r.Cancel("closed") {
		t.Fatal("a declined takeOver must not leave the id registered")
	}
}

// The terminal mark is a handoff signal, not a history: it must be gone the
// moment any owner speaks for the id again, or the registry would accumulate one
// entry per drop for the process lifetime.
func TestCancelRegistryTerminalMarkIsDisposedByEveryOwner(t *testing.T) {
	marks := func(r *CancelRegistry) int {
		r.mu.Lock()
		defer r.mu.Unlock()
		if len(r.terminated) != len(r.terminatedOrder) {
			t.Fatalf("terminated set and eviction order disagree: %d vs %d", len(r.terminated), len(r.terminatedOrder))
		}
		return len(r.terminated)
	}

	// Disposal 1 — the next phase claims the id (the conflict-dialog cancel that
	// ExecutePlan then declines).
	r := NewCancelRegistry()
	r.Register("a", func() {})
	r.Cancel("a")
	r.takeOver("a", func() {})
	if n := marks(r); n != 0 {
		t.Fatalf("marks after takeOver = %d, want 0", n)
	}

	// Disposal 2 — the owner reaches its own terminal event (a cancel during a
	// running transfer: the executor's deferred Unregister).
	r.Register("b", func() {})
	r.Cancel("b")
	r.Unregister("b")
	if n := marks(r); n != 0 {
		t.Fatalf("marks after Unregister = %d, want 0", n)
	}

	// Disposal 3 — a second cancel, which is what the frontend's finally does for
	// a batch abandoned in the dialog.
	r.Register("c", func() {})
	r.Cancel("c")
	r.Cancel("c")
	if n := marks(r); n != 0 {
		t.Fatalf("marks after a repeat Cancel = %d, want 0", n)
	}
}

// Nothing forces a disposal to happen: a batch cancelled and then abandoned
// (renderer gone, app closing) never comes back for its id. That path must cost
// a bounded amount of memory rather than one string per abandoned drop.
func TestCancelRegistryTerminatedSetIsBounded(t *testing.T) {
	r := NewCancelRegistry()
	for i := 0; i < terminatedCap*20; i++ {
		id := fmt.Sprintf("op-%d", i)
		r.Register(id, func() {})
		r.Cancel(id)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.terminated) > terminatedCap {
		t.Fatalf("terminated marks = %d, want at most %d", len(r.terminated), terminatedCap)
	}
	if len(r.terminatedOrder) > terminatedCap {
		t.Fatalf("eviction order = %d, want at most %d", len(r.terminatedOrder), terminatedCap)
	}
	if len(r.actions) != 0 {
		t.Fatalf("actions = %d, want 0: Cancel consumes the entry", len(r.actions))
	}
	// The most recent mark is the one that matters — it is the id a conflict
	// dialog could still be sitting on.
	newest := fmt.Sprintf("op-%d", terminatedCap*20-1)
	if _, ok := r.terminated[newest]; !ok {
		t.Fatalf("eviction dropped the newest mark %q: it must evict the oldest", newest)
	}
}

// Compile-time guard: walkRemoteFS must remain a full domain.RemoteFS.
var _ domain.RemoteFS = (*walkRemoteFS)(nil)
