package usecase

import (
	"context"
	"errors"
	"sync"
	"time"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/pkg/safego"
)

// SyncState is where one vault's sync stands, for the status the UI shows.
type SyncState int

const (
	SyncIdle SyncState = iota + 1
	SyncRunning
	// SyncFailed is a round that failed and will be retried.
	SyncFailed
	// SyncStopped is a vault that failed verification, or whose account keys moved on without this
	// device, or a device that was removed. It stays stopped until sync restarts, which only the
	// user can do: retrying would keep asking a hostile server for more of the same.
	SyncStopped
)

// SyncStatus is one status change of one vault.
type SyncStatus struct {
	Vault  vaultsync.ID
	State  SyncState
	Err    error
	Report SyncReport
	// Vaults lists every vault of the pass that reported this status; the controller drops the
	// reports of vaults no longer in it, such as a team the account left.
	Vaults []vaultsync.ID
	// VaultsChanged is set on the account's status when the set of vaults differs from the last
	// pass's: a team joined or left, which the views must show although nothing was applied.
	VaultsChanged bool
}

// Syncer runs one sync round for a vault; SyncEngine is the implementation.
type Syncer interface {
	Sync(ctx context.Context, vault vaultsync.ID) (SyncReport, error)
}

// SyncSchedulerDeps configure a SyncScheduler.
type SyncSchedulerDeps struct {
	Engine Syncer
	// Events may be nil; the periodic round still runs.
	Events synctrust.EventSource
	// Vaults lists the vaults to sync. On an error it still returns what it can - always the
	// personal vault - and the round counts as failed so it is retried.
	Vaults func(context.Context) ([]vaultsync.ID, error)
	// After is time.After in production and a hand-fired timer in tests.
	After    func(time.Duration) <-chan time.Time
	OnStatus func(SyncStatus)
	// Debounce gathers a burst of local edits into one round; Interval is the fallback round for
	// when no event arrives.
	Debounce time.Duration
	Interval time.Duration
}

const maxSyncBackoff = 5 * time.Minute

// maxRateLimitBackoff caps how far repeated rate limits push the next pass, short of maxSyncBackoff.
// A rate limit is a server that is up and answering, and it lifts within seconds; growing to five
// minutes left a team member behind a shared NAT looking at a stale vault long after the limit was
// gone, until they pressed Sync now. The wait still grows, so clients sharing an address spread out.
const maxRateLimitBackoff = 30 * time.Second

// SyncScheduler decides when to sync: on start, after local edits settle, when the server reports
// a change, and on a slow periodic fallback. It runs only between Start and Stop, which the caller
// ties to the vault being unlocked: the keys live in the vault (plan P4-4).
type SyncScheduler struct {
	d    SyncSchedulerDeps
	kick chan struct{}
	now  chan struct{}

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     *sync.WaitGroup
}

// NewSyncScheduler builds the scheduler idle: nothing runs until Start, so it can be wired at startup
// while the vault is still locked.
func NewSyncScheduler(d SyncSchedulerDeps) *SyncScheduler {
	return &SyncScheduler{d: d, kick: make(chan struct{}, 1), now: make(chan struct{}, 1)}
}

// Start begins scheduling; a second Start while running does nothing.
func (s *SyncScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	events := make(chan synctrust.RemoteEvent, 16)
	wg := &sync.WaitGroup{}
	wg.Add(2)
	safego.GoNamed("sync.events", func() { defer wg.Done(); s.listen(ctx, events) })
	safego.GoNamed("sync.scheduler", func() { defer wg.Done(); s.loop(ctx, events) })
	s.cancel, s.wg = cancel, wg
}

// Stop ends scheduling and waits for a round in progress to return. It is safe to call twice, and
// lock paths do reach it more than once.
func (s *SyncScheduler) Stop() {
	s.mu.Lock()
	cancel, wg := s.cancel, s.wg
	s.cancel, s.wg = nil, nil
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	wg.Wait()
}

// SyncNow asks for a round at once, without the debounce: the user pressed the button.
func (s *SyncScheduler) SyncNow() {
	select {
	case s.now <- struct{}{}:
	default:
	}
}

