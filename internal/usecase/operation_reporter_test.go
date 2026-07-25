package usecase

import (
	"sync"
	"testing"
	"time"
)

// sink collects reporter emissions. The reporter emits while holding its own
// mutex, but the sink keeps one anyway so the concurrency test stays honest
// under -race even if that ever changes.
type reporterSink struct {
	mu     sync.Mutex
	events []TransferProgress
}

func (s *reporterSink) fn(p TransferProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, p)
}

func (s *reporterSink) all() []TransferProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]TransferProgress(nil), s.events...)
}

func (s *reporterSink) terminals() []TransferProgress {
	var out []TransferProgress
	for _, e := range s.all() {
		if isTerminalState(e.State) {
			out = append(out, e)
		}
	}
	return out
}

// Every emitted event carries the operation identity and target the reporter was
// built with; Done/Total/State are the only per-event values.
func TestOperationReporterEmitsIdentityOnEveryEvent(t *testing.T) {
	sink := &reporterSink{}
	rep := newOperationReporter("op-1", "sess-1", transferKindUpload, "/var/www", sink.fn)

	rep.Started()
	rep.Finish("completed")

	events := sink.all()
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2: %+v", len(events), events)
	}
	for _, e := range events {
		if e.ID != "op-1" || e.SessionID != "sess-1" {
			t.Fatalf("identity lost: %+v", e)
		}
		if e.Kind != transferKindUpload || e.Direction != transferKindUpload {
			t.Fatalf("kind/direction = %q/%q, want upload/upload", e.Kind, e.Direction)
		}
		if e.RemotePath != "/var/www" || e.RefreshDir != "/var/www" {
			t.Fatalf("target fields = %q/%q, want /var/www", e.RemotePath, e.RefreshDir)
		}
	}
	if events[0].State != "active" || events[0].Done != 0 || events[0].Total != 0 {
		t.Fatalf("Started event = %+v, want indeterminate active", events[0])
	}
	if events[1].State != "completed" {
		t.Fatalf("terminal event = %+v, want completed", events[1])
	}
}

// withLabel overrides only the human-readable RemotePath caption; withDirection
// overrides only Direction. Neither disturbs the refresh directory.
func TestOperationReporterDisplayOverrides(t *testing.T) {
	sink := &reporterSink{}
	rep := newOperationReporter("op-2", "s", transferKindDownload, "/dst", sink.fn).
		withLabel("3 items").
		withDirection("")

	rep.Started()

	e := sink.all()[0]
	if e.RemotePath != "3 items" {
		t.Fatalf("RemotePath = %q, want the label", e.RemotePath)
	}
	if e.RefreshDir != "/dst" {
		t.Fatalf("RefreshDir = %q, want /dst", e.RefreshDir)
	}
	if e.Direction != "" {
		t.Fatalf("Direction = %q, want empty", e.Direction)
	}
	if e.Kind != transferKindDownload {
		t.Fatalf("Kind = %q, want download", e.Kind)
	}
}

// Scanned streams an indeterminate counter (Total stays 0) that increments once
// per call, independently of which of those ticks the throttle lets through.
func TestOperationReporterScannedCountsEveryTick(t *testing.T) {
	sink := &reporterSink{}
	rep := newOperationReporter("op-3", "s", transferKindUpload, "/d", sink.fn)

	for range emitEvery {
		rep.Scanned()
	}

	events := sink.all()
	if len(events) == 0 {
		t.Fatal("no scan events emitted")
	}
	last := events[len(events)-1]
	if last.Done != emitEvery || last.Total != 0 || last.State != "active" {
		t.Fatalf("last scan event = %+v, want done=%d total=0 active", last, emitEvery)
	}
}

// The done latch: the first terminal state wins and every later call — including
// a deferred Finish("failed") after a successful completion — is a no-op.
// Without the latch the safety-net defer would repaint a finished transfer as an
// error.
func TestOperationReporterLatchesFirstTerminalState(t *testing.T) {
	sink := &reporterSink{}
	rep := newOperationReporter("op-4", "s", transferKindUpload, "/d", sink.fn)

	rep.Report(10, 10, "completed")
	rep.Finish("failed")
	rep.Report(5, 10, "active")
	rep.Report(0, 10, "cancelled")

	terminals := sink.terminals()
	if len(terminals) != 1 {
		t.Fatalf("terminal events = %d, want exactly 1: %+v", len(terminals), terminals)
	}
	if terminals[0].State != "completed" {
		t.Fatalf("terminal state = %q, want completed (first wins)", terminals[0].State)
	}
	if all := sink.all(); len(all) != 1 {
		t.Fatalf("events after the latch closed = %d, want 1: %+v", len(all), all)
	}
}

// Throttling applies to progress only. A terminal event emitted immediately
// after a suppressed one must still reach the UI, or the panel item hangs.
func TestOperationReporterTerminalEventBypassesThrottle(t *testing.T) {
	sink := &reporterSink{}
	rep := newOperationReporter("op-5", "s", transferKindUpload, "/d", sink.fn)

	// Arm the throttle so the next non-terminal event is inside the quiet window.
	rep.throttle.last = time.Now()

	rep.Report(1, 10, "active") // suppressed: within emitInterval, 1 % emitEvery != 0
	if got := len(sink.all()); got != 0 {
		t.Fatalf("throttled progress event leaked: %d event(s)", got)
	}

	rep.Report(1, 10, "failed") // must not be throttled

	events := sink.all()
	if len(events) != 1 || events[0].State != "failed" {
		t.Fatalf("events = %+v, want a single failed event", events)
	}
}

// Report runs on the mover's progress goroutine while Finish runs on the
// caller's defer. The latch must hold under -race: exactly one terminal event,
// no torn state.
func TestOperationReporterConcurrentReportAndFinish(t *testing.T) {
	sink := &reporterSink{}
	rep := newOperationReporter("op-6", "s", transferKindUpload, "/d", sink.fn)

	const writers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for w := range writers {
		wg.Add(1)
		go func(base int64) {
			defer wg.Done()
			<-start
			for i := int64(0); i < 50; i++ {
				rep.Report(base*50+i, 400, "active")
				rep.Scanned()
			}
			rep.Finish("completed")
		}(int64(w))
	}
	close(start)
	wg.Wait()
	rep.Finish("failed") // the safety-net defer: must be swallowed by the latch

	terminals := sink.terminals()
	if len(terminals) != 1 {
		t.Fatalf("terminal events = %d, want exactly 1: %+v", len(terminals), terminals)
	}
	if terminals[0].State != "completed" {
		t.Fatalf("terminal state = %q, want completed", terminals[0].State)
	}
}

// A reporter built without a progress callback must stay usable — planners and
// executors are called with a nil TransferProgressFunc.
func TestOperationReporterNilEmitIsSafe(t *testing.T) {
	rep := newOperationReporter("op-7", "s", transferKindUpload, "/d", nil)
	rep.Started()
	rep.Scanned()
	rep.Report(1, 2, "active")
	rep.Finish("completed")
}

// newOpID mints an opaque, unique, kind-prefixed identifier. The kind prefix is
// there to keep logs readable; the rest is crypto-random, not a timestamp.
func TestNewOpIDIsKindPrefixedAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		id := newOpID(transferKindUpload)
		if len(id) <= len(transferKindUpload)+1 || id[:len(transferKindUpload)+1] != transferKindUpload+"-" {
			t.Fatalf("id = %q, want a %q- prefix", id, transferKindUpload)
		}
		if seen[id] {
			t.Fatalf("duplicate op id %q", id)
		}
		seen[id] = true
	}
}
