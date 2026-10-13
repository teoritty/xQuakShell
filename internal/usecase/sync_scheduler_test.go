package usecase_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

// fakeTimers hands out timer channels the test fires by hand, so no test waits on a real clock.
type fakeTimers struct {
	mu      sync.Mutex
	pending []fakeTimer
	armed   chan time.Duration
}

type fakeTimer struct {
	d  time.Duration
	ch chan time.Time
}

func newFakeTimers() *fakeTimers { return &fakeTimers{armed: make(chan time.Duration, 64)} }

func (f *fakeTimers) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	f.mu.Lock()
	f.pending = append(f.pending, fakeTimer{d: d, ch: ch})
	f.mu.Unlock()
	f.armed <- d
	return ch
}

// fire fires every pending timer of duration d.
func (f *fakeTimers) fire(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := f.pending[:0]
	for _, t := range f.pending {
		if t.d == d {
			t.ch <- time.Time{}
			continue
		}
		kept = append(kept, t)
	}
	f.pending = kept
}

// waitArmed waits until a timer of duration d is armed. The timeout guards a broken scheduler; it
// never paces a passing test.
func (f *fakeTimers) waitArmed(t *testing.T, d time.Duration) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case got := <-f.armed:
			if got == d {
				return
			}
		case <-deadline:
			t.Fatalf("no timer of %s was armed", d)
		}
	}
}

type fakeSyncer struct {
	calls chan vaultsync.ID
	mu    sync.Mutex
	err   error
	// errFor overrides err for one vault.
	errFor map[vaultsync.ID]error
}

func (f *fakeSyncer) Sync(_ context.Context, vault vaultsync.ID) (usecase.SyncReport, error) {
	f.mu.Lock()
	err := f.err
	if e, ok := f.errFor[vault]; ok {
		err = e
	}
	f.mu.Unlock()
	f.calls <- vault
	return usecase.SyncReport{}, err
}