// Notify reports a local edit. It never blocks: one pending kick is as good as many.
func (s *SyncScheduler) Notify() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

func (s *SyncScheduler) loop(ctx context.Context, events <-chan synctrust.RemoteEvent) {
	stopped := map[vaultsync.ID]bool{}
	periodic := s.d.After(s.d.Interval)
	var debounce, retry <-chan time.Time
	var backoff time.Duration
	runNow := true
	for {
		if runNow {
			runNow = false
			retry, backoff = s.runAll(ctx, stopped, backoff)
		}
		// While a failed pass waits for its retry, an edit, an event or the periodic tick does not
		// start another: the retry syncs every vault anyway, and passes started by events from the
		// other devices kept a rate-limited client asking once a second and the limit never lifted.
		// Only the person pressing Sync now goes ahead of the wait.
		idle := retry == nil
		select {
		case <-ctx.Done():
			return
		case <-s.kick:
			if debounce == nil {
				debounce = s.d.After(s.d.Debounce)
			}
		case <-debounce:
			debounce, runNow = nil, idle
		case <-s.now:
			runNow = true
		case <-retry:
			retry, runNow = nil, true
		case <-periodic:
			periodic, runNow = s.d.After(s.d.Interval), idle
		case <-events:
			runNow = idle
		}
	}
}

// runAll syncs every vault that is not stopped and returns when to retry, if anything failed.
func (s *SyncScheduler) runAll(ctx context.Context, stopped map[vaultsync.ID]bool, backoff time.Duration) (<-chan time.Time, time.Duration) {
	vaults, err := s.d.Vaults(ctx)
	failed := err != nil
	report := func(st SyncStatus) {
		st.Vaults = vaults
		s.report(st)
	}
	for _, v := range vaults {
		if stopped[v] {
			continue
		}
		report(SyncStatus{Vault: v, State: SyncRunning})
		r, err := s.d.Engine.Sync(ctx, v)
		var limited *synctrust.RateLimitedError
		switch {
		case err == nil:
			report(SyncStatus{Vault: v, State: SyncIdle, Report: r})
		case ctx.Err() != nil:
			return nil, backoff
		// A revoked device asking again gets the same refusal every time; only the person can act.
		case errors.Is(err, synctrust.ErrIntegrity), errors.Is(err, synctrust.ErrKeysRotated), errors.Is(err, ErrSyncDeviceRevoked):
			stopped[v] = true
			report(SyncStatus{Vault: v, State: SyncStopped, Err: err, Report: r})
		case errors.As(err, &limited):
			// The rest of the pass would only spend more of the limit; it waits with this vault.
			report(SyncStatus{Vault: v, State: SyncFailed, Err: err, Report: r})
			next := max(limited.RetryAfter, min(nextSyncBackoff(backoff), maxRateLimitBackoff))
			return s.d.After(next), next
		default:
			failed = true
			report(SyncStatus{Vault: v, State: SyncFailed, Err: err, Report: r})
		}
	}
	if failed {
		next := nextSyncBackoff(backoff)
		return s.d.After(next), next
	}
	return nil, 0
}

func nextSyncBackoff(cur time.Duration) time.Duration {
	if cur == 0 {
		return time.Second
	}
	return min(cur*2, maxSyncBackoff)
}

func (s *SyncScheduler) report(st SyncStatus) {
	if s.d.OnStatus != nil {
		s.d.OnStatus(st)
	}
}

// listen keeps a notification connection open, reconnecting with backoff when it drops.
func (s *SyncScheduler) listen(ctx context.Context, out chan<- synctrust.RemoteEvent) {
	if s.d.Events == nil {
		return
	}
	var backoff time.Duration
	for ctx.Err() == nil {
		ch, err := s.d.Events.Events(ctx)
		if err == nil {
			backoff = 0
			forwardEvents(ctx, ch, out)
		}
		backoff = nextSyncBackoff(backoff)
		select {
		case <-ctx.Done():
			return
		case <-s.d.After(backoff):
		}
	}
}

func forwardEvents(ctx context.Context, in <-chan synctrust.RemoteEvent, out chan<- synctrust.RemoteEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, open := <-in:
			if !open {
				return
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}
}
