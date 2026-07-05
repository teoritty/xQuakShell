package conlimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiterRespectsLimit(t *testing.T) {
	l := New(2)
	ctx := context.Background()

	var peak int32
	var active int32
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.Acquire(ctx); err != nil {
				t.Error(err)
				return
			}
			defer l.Release()
			cur := atomic.AddInt32(&active, 1)
			for {
				old := atomic.LoadInt32(&peak)
				if cur <= old || atomic.CompareAndSwapInt32(&peak, old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt32(&active, -1)
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&peak); got > 2 {
		t.Fatalf("peak concurrent = %d, want <= 2", got)
	}
}

func TestLimiterAcquireContextCancel(t *testing.T) {
	l := New(1)
	ctx, cancel := context.WithCancel(context.Background())

	if err := l.Acquire(ctx); err != nil {
		t.Fatal(err)
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- l.Acquire(ctx)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-waitDone:
		if err == nil {
			t.Fatal("expected context error")
		}
	case <-time.After(time.Second):
		t.Fatal("Acquire did not unblock after context cancel")
	}
	l.Release()
}

func TestLimiterSetLimit(t *testing.T) {
	l := New(4)
	l.SetLimit(8)
	if got := l.Limit(); got != 8 {
		t.Fatalf("Limit() = %d, want 8", got)
	}
	l.SetLimit(0)
	if got := l.Limit(); got != 1 {
		t.Fatalf("Limit() after zero = %d, want 1", got)
	}
}
