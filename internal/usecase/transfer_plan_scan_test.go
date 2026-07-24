package usecase

import (
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
	entries, err := walkLocalSource(fs, "/src", func() { ticks++ })
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
	entries, err := walkLocalSource(fs, "/src/a.txt", nil)
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

	onScan, emit := newScanReporter("op-1", "sess", transferKindDownload, "/dst", onProgress)
	emit(0, "active")
	for range emitEvery { // guarantees at least one throttled emit
		onScan()
	}

	plan, err := finishPlan(&TransferPlan{Kind: transferKindDownload}, nil, "op-1", emit)
	if err != nil {
		t.Fatal(err)
	}
	if plan.OpID != "op-1" {
		t.Fatalf("OpID = %q, want op-1", plan.OpID)
	}
	if len(events) < 2 {
		t.Fatalf("want initial + scanning events, got %d", len(events))
	}
	for _, e := range events {
		if e.ID != "op-1" || e.Total != 0 || e.State != "active" || e.Kind != transferKindDownload {
			t.Fatalf("unexpected scan event: %+v", e)
		}
	}
}

// A failed enumeration must retire the transient scan item with a terminal event
// rather than leaving it spinning.
func TestFinishPlanEmitsFailedOnError(t *testing.T) {
	var events []TransferProgress
	_, emit := newScanReporter("op-x", "s", transferKindUpload, "/d", func(p TransferProgress) {
		events = append(events, p)
	})
	if _, err := finishPlan(nil, errors.New("boom"), "op-x", emit); err == nil {
		t.Fatal("want error")
	}
	if len(events) != 1 || events[0].State != "failed" {
		t.Fatalf("events = %+v, want one failed", events)
	}
}
