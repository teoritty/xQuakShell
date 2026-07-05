// Package safego launches background goroutines with panic recovery so a panic
// in one goroutine cannot crash the entire process.
package safego

import (
	"log/slog"
	"runtime/debug"
	"sync"
)

type panicHandler func(name string, recovered any, stack []byte)

var (
	panicHandlerMu sync.RWMutex
	onPanic        panicHandler = defaultPanicHandler
)

func defaultPanicHandler(name string, recovered any, stack []byte) {
	args := []any{
		slog.Any("panic", recovered),
		slog.String("stack", string(stack)),
	}
	if name != "" {
		args = append([]any{slog.String("goroutine", name)}, args...)
	}
	slog.Error("recovered panic in goroutine", args...)
}

// SetPanicHandler replaces panic logging. Intended for tests only.
// The returned function restores the previous handler.
func SetPanicHandler(h panicHandler) func() {
	panicHandlerMu.Lock()
	prev := onPanic
	onPanic = h
	panicHandlerMu.Unlock()
	return func() {
		panicHandlerMu.Lock()
		onPanic = prev
		panicHandlerMu.Unlock()
	}
}

// Go runs fn in a new goroutine. Panics are recovered and logged; they do not crash the process.
func Go(fn func()) {
	GoNamed("", fn)
}

// GoNamed is like Go but adds a stable label for panic diagnostics.
// Production call sites should always pass a dotted name such as "ipc.readLoop".
func GoNamed(name string, fn func()) {
	go func() {
		defer recoverAndLog(name)
		fn()
	}()
}

func recoverAndLog(name string) {
	r := recover()
	if r == nil {
		return
	}
	stack := debug.Stack()
	panicHandlerMu.RLock()
	h := onPanic
	panicHandlerMu.RUnlock()
	h(name, r, stack)
}
