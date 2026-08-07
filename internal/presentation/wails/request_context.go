package wails

import (
	"context"
	"time"
)

// shutdownCleanupTimeout bounds the teardown work in AppAPI.Shutdown, which
// cannot inherit the app context because that context is what is dying. Long
// enough for a plugin to exit and an audit retention pass to run, short enough
// that a wedged one does not hold the window open.
const shutdownCleanupTimeout = 10 * time.Second

// reqCtx is the context every bound RPC handler runs under.
//
// Wails binds methods by reflection and cannot pass a context in, so the app
// context captured at startup is the only lifecycle a handler can attach to.
// Handlers used to reach for context.Background() individually, which meant no
// operation the UI started could ever be cancelled: closing the window left
// in-flight transfers and plugin calls running against a backend that was
// shutting down underneath them.
//
// The nil branch is not defensive padding. Wails calls SetContext during
// startup, so every handler reachable before that - and every unit test, which
// constructs AppAPI directly and never has a Wails runtime - would otherwise
// dereference a nil context.
func (a *AppAPI) reqCtx() context.Context {
	if a == nil || a.ctx == nil {
		return context.Background()
	}
	return a.ctx
}
