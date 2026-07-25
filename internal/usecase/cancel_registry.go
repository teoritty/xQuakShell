package usecase

import "sync"

// CancelRegistry maps an operation id to the action that aborts it. The action
// is whatever ends the operation *and closes its panel item*: cancelling a
// context while work runs, or emitting a terminal event while the operation is
// parked between phases (e.g. waiting for the user to resolve conflicts).
//
// Registration means "this item is active and someone must be able to close it",
// not "work is running right now". Ownership passes along the phase chain
// (plan -> resolve conflicts -> execute) via Replace and is dropped only
// together with the terminal event.
//
// There is exactly one registry per application: the id space is shared by the
// planner, the transfer executor and RemoteOpService, so a single map is what
// lets CancelTransfer stay one lookup instead of a walk over every owner.
// It is safe for concurrent use.
type CancelRegistry struct {
	mu      sync.Mutex
	actions map[string]func()
}

// NewCancelRegistry creates an empty registry.
func NewCancelRegistry() *CancelRegistry {
	return &CancelRegistry{actions: make(map[string]func())}
}

// Register stores the abort action for a newly active operation. The caller
// must register before emitting the operation's first progress event, so the
// cancel button the panel draws does something from the instant the item
// appears.
func (r *CancelRegistry) Register(id string, action func()) {
	r.set(id, action)
}

// Replace swaps the abort action for id, keeping the registration alive across a
// phase handoff. It is deliberately distinct from Register at the call site: it
// documents that the id was already owned by the previous phase and that the
// item stays active and closable throughout. It must not matter whether the
// previous phase already dropped its entry, so Replace stores unconditionally.
func (r *CancelRegistry) Replace(id string, action func()) {
	r.set(id, action)
}

func (r *CancelRegistry) set(id string, action func()) {
	r.mu.Lock()
	r.actions[id] = action
	r.mu.Unlock()
}

// Unregister drops the entry for id. Call it only together with the operation's
// terminal event: while an item is shown as active it must remain closable.
func (r *CancelRegistry) Unregister(id string) {
	r.mu.Lock()
	delete(r.actions, id)
	r.mu.Unlock()
}

// Cancel invokes and removes the abort action for id, if present. Returns true
// when an operation was found and aborted.
func (r *CancelRegistry) Cancel(id string) bool {
	r.mu.Lock()
	action, ok := r.actions[id]
	delete(r.actions, id)
	r.mu.Unlock()
	if ok && action != nil {
		action()
	}
	return ok
}
