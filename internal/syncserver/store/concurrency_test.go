package store_test

import (
	"fmt"
	"sync"
	"testing"

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
