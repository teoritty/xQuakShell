package loghub

import (
	"sync"
)

const defaultCapacity = 5000

// Hub stores recent log entries and fans them out to subscribers.
type Hub struct {
	mu      sync.RWMutex
	buffer  []Entry
	cap     int
	subs    map[int]chan Entry
	nextSub int
}

// NewHub creates a hub with the given ring buffer capacity.
func NewHub(capacity int) *Hub {
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	return &Hub{
		cap:  capacity,
		subs: make(map[int]chan Entry),
	}
}

var defaultHub = NewHub(defaultCapacity)

// Default returns the process-wide log hub.
func Default() *Hub {
	return defaultHub
}

// Publish appends an entry and notifies subscribers.
func (h *Hub) Publish(e Entry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.appendLocked(e)
	h.broadcastLocked(e)
}

func (h *Hub) appendLocked(e Entry) {
	h.buffer = append(h.buffer, e)
	if len(h.buffer) > h.cap {
		h.buffer = h.buffer[len(h.buffer)-h.cap:]
	}
}

func (h *Hub) broadcastLocked(e Entry) {
	for _, ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Snapshot returns a copy of the current ring buffer.
func (h *Hub) Snapshot() []Entry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Entry, len(h.buffer))
	copy(out, h.buffer)
	return out
}

// Subscribe returns a subscription id, backlog snapshot, and a channel of live entries.
func (h *Hub) Subscribe(buffer int) (id int, backlog []Entry, ch <-chan Entry) {
	if buffer <= 0 {
		buffer = 64
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	id = h.nextSub
	h.nextSub++
	backlog = make([]Entry, len(h.buffer))
	copy(backlog, h.buffer)
	c := make(chan Entry, buffer)
	h.subs[id] = c
	return id, backlog, c
}

// Unsubscribe removes a subscription and closes its channel.
func (h *Hub) Unsubscribe(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.subs[id]; ok {
		delete(h.subs, id)
		close(ch)
	}
}
