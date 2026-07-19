package loghub

import (
	"strconv"
	"sync"
	"time"

	"ssh-client/internal/domain"
)

const defaultCapacity = 5000

// dropMarkerSource tags the synthetic entry emitted when a subscriber falls
// behind and live entries had to be dropped.
const dropMarkerSource = "loghub"

// subscriber holds a live channel plus a counter of entries that could not be
// delivered because the channel was full. The counter is guarded by Hub.mu.
type subscriber struct {
	ch      chan domain.DebugLogEntry
	dropped int
}

// Hub stores recent log entries and fans them out to subscribers.
type Hub struct {
	mu      sync.RWMutex
	buffer  []domain.DebugLogEntry
	cap     int
	subs    map[int]*subscriber
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
		subs: make(map[int]*subscriber),
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

// broadcastLocked delivers e to every subscriber without ever blocking. When a
// subscriber's channel is full the entry is counted rather than silently lost;
// the accumulated count is surfaced as a synthetic warn marker on the next
// successful delivery so the drop is visible in the viewer.
func (h *Hub) broadcastLocked(e domain.DebugLogEntry) {
	for _, s := range h.subs {
		if s.dropped > 0 {
			select {
			case s.ch <- dropMarker(s.dropped):
				s.dropped = 0
			default:
				// Still backed up: keep counting and skip the real entry too.
				s.dropped++
				continue
			}
		}
		select {
		case s.ch <- e:
		default:
			s.dropped++
		}
	}
}

func dropMarker(n int) domain.DebugLogEntry {
	return domain.DebugLogEntry{
		Time:    time.Now(),
		Level:   "warn",
		Source:  dropMarkerSource,
		Message: "dropped " + strconv.Itoa(n) + " log entries (viewer too slow)",
		Fields:  map[string]string{"dropped": strconv.Itoa(n)},
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
	h.subs[id] = &subscriber{ch: c}
	return id, backlog, c
}

// Unsubscribe removes a subscription and closes its channel.
func (h *Hub) Unsubscribe(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.subs[id]; ok {
		delete(h.subs, id)
		close(s.ch)
	}
}
