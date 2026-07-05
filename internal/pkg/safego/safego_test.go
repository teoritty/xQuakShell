package safego

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGo_RecoversPanic(t *testing.T) {
	done := make(chan struct{}, 1)
	restore := SetPanicHandler(func(name string, recovered any, stack []byte) {
		if recovered != "boom" {
			t.Errorf("recovered = %v, want boom", recovered)
		}
		done <- struct{}{}
	})
	defer restore()

	Go(func() { panic("boom") })

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for panic handler")
	}
}

func TestGoNamed_IncludesLabel(t *testing.T) {
	done := make(chan struct{}, 1)
	restore := SetPanicHandler(func(name string, recovered any, stack []byte) {
		if name != "test.worker" {
			t.Errorf("name = %q, want test.worker", name)
		}
		done <- struct{}{}
	})
	defer restore()

	GoNamed("test.worker", func() { panic("boom") })

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for panic handler")
	}
}

func TestGo_NormalReturn(t *testing.T) {
	var called atomic.Bool
	restore := SetPanicHandler(func(name string, recovered any, stack []byte) {
		called.Store(true)
	})
	defer restore()

	done := make(chan struct{})
	Go(func() {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for goroutine")
	}
	if called.Load() {
		t.Fatal("panic handler called on normal return")
	}
}

func TestGo_PanicAfterDefer(t *testing.T) {
	var cleanupDone atomic.Bool
	done := make(chan struct{}, 1)
	restore := SetPanicHandler(func(name string, recovered any, stack []byte) {
		if !cleanupDone.Load() {
			t.Error("cleanup defer did not run before panic handler")
		}
		done <- struct{}{}
	})
	defer restore()

	Go(func() {
		defer func() { cleanupDone.Store(true) }()
		panic("after defer")
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for panic handler")
	}
}

func TestGoNamed_WaitGroupCompat(t *testing.T) {
	var wg sync.WaitGroup
	var cleanupDone atomic.Bool
	done := make(chan struct{}, 1)
	restore := SetPanicHandler(func(name string, recovered any, stack []byte) {
		done <- struct{}{}
	})
	defer restore()

	wg.Add(1)
	GoNamed("test.wg", func() {
		defer wg.Done()
		defer func() { cleanupDone.Store(true) }()
		panic("wg panic")
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for panic handler")
	}

	wg.Wait()
	if !cleanupDone.Load() {
		t.Fatal("defer cleanup did not run")
	}
}
