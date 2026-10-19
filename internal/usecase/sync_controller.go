package usecase

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

var (
	// ErrSyncOff is an operation that needs sync running while it is not: disabled, or locked.
	ErrSyncOff = errors.New("sync: not running")
	// ErrHeldReauth is a confirmation of a held change without the right master password. One error
	// for a wrong password and a locked vault, so the gate is no oracle for the password.
	ErrHeldReauth = errors.New("sync: the master password is needed to accept this change")
	// ErrShredReauth is a Shred now without the right master password, one error for both causes
	// for the same reason as ErrHeldReauth.
	ErrShredReauth = errors.New("sync: the master password is needed to shred the trash and history")
	// ErrNoPendingJoin is a join choice with no login waiting for it.
	ErrNoPendingJoin = errors.New("sync: no account is waiting to be joined")
)

const (
	syncDebounce = 2 * time.Second
	syncInterval = 5 * time.Minute
)

// SyncControllerDeps are what the controller borrows from the composition root.
type SyncControllerDeps struct {
	Accounts *SyncAccountService
	// Vault is the undecorated repository: it verifies the master password, and the engines the
	// controller builds write through it without signalling a local edit.
	Vault     domain.VaultRepository
	State     synctrust.StateStore
	NewEngine func(SyncSession) *SyncEngine
	After     func(time.Duration) <-chan time.Time
	OnStatus  func(SyncStatus)
	Random    io.Reader
}

// SyncController runs sync for the open vault: it starts when the vault unlocks with sync enabled,
// stops when it locks, and carries the account operations the settings page offers. Every use of
// the engine - the scheduler's rounds and the user's actions alike - holds one lock, because two
// pulls of one vault at once would race on its stored state.
type SyncController struct {
	d SyncControllerDeps

	mu          sync.Mutex
	engine      *SyncEngine
	session     SyncSession
	sched       *SyncScheduler
	last        SyncStatus
	perVault    map[vaultsync.ID]SyncStatus
	membership  map[vaultsync.ID]int
	knownVaults []vaultsync.ID
	joining     []vaultsync.ID
	teams       map[vaultsync.ID]cachedTeam
	invites     []Invitation
	pendingJoin *SyncSession

	op sync.Mutex
}

// NewSyncController builds an idle controller.
func NewSyncController(d SyncControllerDeps) *SyncController { return &SyncController{d: d} }

// lockedSyncer serialises the scheduler's rounds with the controller's own engine calls.
type lockedSyncer struct{ c *SyncController }

func (l lockedSyncer) Sync(ctx context.Context, vault vaultsync.ID) (SyncReport, error) {
	l.c.op.Lock()
	defer l.c.op.Unlock()
	e := l.c.currentEngine()
	if e == nil {
		return SyncReport{}, ErrSyncOff
	}
	return e.Sync(ctx, vault)
}

// vaults lists the session's vaults through the running engine, under the engine lock: the listing
// may drop a team the account left, which writes to the vault.
func (c *SyncController) vaults(s SyncSession) func(context.Context) ([]vaultsync.ID, error) {
	return func(ctx context.Context) ([]vaultsync.ID, error) {
		c.op.Lock()
		defer c.op.Unlock()
		e := c.currentEngine()
		if e == nil {
			return []vaultsync.ID{s.Personal}, ErrSyncOff
		}
		return e.Vaults(ctx)
	}
}

func (c *SyncController) currentEngine() *SyncEngine {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.engine
}

// Start resumes sync for the account stored in the vault, if any. It does not touch the network;
// the first round connects.
func (c *SyncController) Start(ctx context.Context) error {
	if c.running() {
		return nil
	}
	s, ok, err := c.d.Accounts.Resume()
	if err != nil || !ok {
		return err
	}
	info, _, err := c.AccountInfo()
	if err != nil {
		return err
	}
	if info.JoinPending {
		c.mu.Lock()
		c.pendingJoin = &s
		c.mu.Unlock()
		return nil
	}
	c.adopt(ctx, s)
	return nil
}

func (c *SyncController) running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sched != nil
}

// adopt makes s the running session and starts scheduling for it.
func (c *SyncController) adopt(ctx context.Context, s SyncSession) {
	c.Stop()
	engine := c.d.NewEngine(s)
	sched := NewSyncScheduler(SyncSchedulerDeps{Engine: lockedSyncer{c}, Events: s.Server,
		Vaults: c.vaults(s), After: c.d.After, OnStatus: c.record,
		Debounce: syncDebounce, Interval: syncInterval})
	c.mu.Lock()
	c.engine, c.session, c.sched = engine, s, sched
	c.mu.Unlock()
	sched.Start(ctx)
}

