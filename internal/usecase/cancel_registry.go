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
//
// The registry is also the single source of truth for "has this id already been
// closed?". Each phase builds its own operationReporter, and each reporter has
// its own done latch, so no reporter can enforce the one-terminal-event-per-id
// invariant across a phase handoff. The registry can, because it outlives every
// phase: Cancel records the id in `terminated`, and the next phase asks via
// takeOver instead of blindly claiming the id.
type CancelRegistry struct {
	mu      sync.Mutex
	actions map[string]func()
	// terminated holds ids whose one terminal event has already been emitted by
	// Cancel while no phase was running the work. Entries are removed as soon as
	// any owner speaks for the id again (Unregister, takeOver, a second Cancel),
	// which is the normal disposal path; terminatedOrder caps what is left when
	// nobody ever comes back — see markTerminatedLocked.
	terminated      map[string]struct{}
	terminatedOrder []string
}

// terminatedCap bounds the terminated set. The set only needs to survive the gap
// between a cancel click and the next phase's takeOver — one conflict dialog, one
// id. Entries beyond that gap exist only when no phase ever claims the id again
// (a batch the user abandoned in the dialog after cancelling it, a renderer that
// died mid-flow), and those would otherwise accumulate for the process lifetime.
// The oldest is evicted past this many, so the worst case is a fixed handful of
// strings rather than one per abandoned drop. Losing an evicted mark can only
// resurrect an id that has been parked, unclaimed, through 64 later
// cancellations, which no live conflict dialog can be.
const terminatedCap = 64

// NewCancelRegistry creates an empty registry.
func NewCancelRegistry() *CancelRegistry {
	return &CancelRegistry{
		actions:    make(map[string]func()),
		terminated: make(map[string]struct{}),
	}
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

// takeOver is Replace for a phase that can still decline the id. It answers, and
// acts on, one question atomically: was this id already closed while it was
// parked between phases? If it was, the id is *not* re-registered — the panel
// item is gone, the terminal event is out, and re-registering would revive it.
// If it was not, action becomes the id's abort action exactly as Replace would.
//
// Doing both under one lock is the point. Split into a read then a Replace, a
// cancel click landing between them would fire the previous phase's closer —
// emitting the id's terminal event — and then be overwritten by the new
// registration, so the incoming phase would proceed on an id the user already
// closed. That is the very race this method exists to remove, so callers must
// not reintroduce it by peeking first.
func (r *CancelRegistry) takeOver(id string, action func()) (accepted bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, closed := r.terminated[id]; closed {
		r.forgetTerminatedLocked(id)
		delete(r.actions, id)
		return false
	}
	r.actions[id] = action
	return true
}

// Unregister drops the entry for id. Call it only together with the operation's
// terminal event: while an item is shown as active it must remain closable.
// It also forgets any terminal mark, because an owner reaching its own terminal
// event is the definitive statement that the id is finished and nothing further
// will ask about it.
func (r *CancelRegistry) Unregister(id string) {
	r.mu.Lock()
	delete(r.actions, id)
	r.forgetTerminatedLocked(id)
	r.mu.Unlock()
}

// Cancel invokes and removes the abort action for id, if present. Returns true
// when an operation was found and aborted.
//
// A successful cancel also records the id as terminated: the action it just ran
// emitted (or triggered) the id's one terminal event, and the next phase must be
// able to find that out — see takeOver. A cancel that finds no action instead
// clears the mark: nothing owns the id any more, so the caller asking to close it
// is the last word on it.
func (r *CancelRegistry) Cancel(id string) bool {
	r.mu.Lock()
	action, ok := r.actions[id]
	delete(r.actions, id)
	if ok {
		r.markTerminatedLocked(id)
	} else {
		r.forgetTerminatedLocked(id)
	}
	r.mu.Unlock()
	if ok && action != nil {
		action()
	}
	return ok
}

// markTerminatedLocked records id as closed, evicting the oldest marks past
// terminatedCap. Caller holds r.mu.
func (r *CancelRegistry) markTerminatedLocked(id string) {
	if _, dup := r.terminated[id]; dup {
		return
	}
	r.terminated[id] = struct{}{}
	r.terminatedOrder = append(r.terminatedOrder, id)
	for len(r.terminatedOrder) > terminatedCap {
		delete(r.terminated, r.terminatedOrder[0])
		// Shift in place rather than resliceing: a plain [1:] would walk the
		// backing array forward forever, reallocating on every append.
		r.terminatedOrder = append(r.terminatedOrder[:0], r.terminatedOrder[1:]...)
	}
}

// forgetTerminatedLocked drops id's terminal mark. Caller holds r.mu.
func (r *CancelRegistry) forgetTerminatedLocked(id string) {
	if _, ok := r.terminated[id]; !ok {
		return
	}
	delete(r.terminated, id)
	for i, v := range r.terminatedOrder {
		if v == id {
			r.terminatedOrder = append(r.terminatedOrder[:i], r.terminatedOrder[i+1:]...)
			return
		}
	}
}
