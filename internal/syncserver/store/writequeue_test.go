package store

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// queuedForTest reports how many writers wait. Tests wait on it instead of sleeping: a goroutine
// counts as queued only once acquire has put it in line.
func (q *writeQueue) queuedForTest() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.waiters)
}

func waitQueued(t *testing.T, q *writeQueue, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for q.queuedForTest() != n {
		if time.Now().After(deadline) {
			t.Fatalf("queued = %d, want %d", q.queuedForTest(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

// Writers are admitted in the order they asked. SQLite's busy wait has no order at all, which is
// how one writer starved past its timeout while others kept taking the lock.
func TestWriteQueueAdmitsWritersInTheOrderTheyAsked(t *testing.T) {
	var q writeQueue
	if err := q.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	const n = 8
	var mu sync.Mutex
	var order []int
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := q.acquire(context.Background()); err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			q.release()
		}()
		waitQueued(t, &q, i+1)
	}
	q.release()
	wg.Wait()
	if want := []int{0, 1, 2, 3, 4, 5, 6, 7}; !slices.Equal(order, want) {
		t.Fatalf("admitted in order %v, want %v; the queue must be first come, first served", order, want)
	}
}

// A writer whose request ends while it waits leaves the line without holding the lock, and the
// writers behind it are not stuck waiting for a turn nobody will take.
func TestWriteQueueDropsAWriterWhoseContextEnds(t *testing.T) {
	var q writeQueue
	if err := q.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	cctx, cancel := context.WithCancel(context.Background())
	gaveUp := make(chan error, 1)
	go func() { gaveUp <- q.acquire(cctx) }()
	waitQueued(t, &q, 1)
	behind := make(chan error, 1)
	go func() { behind <- q.acquire(context.Background()) }()
	waitQueued(t, &q, 2)

	cancel()
	if err := <-gaveUp; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled for a writer whose request ended", err)
	}
	waitQueued(t, &q, 1)
	q.release()
	select {
	case err := <-behind:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the writer behind a cancelled one never got its turn")
	}
	q.release()
	if err := q.acquire(context.Background()); err != nil {
		t.Fatalf("the queue is free again: %v", err)
	}
}

// The turn can reach a writer in the instant its context ends: release closes its channel while
// acquire is already on the cancelled branch. That writer then holds a turn it will not use, and
// unless it passes it on, the queue stays held with nobody holding it and every later writer waits
// forever. The test holds q.mu so the cancelled branch is the only one acquire can take, then hands
// the turn over itself, as release would at that moment.
func TestWriteQueuePassesOnATurnHandedToAWriterThatGaveUp(t *testing.T) {
	var q writeQueue
	if err := q.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	cctx, cancel := context.WithCancel(context.Background())
	gaveUp := make(chan error, 1)
	go func() {
		err := q.acquire(cctx)
		if err == nil {
			// The turn won the race after all; the writer takes it and is done.
			q.release()
		}
		gaveUp <- err
	}()
	waitQueued(t, &q, 1)
	behind := make(chan error, 1)
	go func() { behind <- q.acquire(context.Background()) }()
	waitQueued(t, &q, 2)

	q.mu.Lock()
	cancel()
	// While q.mu is held nothing can close the writer's turn, so its select can only see the
	// cancellation; this pause lets it get there and block on q.mu.
	time.Sleep(20 * time.Millisecond)
	q.handOffLocked()
	q.mu.Unlock()

	<-gaveUp
	select {
	case err := <-behind:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the turn handed to a writer that gave up was lost; the writer behind it waits forever")
	}
}

// However cancellations and releases interleave, the lock never has two holders and a turn handed
// to a writer that just gave up is passed on, not lost.
func TestWriteQueueNeverHasTwoHolders(t *testing.T) {
	var q writeQueue
	var holders, admitted, cancelled atomic.Int32
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(i%4)*time.Millisecond)
			defer cancel()
			if i%4 == 0 {
				ctx = context.Background()
			}
			if err := q.acquire(ctx); err != nil {
				cancelled.Add(1)
				return
			}
			if holders.Add(1) != 1 {
				t.Error("two writers held the lock at once")
			}
			admitted.Add(1)
			time.Sleep(200 * time.Microsecond)
			holders.Add(-1)
			q.release()
		}()
	}
	wg.Wait()
	if admitted.Load()+cancelled.Load() != 64 || admitted.Load() < 16 {
		t.Fatalf("admitted %d, cancelled %d; every writer with no deadline must get its turn", admitted.Load(), cancelled.Load())
	}
	done := make(chan error, 1)
	go func() { done <- q.acquire(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a turn was lost: the queue stays held with nobody holding it")
	}
}
