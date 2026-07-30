package usecase

import (
	"sync"
	"time"

	"xquakshell/internal/domain/discovery"
)

// Pace control for discovery, both directions in one file because they are one concern with two
// ends: how fast a plugin may push work at the host, and how fast the host may push redraws at the
// frontend. They are separate limits because they bound different costs — snapshot processing
// versus tree re-rendering — and a single number could not be right for both.
//
// Time is injected in both. A limiter tested with time.Sleep is a limiter tested by guessing, and
// the tests it produces are slow and flaky in equal measure.

// DiscoveryPublishLimiter caps inbound discovery.publish at MaxPublishPerSecond per (plugin,
// connection).
//
// Exceeding it drops the snapshot and logs; it never kills the plugin. Publishing too fast is what
// an over-eager poller does, and the level-triggered design already expects redundant publishes —
// killing the process would turn a tuning mistake into a user-visible outage, and the next observe
// would just restart it. The dropped snapshot costs nothing lasting either: the plugin re-publishes
// on its own schedule, because publish is a level, not an edge.
type DiscoveryPublishLimiter struct {
	now func() time.Time

	mu      sync.Mutex
	windows map[string]*discoveryPublishWindow
}

type discoveryPublishWindow struct {
	start time.Time
	count int
}

// NewDiscoveryPublishLimiter creates a limiter. now defaults to time.Now.
func NewDiscoveryPublishLimiter(now func() time.Time) *DiscoveryPublishLimiter {
	if now == nil {
		now = time.Now
	}
	return &DiscoveryPublishLimiter{now: now, windows: make(map[string]*discoveryPublishWindow)}
}

// Allow reports whether this publish fits within the current second's budget.
func (l *DiscoveryPublishLimiter) Allow(pluginID, connectionID string) bool {
	key := discoveryPaceKey(pluginID, connectionID)
	at := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()
	window, ok := l.windows[key]
	if !ok || at.Sub(window.start) >= time.Second {
		l.windows[key] = &discoveryPublishWindow{start: at, count: 1}
		return true
	}
	window.count++
	return window.count <= discovery.MaxPublishPerSecond
}

// Forget drops a pair's window, so a closed connection or an uninstalled plugin does not leave an
// entry behind for the process lifetime.
func (l *DiscoveryPublishLimiter) Forget(pluginID, connectionID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.windows, discoveryPaceKey(pluginID, connectionID))
}

// DiscoveryEmitCoalescer limits frontend notifications to one per CoalesceInterval per node.
//
// It coalesces rather than merely throttles: an update that arrives inside a closed window is not
// discarded but deferred, and fires once the window opens. The difference matters because publishes
// are level-triggered — a plugin streaming enumeration progress ends with the state that matters,
// and a plain drop would land on the floor, leaving the user staring at an intermediate tree with
// nothing left to correct it.
//
// Both the clock and the timer are injected so a test can open the window on demand rather than
// sleeping through it.
type DiscoveryEmitCoalescer struct {
	now   func() time.Time
	after func(time.Duration, func())
	emit  func(connectionID string)

	mu      sync.Mutex
	last    map[string]time.Time
	pending map[string]struct{}
}

// NewDiscoveryEmitCoalescer creates a coalescer that calls emit with the connection whose tree
// changed. now defaults to time.Now and after to time.AfterFunc; emit may be nil while the
// presentation layer that consumes these events is not wired.
func NewDiscoveryEmitCoalescer(emit func(connectionID string), now func() time.Time, after func(time.Duration, func())) *DiscoveryEmitCoalescer {
	if now == nil {
		now = time.Now
	}
	if after == nil {
		after = func(d time.Duration, fn func()) { time.AfterFunc(d, fn) }
	}
	return &DiscoveryEmitCoalescer{
		now:     now,
		after:   after,
		emit:    emit,
		last:    make(map[string]time.Time),
		pending: make(map[string]struct{}),
	}
}

// Submit records that a node's branch changed and emits now, or schedules one trailing emit for
// when the node's window reopens.
func (c *DiscoveryEmitCoalescer) Submit(connectionID, nodeID string) {
	key := discoveryPaceKey(connectionID, nodeID)
	at := c.now()

	c.mu.Lock()
	last, seen := c.last[key]
	elapsed := at.Sub(last)
	if seen && elapsed < discovery.CoalesceInterval {
		if _, already := c.pending[key]; already {
			// A trailing emit is already booked; this update rides along on it. Booking a second
			// timer would emit twice for one window, which is the churn this type exists to stop.
			c.mu.Unlock()
			return
		}
		c.pending[key] = struct{}{}
		c.mu.Unlock()
		c.after(discovery.CoalesceInterval-elapsed, func() { c.flush(key, connectionID) })
		return
	}
	c.last[key] = at
	c.mu.Unlock()
	c.notify(connectionID)
}

func (c *DiscoveryEmitCoalescer) flush(key, connectionID string) {
	c.mu.Lock()
	delete(c.pending, key)
	c.last[key] = c.now()
	c.mu.Unlock()
	c.notify(connectionID)
}

// notify calls the frontend callback outside the lock. Holding it across an emit would put an
// unknown amount of presentation work inside this type's critical section.
func (c *DiscoveryEmitCoalescer) notify(connectionID string) {
	if c.emit != nil {
		c.emit(connectionID)
	}
}

func discoveryPaceKey(left, right string) string { return left + "\x00" + right }
