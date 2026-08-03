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

// discoveryPaceKey identifies one budgeted thing. It is a struct rather than a joined string so
// that forgetting everything belonging to a connection is an exact field comparison, not a prefix
// match over keys whose other half is plugin- or node-supplied.
type discoveryPaceKey struct {
	connectionID string
	item         string
}

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
	windows map[discoveryPaceKey]*discoveryPublishWindow
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
	return &DiscoveryPublishLimiter{now: now, windows: make(map[discoveryPaceKey]*discoveryPublishWindow)}
}

// Allow reports whether this publish fits within the current second's budget.
func (l *DiscoveryPublishLimiter) Allow(pluginID, connectionID string) bool {
	key := discoveryPaceKey{connectionID: connectionID, item: pluginID}
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

// ForgetPlugin drops every window belonging to one plugin, across all connections. It is the
// counterpart of a plugin being stopped rather than a connection closing: leaving a half-spent
// budget behind would throttle the first publishes the plugin makes after being started again.
//
// It sweeps by plugin rather than taking a (plugin, connection) pair because a window can exist for
// a connection the plugin never successfully drew under: a publish refused for a collapsed branch
// or a non-leading session is counted before it is refused, on purpose — the budget exists to bound
// work the host does on the plugin's behalf, and deciding to drop a snapshot is some of that work.
// Only the limiter's own keys can enumerate those.
func (l *DiscoveryPublishLimiter) ForgetPlugin(pluginID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key := range l.windows {
		if key.item == pluginID {
			delete(l.windows, key)
		}
	}
}

// ForgetConnection drops every window belonging to a connection, so a closed connection does not
// leave one entry per plugin behind for the lifetime of the process.
func (l *DiscoveryPublishLimiter) ForgetConnection(connectionID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key := range l.windows {
		if key.connectionID == connectionID {
			delete(l.windows, key)
		}
	}
}

// DiscoveryEmitCoalescer limits frontend notifications to one per CoalesceInterval per node.
//
// It coalesces rather than merely throttles: an update that arrives inside a closed window is not
// discarded but deferred, and fires once the window opens. The difference matters because publishes
// are level-triggered — a plugin streaming enumeration progress ends with the state that matters,
// and a plain drop would land on the floor, leaving the user staring at an intermediate tree with
// nothing left to correct it.
//
// The emit names the node, not just the connection. Windows are per node, so a connection whose
// branches are all filling at once produces one emit per branch; without the node a reader could
// only redraw the whole tree each time, and the interval would end up bounding the number of events
// rather than the amount of re-rendering it exists to bound.
//
// Both the clock and the timer are injected so a test can open the window on demand rather than
// sleeping through it.
type DiscoveryEmitCoalescer struct {
	now   func() time.Time
	after func(time.Duration, func())
	emit  func(connectionID, nodeID string)

	mu      sync.Mutex
	last    map[discoveryPaceKey]time.Time
	pending map[discoveryPaceKey]struct{}
}

// NewDiscoveryEmitCoalescer creates a coalescer that calls emit with the connection and the node
// whose branch changed. now defaults to time.Now and after to time.AfterFunc; emit may be nil while
// the presentation layer that consumes these events is not wired.
func NewDiscoveryEmitCoalescer(emit func(connectionID, nodeID string), now func() time.Time, after func(time.Duration, func())) *DiscoveryEmitCoalescer {
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
		last:    make(map[discoveryPaceKey]time.Time),
		pending: make(map[discoveryPaceKey]struct{}),
	}
}

// Submit records that a node's branch changed and emits now, or schedules one trailing emit for
// when that node's window reopens.
func (c *DiscoveryEmitCoalescer) Submit(connectionID, nodeID string) {
	key := discoveryPaceKey{connectionID: connectionID, item: nodeID}
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
		c.after(discovery.CoalesceInterval-elapsed, func() { c.flush(key) })
		return
	}
	c.last[key] = at
	c.mu.Unlock()
	c.notify(connectionID, nodeID)
}

// ForgetConnection drops every window belonging to a connection. A trailing emit already booked for
// a forgotten key still fires, and harmlessly: it reports a change on a connection whose tree is
// gone, which is precisely what a reader still showing that tree needs to hear.
func (c *DiscoveryEmitCoalescer) ForgetConnection(connectionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.last {
		if key.connectionID == connectionID {
			delete(c.last, key)
		}
	}
	for key := range c.pending {
		if key.connectionID == connectionID {
			delete(c.pending, key)
		}
	}
}

func (c *DiscoveryEmitCoalescer) flush(key discoveryPaceKey) {
	c.mu.Lock()
	delete(c.pending, key)
	c.last[key] = c.now()
	c.mu.Unlock()
	c.notify(key.connectionID, key.item)
}

// notify calls the frontend callback outside the lock. Holding it across an emit would put an
// unknown amount of presentation work inside this type's critical section.
func (c *DiscoveryEmitCoalescer) notify(connectionID, nodeID string) {
	if c.emit != nil {
		c.emit(connectionID, nodeID)
	}
}

// DiscoveryPace bundles the two controls so that everything a connection accrues in the name of
// pacing has exactly one place to be forgotten.
//
// They were two fields on two different collaborators before, and the result was a ForgetConnection
// that nothing called: a teardown had to remember two objects, so it remembered neither.
type DiscoveryPace struct {
	limiter   *DiscoveryPublishLimiter
	coalescer *DiscoveryEmitCoalescer
}

// NewDiscoveryPace bundles a limiter and a coalescer.
func NewDiscoveryPace(limiter *DiscoveryPublishLimiter, coalescer *DiscoveryEmitCoalescer) *DiscoveryPace {
	return &DiscoveryPace{limiter: limiter, coalescer: coalescer}
}

// AllowPublish reports whether an inbound snapshot fits the plugin's budget for this connection.
func (p *DiscoveryPace) AllowPublish(pluginID, connectionID string) bool {
	return p.limiter.Allow(pluginID, connectionID)
}

// Emit reports that a node's branch changed, subject to coalescing.
func (p *DiscoveryPace) Emit(connectionID, nodeID string) {
	p.coalescer.Submit(connectionID, nodeID)
}

// ForgetPlugin drops one plugin's publish budget everywhere. The coalescer is untouched on purpose:
// its windows are keyed by node, and a node belongs to whichever plugin published it, so there is
// nothing plugin-shaped for it to forget.
func (p *DiscoveryPace) ForgetPlugin(pluginID string) {
	p.limiter.ForgetPlugin(pluginID)
}

// ForgetConnection drops all pace state for a connection.
func (p *DiscoveryPace) ForgetConnection(connectionID string) {
	p.limiter.ForgetConnection(connectionID)
	p.coalescer.ForgetConnection(connectionID)
}
