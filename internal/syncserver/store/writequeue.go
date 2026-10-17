package store

import (
	"context"
	"slices"
	"sync"
)

// writeQueue admits this process's writers to the database one at a time, in the order they asked.
//
// BEGIN IMMEDIATE already runs transactions one at a time, but SQLite's busy timeout is not a queue:
// a blocked writer sleeps and retries, and when other writers keep winning the lock it gives up with
// SQLITE_BUSY after the timeout however short its own work is. On a slow disk that starved a writer
// in a test; on a busy server it fails a client's request. Waiting here instead hands the lock to
// the longest-waiting writer, and leaves SQLite's timeout to the only contention left - another
// process on the same file, such as an admin command.
//
// Neither sync.Mutex nor a one-connection pool would do: a Mutex wait cannot be cancelled, and
// database/sql hands a freed connection to a random waiter, not the first.
type writeQueue struct {
	mu      sync.Mutex
	held    bool
	waiters []chan struct{}
}

// acquire waits for this writer's turn, or until ctx ends. On error the caller holds nothing.
func (q *writeQueue) acquire(ctx context.Context) error {
	q.mu.Lock()
	if !q.held {
		q.held = true
		q.mu.Unlock()
		return nil
	}
	turn := make(chan struct{})
	q.waiters = append(q.waiters, turn)
	q.mu.Unlock()
	select {
	case <-turn:
		return nil
	case <-ctx.Done():
		q.mu.Lock()
		defer q.mu.Unlock()
		if i := slices.Index(q.waiters, turn); i >= 0 {
			q.waiters = slices.Delete(q.waiters, i, i+1)
			return ctx.Err()
		}
		// release handed this writer the turn as its context ended. It is not taking it, so the
		// next writer gets it; returning without passing it on would leave the queue held forever.
		q.handOffLocked()
		return ctx.Err()
	}
}

// release ends the holder's turn and gives it to the writer that has waited longest.
func (q *writeQueue) release() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handOffLocked()
}

// handOffLocked passes the lock straight to the first waiter, so nobody arriving later can take it
// in between. The caller holds q.mu.
func (q *writeQueue) handOffLocked() {
	if len(q.waiters) == 0 {
		q.held = false
		return
	}
	next := q.waiters[0]
	q.waiters = q.waiters[1:]
	close(next)
}
