package usecase

import (
	"context"
	"errors"
	"testing"

	"xquakshell/internal/domain"
)

// A transfer the user cancels must come back as ErrTransferCancelled, whatever the interrupted
// copy happened to fail with, and a transfer that genuinely fails must not. The handler turns the
// first into silence and the second into an error dialog, so this line decides which one the
// user sees.

// interruptedHostFS stands in for a copy that is in flight when the cancel lands: CopyTo runs
// onCopy (the user's click) and then fails the way an interrupted write does - with an error that
// knows nothing about contexts.
type interruptedHostFS struct {
	*mockHostFS
	onCopy  func()
	copyErr error
}

func (h *interruptedHostFS) CopyTo(_, _ string) error {
	if h.onCopy != nil {
		h.onCopy()
	}
	return h.copyErr
}

func existingSourceHostFS() *mockHostFS {
	return &mockHostFS{statFn: func(string) (domain.HostFileInfo, error) {
		return domain.HostFileInfo{Size: 7}, nil
	}}
}

func TestExecutePlanCancelledMidCopyIsReportedAsCancellation(t *testing.T) {
	cancels := NewCancelRegistry()
	hostFS := &interruptedHostFS{
		mockHostFS: existingSourceHostFS(),
		copyErr:    errors.New("write /dst/a.txt: file already closed"),
	}
	sink := &execEvents{}
	plan, err := NewTransferPlanner(nil, hostFS, cancels).PlanLocalCopy([]string{"/src/a.txt"}, "/dst", sink.fn)
	if err != nil {
		t.Fatalf("PlanLocalCopy: %v", err)
	}
	hostFS.onCopy = func() { cancels.Cancel(plan.OpID) }

	err = execService(hostFS, cancels).ExecutePlan(context.Background(), "", plan, nil, sink.fn)
	if !errors.Is(err, ErrTransferCancelled) {
		t.Fatalf("err = %v, want ErrTransferCancelled: the copy's own error does not say it was cancelled", err)
	}
	terminals := sink.terminals()
	if len(terminals) != 1 || terminals[0].State != "cancelled" {
		t.Fatalf("terminals = %+v, want one cancelled event; the RPC result must agree with the row", terminals)
	}
}

func TestExecutePlanRealFailureIsNotMistakenForCancellation(t *testing.T) {
	cancels := NewCancelRegistry()
	hostFS := &interruptedHostFS{
		mockHostFS: existingSourceHostFS(),
		copyErr:    errors.New("disk full"),
	}
	sink := &execEvents{}
	plan, err := NewTransferPlanner(nil, hostFS, cancels).PlanLocalCopy([]string{"/src/a.txt"}, "/dst", sink.fn)
	if err != nil {
		t.Fatalf("PlanLocalCopy: %v", err)
	}

	err = execService(hostFS, cancels).ExecutePlan(context.Background(), "", plan, nil, sink.fn)
	if err == nil || errors.Is(err, ErrTransferCancelled) {
		t.Fatalf("err = %v, want the real failure; hiding it would tell the user a failed copy succeeded", err)
	}
	terminals := sink.terminals()
	if len(terminals) != 1 || terminals[0].State != "failed" {
		t.Fatalf("terminals = %+v, want one failed event", terminals)
	}
}

func TestSettleCancelLeavesSuccessAndLiveFailuresAlone(t *testing.T) {
	live := context.Background()
	if err := settleCancel(live, nil); err != nil {
		t.Errorf("settleCancel(live, nil) = %v, want nil", err)
	}
	boom := errors.New("boom")
	if err := settleCancel(live, boom); err != boom {
		t.Errorf("settleCancel(live, boom) = %v, want boom unchanged", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := settleCancel(cancelled, nil); err != nil {
		t.Errorf("settleCancel(cancelled, nil) = %v, want nil: a transfer that finished is not cancelled", err)
	}
	err := settleCancel(cancelled, boom)
	if !errors.Is(err, ErrTransferCancelled) || !errors.Is(err, boom) {
		t.Errorf("settleCancel(cancelled, boom) = %v, want ErrTransferCancelled wrapping boom", err)
	}
}