func (f *fakeSyncer) fail(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

type fakeEvents struct {
	ch    chan synctrust.RemoteEvent
	fails int
	mu    sync.Mutex
}

func (f *fakeEvents) Events(ctx context.Context) (<-chan synctrust.RemoteEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fails > 0 {
		f.fails--
		return nil, errors.New("connection refused")
	}
	return f.ch, nil
}

const (
	testDebounce = 2 * time.Second
	testInterval = 5 * time.Minute
)

type schedWorld struct {
	t      *testing.T
	timers *fakeTimers
	syncer *fakeSyncer
	events *fakeEvents
	status chan usecase.SyncStatus
	s      *usecase.SyncScheduler
	vault  vaultsync.ID
}

func newSchedWorld(t *testing.T, eventFails int) *schedWorld {
	return newSchedWorldOf(t, eventFails, vaultsync.ID{7})
}

func newSchedWorldOf(t *testing.T, eventFails int, vaults ...vaultsync.ID) *schedWorld {
	w := &schedWorld{t: t, timers: newFakeTimers(), syncer: &fakeSyncer{calls: make(chan vaultsync.ID, 16)},
		events: &fakeEvents{ch: make(chan synctrust.RemoteEvent, 4), fails: eventFails},
		status: make(chan usecase.SyncStatus, 64), vault: vaults[0]}
	w.s = usecase.NewSyncScheduler(usecase.SyncSchedulerDeps{Engine: w.syncer, Events: w.events,
		Vaults: func() []vaultsync.ID { return vaults }, After: w.timers.After,
		OnStatus: func(s usecase.SyncStatus) { w.status <- s }, Debounce: testDebounce, Interval: testInterval})
	w.s.Start(context.Background())
	t.Cleanup(w.s.Stop)
	return w
}

func (w *schedWorld) expectSync() {
	w.t.Helper()
	w.expectSyncOf(w.vault)
}

func (w *schedWorld) expectSyncOf(vault vaultsync.ID) {
	w.t.Helper()
	select {
	case v := <-w.syncer.calls:
		if v != vault {
			w.t.Fatalf("synced %v, want %v", v, vault)
		}
	case <-time.After(5 * time.Second):
		w.t.Fatal("no sync happened")
	}
}

func (w *schedWorld) expectNoSync() {
	w.t.Helper()
	select {
	case <-w.syncer.calls:
		w.t.Fatal("an unexpected sync happened")
	default:
	}
}

func TestSchedulerSyncsOnStart(t *testing.T) {
	w := newSchedWorld(t, 0)
	w.expectSync()
}

func TestLocalChangesAreDebounced(t *testing.T) {
	w := newSchedWorld(t, 0)
	w.expectSync()
	w.timers.waitArmed(t, testInterval)
	w.s.Notify()
	w.s.Notify()
	w.s.Notify()
	w.timers.waitArmed(t, testDebounce)
	w.expectNoSync()
	w.timers.fire(testDebounce)
	w.expectSync()
	w.expectNoSync()
}

func TestServerEventTriggersAPull(t *testing.T) {
	w := newSchedWorld(t, 0)
	w.expectSync()
	w.events.ch <- synctrust.RemoteEvent{Vault: w.vault, Seq: 9}
	w.expectSync()
}

func TestPeriodicFallback(t *testing.T) {
	w := newSchedWorld(t, 0)
	w.expectSync()
	w.timers.waitArmed(t, testInterval)
	w.timers.fire(testInterval)
	w.expectSync()
}

func TestIntegrityFailureStopsTheVault(t *testing.T) {
	bad, good := vaultsync.ID{7}, vaultsync.ID{8}
	w := newSchedWorldOf(t, 0, bad, good)
	w.expectSyncOf(bad)
	w.expectSyncOf(good)
	w.syncer.mu.Lock()
	w.syncer.errFor = map[vaultsync.ID]error{bad: synctrust.ErrIntegrity}
	w.syncer.mu.Unlock()
	w.timers.waitArmed(t, testInterval)
	w.timers.fire(testInterval)
	w.expectSyncOf(bad)
	w.expectSyncOf(good)
	var stopped bool
	for !stopped {
		select {
		case st := <-w.status:
			stopped = st.State == usecase.SyncStopped && errors.Is(st.Err, synctrust.ErrIntegrity)
		case <-time.After(5 * time.Second):
			t.Fatal("no stopped status after an integrity failure")
		}
	}
	// Rounds run vaults in order, so the next call after a trigger is the stopped vault if it was
	// not skipped, and the healthy one if it was.
	w.events.ch <- synctrust.RemoteEvent{Vault: bad}
	w.expectSyncOf(good)
}

func TestRateLimitWaitsAsAsked(t *testing.T) {
	w := newSchedWorld(t, 0)
	w.syncer.fail(&synctrust.RateLimitedError{RetryAfter: 42 * time.Second})
	w.expectSync()
	w.timers.waitArmed(t, 42*time.Second)
	w.syncer.fail(nil)
	w.timers.fire(42 * time.Second)
	w.expectSync()
}

func TestFailuresBackOff(t *testing.T) {
	w := newSchedWorld(t, 0)
	w.syncer.fail(errors.New("network down"))
	w.expectSync()
	w.timers.waitArmed(t, time.Second)
	w.timers.fire(time.Second)
	w.expectSync()
	w.timers.waitArmed(t, 2*time.Second)
}

func TestEventSourceReconnects(t *testing.T) {
	w := newSchedWorld(t, 1)
	w.expectSync()
	w.timers.waitArmed(t, time.Second)
	w.timers.fire(time.Second)
	w.events.ch <- synctrust.RemoteEvent{Vault: w.vault}
	w.expectSync()
}

func TestStopEndsEverything(t *testing.T) {
	w := newSchedWorld(t, 0)
	w.expectSync()
	w.s.Stop()
	w.s.Notify()
	w.s.Stop()
	w.timers.fire(testDebounce)
	w.timers.fire(testInterval)
	w.expectNoSync()
}

type countingVault struct {
	syncTestVault
	fail error
}

func (v *countingVault) UpdateData(ctx context.Context, mutate func(*domain.VaultData) error) error {
	if v.fail != nil {
		return v.fail
	}
	return v.syncTestVault.UpdateData(ctx, mutate)
}

func TestObservingVaultSignalsOnlySuccessfulWrites(t *testing.T) {
	inner := &countingVault{syncTestVault: syncTestVault{data: domain.NewVaultData()}}
	var signals int
	v := usecase.NewSyncObservingVault(inner, func() { signals++ })
	_ = v.UpdateData(syncCtx, func(*domain.VaultData) error { return nil })
	_ = v.UpdateData(syncCtx, func(*domain.VaultData) error { return errors.New("refused") })
	inner.fail = errors.New("disk full")
	_ = v.UpdateData(syncCtx, func(*domain.VaultData) error { return nil })
	if signals != 1 {
		t.Fatalf("signals = %d, want 1; only a write that landed is a change to push", signals)
	}
}
