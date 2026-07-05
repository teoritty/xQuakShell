package loghub

import (
	"sync"

	"ssh-client/internal/domain"
)

const defaultCapacity = 5000

// Hub stores recent log entries and fans them out to subscribers.
type Hub struct {
	mu      sync.RWMutex
	buffer  []domain.DebugLogEntry
	cap     int
	subs    map[int]chan domain.DebugLogEntry
	nextSub int
}

var _ domain.LogStream = (*Hub)(nil)

// NewHub creates a hub with the given ring buffer capacity.
func NewHub(capacity int) *Hub {
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	return &Hub{
		cap:  capacity,
		subs: make(map[int]chan domain.DebugLogEntry),
	}
}

var defaultHub = NewHub(defaultCapacity)

// Default returns the process-wide log hub.
func Default() *Hub {
	return defaultHub
}

// Publish appends an entry and notifies subscribers.
func (h *Hub) Publish(e domain.DebugLogEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.appendLocked(e)
	h.broadcastLocked(e)
}

func (h *Hub) appendLocked(e domain.DebugLogEntry) {
	h.buffer = append(h.buffer, e)
	if len(h.buffer) > h.cap {
		h.buffer = h.buffer[len(h.buffer)-h.cap:]
	}
}

func (h *Hub) broadcastLocked(e domain.DebugLogEntry) {
	for _, ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Snapshot returns a copy of the current ring buffer.
func (h *Hub) Snapshot() []domain.DebugLogEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]domain.DebugLogEntry, len(h.buffer))
	copy(out, h.buffer)
	return out
}

// Subscribe returns a subscription id, backlog snapshot, and a channel of live entries.
func (h *Hub) Subscribe(buffer int) (id int, backlog []domain.DebugLogEntry, ch <-chan domain.DebugLogEntry) {
	if buffer <= 0 {
		buffer = 64
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	id = h.nextSub
	h.nextSub++
	backlog = make([]domain.DebugLogEntry, len(h.buffer))
	copy(backlog, h.buffer)
	c := make(chan domain.DebugLogEntry, buffer)
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