// Stop ends scheduling and drops the session; the vault locking calls it, and so does every
// account operation that replaces the keys.
func (c *SyncController) Stop() {
	c.mu.Lock()
	sched := c.sched
	c.engine, c.sched, c.session = nil, nil, SyncSession{}
	// A new session starts from its own reports: a stop reason of the old keys must not outlive them.
	c.perVault, c.membership, c.knownVaults, c.joining, c.teams, c.invites, c.last = nil, nil, nil, nil, nil, nil, SyncStatus{}
	c.mu.Unlock()
	if sched != nil {
		sched.Stop()
	}
}

// Notify reports a local edit to push.
func (c *SyncController) Notify() {
	c.mu.Lock()
	sched := c.sched
	c.mu.Unlock()
	if sched != nil {
		sched.Notify()
	}
}

// SyncNow asks for a round at once.
func (c *SyncController) SyncNow() error {
	c.mu.Lock()
	sched := c.sched
	c.mu.Unlock()
	if sched == nil {
		return ErrSyncOff
	}
	sched.SyncNow()
	return nil
}

// record keeps the vault's status and reports the account's: the state, the error and the held and
// quarantined counts are the account's, while the vault and the rest of the report are the round's
// that just reported - its Applied count is how a listener knows to re-read the vault.
func (c *SyncController) record(st SyncStatus) {
	c.mu.Lock()
	if c.perVault == nil {
		c.perVault = map[vaultsync.ID]SyncStatus{}
	}
	changed := c.followVaults(st)
	changed = c.followMembership(st) || changed
	c.perVault[st.Vault] = st
	agg := rollupStatus(c.perVault, c.session.Personal)
	agg.VaultsChanged = changed
	round := st.Report
	round.Pull.Held, round.Pull.Quarantined = agg.Report.Pull.Held, agg.Report.Pull.Quarantined
	agg.Vault, agg.Report = st.Vault, round
	c.last = agg
	c.mu.Unlock()
	if c.d.OnStatus != nil {
		c.d.OnStatus(agg)
	}
}

// followVaults forgets the reports of vaults a pass no longer lists and says whether the views
// must re-read: at once when a vault left, and when a vault that joined finishes its first round.
// Saying so when the joined vault first appears was too early - its team was not in the vault yet,
// and the views re-read nothing new and were never told again. A report from before the first
// pass, or from a caller that does not list vaults, keeps what is known. The caller holds c.mu.
func (c *SyncController) followVaults(st SyncStatus) bool {
	changed := false
	if st.Vaults != nil {
		for id := range c.perVault {
			if !slices.Contains(st.Vaults, id) {
				delete(c.perVault, id)
				changed = true
			}
		}
		c.noteJoined(st.Vaults)
		c.knownVaults = slices.Clone(st.Vaults)
	}
	if st.State != SyncRunning && slices.Contains(c.joining, st.Vault) {
		c.joining = slices.DeleteFunc(c.joining, func(id vaultsync.ID) bool { return id == st.Vault })
		changed = true
	}
	return changed
}

// followMembership says whether a vault's membership log moved since the last round that read it.
// A role change or a removal carries no record, so nothing is applied and the views would keep a
// role the account no longer has: a viewer an editable form whose edits are silently put back. A
// round that never read the log reports zero and is skipped, or a failure would read as a move.
// The caller holds c.mu.
func (c *SyncController) followMembership(st SyncStatus) bool {
	seq := st.Report.Pull.MembershipSeq
	if seq == 0 {
		return false
	}
	if c.membership == nil {
		c.membership = map[vaultsync.ID]int{}
	}
	last, seen := c.membership[st.Vault]
	c.membership[st.Vault] = seq
	return seen && last != seq
}

// noteJoined remembers the vaults of a pass that the last pass did not list. The first pass
// joins nothing: every vault in it is the account's from before. The caller holds c.mu.
func (c *SyncController) noteJoined(vaults []vaultsync.ID) {
	if c.knownVaults == nil {
		return
	}
	for _, id := range vaults {
		if !slices.Contains(c.knownVaults, id) {
			c.joining = append(c.joining, id)
		}
	}
}

// Status is the account's status across its vaults, and whether sync is running.
func (c *SyncController) Status() (SyncStatus, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last, c.sched != nil
}

// withEngine runs f on the running engine under the engine lock.
func (c *SyncController) withEngine(f func(*SyncEngine, SyncSession) error) error {
	c.op.Lock()
	defer c.op.Unlock()
	c.mu.Lock()
	e, s := c.engine, c.session
	c.mu.Unlock()
	if e == nil {
		return ErrSyncOff
	}
	return f(e, s)
}
