package usecase

import (
	"context"
	"sync"
)

// cancelRegistry maps operation IDs to their cancel functions so a running
// background operation (transfer, delete, recursive chmod/chown) can be
// cancelled by ID from the presentation layer. It is safe for concurrent use.
type cancelRegistry struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func newCancelRegistry() *cancelRegistry {
	return &cancelRegistry{cancels: make(map[string]context.CancelFunc)}
}

// Register stores the cancel function for id.
func (r *cancelRegistry) Register(id string, cancel context.CancelFunc) {
	r.mu.Lock()
	r.cancels[id] = cancel
	r.mu.Unlock()
}

// Unregister drops the entry for id (called when the operation finishes).
func (r *cancelRegistry) Unregister(id string) {
	r.mu.Lock()
	delete(r.cancels, id)
	r.mu.Unlock()
}

// Cancel invokes and removes the cancel function for id, if present. Returns
// true when an operation was found and cancelled.
func (r *cancelRegistry) Cancel(id string) bool {
	r.mu.Lock()
	cancel, ok := r.cancels[id]
	delete(r.cancels, id)
	r.mu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
	return ok
}
