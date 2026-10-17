package store_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/syncserver/usecase"
)

// Every use case transaction reads before it writes - an invite is checked, then consumed; a head is
// compared, then advanced. Started as a plain deferred transaction, such a read takes a snapshot, and
// when another connection commits a write in between, SQLite refuses the upgrade to a writer at once
// with SQLITE_BUSY: the busy timeout does not apply, because waiting could never make that snapshot
// current again. Under two devices syncing at the same moment that failure reached clients - and
// in registration, joined with ErrForbidden, as "your role does not allow that". Transactions must
// take the write lock when they begin, so a second writer waits instead.
func TestConcurrentReadThenWriteTransactionsAllCommit(t *testing.T) {
	db, _ := openTemp(t)
	const workers, rounds = 8, 25
	var wg sync.WaitGroup
	errs := make(chan error, workers*rounds)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range rounds {
				errs <- db.WithTx(ctx, func(tx usecase.Tx) error {
					hash := []byte(fmt.Sprintf("invite-%d-%d", w, r))
					if _, err := tx.InviteUsable(hash, 1); err != nil {
						return err
					}
					return tx.InsertInvite(hash, 100)
				})
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("a concurrent read-then-write transaction failed: %v; a writer must wait for the lock, not fail", err)
		}
	}
}

// A writer queued behind a transaction that outlasts SQLite's busy timeout still commits. SQLite's
// own wait is not a queue: a blocked writer sleeps and retries, and when another one keeps winning
// the lock it gives up with SQLITE_BUSY once five seconds have passed. On a slow Windows runner the
// test above lost a writer that way, and a busy server would lose a client's request the same way.
// Holding the lock for longer than the timeout is that starvation made certain.
func TestAWriterBehindALongTransactionWaitsItsTurn(t *testing.T) {
	db, _ := openTemp(t)
	holding, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- db.WithTx(ctx, func(tx usecase.Tx) error {
			if err := tx.InsertInvite([]byte("first"), 100); err != nil {
				return err
			}
			close(holding)
			<-release
			return nil
		})
	}()
	<-holding
	second := make(chan error, 1)
	go func() {
		second <- db.WithTx(ctx, func(tx usecase.Tx) error { return tx.InsertInvite([]byte("second"), 100) })
	}()
	// Longer than busy_timeout(5000): SQLite's wait alone has given up by now.
	time.Sleep(5500 * time.Millisecond)
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("the holding transaction: %v", err)
	}
	if err := <-second; err != nil {
		t.Fatalf("the queued writer failed: %v; it must wait its turn however long the one ahead takes", err)
	}
}
