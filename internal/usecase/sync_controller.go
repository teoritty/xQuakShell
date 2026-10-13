package usecase

import (
	"context"
	"errors"
	"io"
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
		Vaults: func() []vaultsync.ID { return []vaultsync.ID{s.Personal} }, After: c.d.After, OnStatus: c.record,
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

func (c *SyncController) record(st SyncStatus) {
	c.mu.Lock()
	c.last = st
	c.mu.Unlock()
	if c.d.OnStatus != nil {
		c.d.OnStatus(st)
	}
}

// Status is the last status a round reported, and whether sync is running.
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
