package usecase

import (
	"context"
	"errors"
	"testing"

	"xquakshell/internal/domain"
)

// walkLocalSource must tick onScan exactly once per discovered entry so the
// planner can stream a live "scanning" counter during enumeration.
func TestWalkLocalSourceTicksOnScanPerEntry(t *testing.T) {
	fs := &mockHostFS{
		statFn: func(string) (domain.HostFileInfo, error) {
			return domain.HostFileInfo{IsDir: true}, nil
		},
		listFn: func(dir string, _ bool, _ func(string, string) bool) ([]domain.LocalFileEntry, error) {
			switch dir {
			case "/src":
				return []domain.LocalFileEntry{
					{Name: "a.txt", Path: "/src/a.txt", Size: 1},
					{Name: "sub", Path: "/src/sub", IsDir: true},
				}, nil
			case "/src/sub":
				return []domain.LocalFileEntry{
					{Name: "b.txt", Path: "/src/sub/b.txt", Size: 2},
				}, nil
			}
			return nil, nil
		},
	}
	var ticks int
	entries, err := walkLocalSource(context.Background(), fs, "/src", func() { ticks++ })
	if err != nil {
		t.Fatal(err)
	}
	// root + a.txt + sub + sub/b.txt
	if len(entries) != 4 {
		t.Fatalf("entries = %d, want 4", len(entries))
	}
	if ticks != len(entries) {
		t.Fatalf("ticks = %d, want %d (one per entry)", ticks, len(entries))
	}
}

// walkLocalSource tolerates a nil onScan (e.g. non-planner callers).
func TestWalkLocalSourceNilOnScan(t *testing.T) {
	fs := &mockHostFS{
		statFn: func(string) (domain.HostFileInfo, error) {
			return domain.HostFileInfo{IsDir: false, Size: 3}, nil
		},
	}
	entries, err := walkLocalSource(context.Background(), fs, "/src/a.txt", nil)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %d err = %v", len(entries), err)
	}
}

// The scan reporter streams indeterminate "scanning" progress under the op id,
// and finishPlan stamps that id onto a successful plan so the executor can reuse
// it for one continuous panel item.
func TestScanReporterEmitsAndFinishPlanStampsOpID(t *testing.T) {
	var events []TransferProgress
	onProgress := func(p TransferProgress) { events = append(events, p) }

	rep := newOperationReporter(newOpID(transferKindDownload), "sess", transferKindDownload, "/dst", onProgress)
	rep.Started()
	for range emitEvery { // guarantees at least one throttled emit
		rep.Scanned()
	}

	pending := &TransferPlan{Kind: transferKindDownload, Files: []PlannedFile{{Source: "/a", Target: "/dst/a"}}}
	plan, err := finishPlan(context.Background(), pending, nil, rep, NewCancelRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if plan.OpID == "" || plan.OpID != rep.opID {
		t.Fatalf("OpID = %q, want the scan reporter's id %q", plan.OpID, rep.opID)
	}
	if len(events) < 2 {
		t.Fatalf("want initial + scanning events, got %d", len(events))
	}
	for _, e := range events {
		if e.ID != rep.opID || e.Total != 0 || e.State != "active" || e.Kind != transferKindDownload {
			t.Fatalf("unexpected scan event: %+v", e)
		}
	}
}

// The backend must report a local copy's own honest kind — presentation
// decisions (icon, rate display) belong to the panel, not the usecase layer.
// This pins the removal of the old batchDisplayKind rewrite to "upload".
func TestScanReporterLocalCopyReportsOwnKind(t *testing.T) {
	var events []TransferProgress
	rep := newOperationReporter(newOpID(transferKindLocalCopy), "", transferKindLocalCopy, "/dst", func(p TransferProgress) {
		events = append(events, p)
	})
	rep.Started()
	if len(events) != 1 || events[0].Kind != transferKindLocalCopy {
		t.Fatalf("events = %+v, want one event with kind %q", events, transferKindLocalCopy)
	}
}

// A broken enumeration must retire the transient scan item with a terminal event
// rather than leaving it spinning — and must distinguish the user cancelling the
// scan from a genuine failure, since after the walk learned to honour the
// context, cancellation is the most common error it returns.
func TestFinishPlanDistinguishesCancelledFromFailed(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	for _, tc := range []struct {
		name  string
		ctx   context.Context
		err   error
		state string
	}{
		{"genuine failure", context.Background(), errors.New("boom"), "failed"},
		{"user cancellation", cancelled, context.Canceled, "cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []TransferProgress
			rep := newOperationReporter(newOpID(transferKindUpload), "s", transferKindUpload, "/d", func(p TransferProgress) {
				events = append(events, p)
			})
			cancels := NewCancelRegistry()
			cancels.Register(rep.opID, func() {})

			if _, err := finishPlan(tc.ctx, nil, tc.err, rep, cancels); err == nil {
				t.Fatal("want error")
			}
			if len(events) != 1 || events[0].State != tc.state {
				t.Fatalf("events = %+v, want one %s", events, tc.state)
			}
			if cancels.Cancel(rep.opID) {
				t.Fatal("a closed operation must not stay in the cancel registry")
			}
		})
	}
}
