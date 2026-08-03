package usecase

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"sort"
	"sync"
)

// discoveryObserveMethod is the host->plugin notification carrying the full observed set.
const discoveryObserveMethod = "discovery.observe"

// DiscoveryNotifier sends a host->plugin notification. Satisfied by *PluginManager.
type DiscoveryNotifier interface {
	Notify(ctx context.Context, pluginID, method string, params json.RawMessage) error
}

// DiscoveryPluginTarget is one plugin the host may address discovery notifications to.
type DiscoveryPluginTarget struct {
	PluginID        string
	ParentProtocols []string
}

// DiscoveryPluginLookup lists the installed plugins that declared capabilities.discovery.
// Satisfied by *PluginRegistry.
type DiscoveryPluginLookup interface {
	DiscoveryPlugins() []DiscoveryPluginTarget
}

// DiscoveryLeaderLookup resolves a connection to the session its discovery traffic rides on. It is
// the only thing the observer knows about sessions, and it is an interface so that the one place
// that maps connections to sessions stays discovery_leader.go.
type DiscoveryLeaderLookup interface {
	Leading(connectionID string) (sessionID string, protocol string, ok bool)
	ConnectionForSession(sessionID string) (connectionID string, ok bool)
	Connections() []string
}

// DiscoveryObserver owns the set of expanded nodes per connection, and nothing else. It holds no
// tree state: the store owns that, and duplicating even part of it here would create the second
// owner ADR-009 exists to prevent.
//
// The set is a level, not a stream of edges (ADR-014 "data flow"). Every change resends the whole
// set, and so does every plugin (re)start — that resend is the entire point of the design: a
// crashed plugin comes back and is told what to watch without anyone having to remember which
// expand events it missed while it was gone.
type DiscoveryObserver struct {
	plugins  DiscoveryPluginLookup
	notifier DiscoveryNotifier

	mu       sync.Mutex
	leader   DiscoveryLeaderLookup
	observed map[string]*discoveryObservedSet
	// sending serializes outbound observes per connection. It is a SECOND lock, deliberately, and
	// the only one held across a Notify: mu guards state and must never be held across an IPC call
	// (ADR-009), but with nothing serializing the sends themselves, two goroutines that each read
	// the set and then each send can deliver them in the opposite order, leaving a plugin watching
	// a set the host has already moved on from. Per connection rather than global, so one plugin
	// blocking in Notify cannot stall observes for every other connection.
	sending map[string]*sync.Mutex
}

// discoveryObservedSet is one connection's expanded nodes plus a version that increments on every
// change, and the highest version actually delivered. Reading the set and its version under one
// acquisition is what lets a send that lost the race be dropped rather than overwrite a newer one.
type discoveryObservedSet struct {
	nodeIDs map[string]struct{}
	version uint64
	sent    uint64
}

// NewDiscoveryObserver creates an observer. The leader is bound afterwards with SetLeader because
// the leader needs the observer too — the same late-binding shape SessionLifecycleService uses for
// the channel bus.
func NewDiscoveryObserver(plugins DiscoveryPluginLookup, notifier DiscoveryNotifier) *DiscoveryObserver {
	return &DiscoveryObserver{
		plugins:  plugins,
		notifier: notifier,
		observed: make(map[string]*discoveryObservedSet),
		sending:  make(map[string]*sync.Mutex),
	}
}

// SetLeader binds the connection->session resolver after construction.
func (o *DiscoveryObserver) SetLeader(leader DiscoveryLeaderLookup) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.leader = leader
}

// SetObserved replaces a connection's observed set with the full set of currently expanded nodes.
// "" denotes the connection root. It is a full set and never a delta: the frontend states what is
// expanded now, and the host does not try to reconstruct that from a history of clicks.
func (o *DiscoveryObserver) SetObserved(connectionID string, nodeIDs []string) {
	o.mu.Lock()
	ids := make(map[string]struct{}, len(nodeIDs))
	for _, id := range nodeIDs {
		ids[id] = struct{}{}
	}
	set := o.setLocked(connectionID)
	set.nodeIDs = ids
	set.version++
	o.mu.Unlock()
	o.broadcast(connectionID)
}

// Retain drops removed node IDs from a connection's observed set and, if any were actually in it,
// resends the set. This is the other half of ApplySnapshot's removed list: a cascade delete takes
// grandchildren with it, and the host must stop asking plugins to watch nodes that no longer exist
// before those IDs can be reused by something else.
func (o *DiscoveryObserver) Retain(connectionID string, removed []string) {
	o.mu.Lock()
	set, ok := o.observed[connectionID]
	changed := false
	if ok {
		for _, id := range removed {
			if _, watched := set.nodeIDs[id]; watched {
				delete(set.nodeIDs, id)
				changed = true
			}
		}
		if changed {
			set.version++
		}
	}
	o.mu.Unlock()
	if changed {
		o.broadcast(connectionID)
	}
}

// IsObserved reports whether a node is currently expanded. A publish for anything else is dropped
// silently by the caller: a plugin that was mid-enumeration when the user collapsed a branch is
// behaving correctly, and there is nothing to report.
func (o *DiscoveryObserver) IsObserved(connectionID, nodeID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	set, ok := o.observed[connectionID]
	if !ok {
		return false
	}
	_, watched := set.nodeIDs[nodeID]
	return watched
}

// ConnectionChanged resends the current set for a connection, called when the session carrying the
// traffic changes so the new leader's plugins learn what to watch.
//
// It bumps the version even though the contents did not change: what is stale here is the delivery,
// not the set, and a broadcast that skipped an already-delivered version would leave the new leader
// knowing nothing.
func (o *DiscoveryObserver) ConnectionChanged(connectionID string) {
	o.mu.Lock()
	o.setLocked(connectionID).version++
	o.mu.Unlock()
	o.broadcast(connectionID)
}

// ClearConnection forgets a connection's observed set. No notification follows: this runs when the
// last ready session is gone, so there is no transport left to notify over.
func (o *DiscoveryObserver) ClearConnection(connectionID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.observed, connectionID)
	delete(o.sending, connectionID)
}

// PluginStarted resends every connection's observed set to one plugin that has just (re)started.
// Without this the level-triggered design collapses into an edge-triggered one: a plugin that
// restarted would sit idle, holding no record of what the user has expanded, and the branches it
// used to fill would stay empty until the user collapsed and re-expanded each one by hand.
func (o *DiscoveryObserver) PluginStarted(pluginID string) {
	o.mu.Lock()
	leader := o.leader
	o.mu.Unlock()
	if leader == nil {
		return
	}
	for _, connectionID := range leader.Connections() {
		sessionID, protocol, ok := leader.Leading(connectionID)
		if !ok {
			continue
		}
		for _, target := range o.plugins.DiscoveryPlugins() {
			if target.PluginID != pluginID || !discoveryAddressable(target, protocol) {
				continue
			}
			// Under the connection's send gate, reading the set inside it, so this resend cannot
			// interleave with a broadcast and hand one plugin an older set than its peers. It
			// leaves the delivered-version mark alone: reaching one plugin is not delivery.
			gate := o.sendGate(connectionID)
			gate.Lock()
			o.send(target.PluginID, sessionID, o.nodeIDs(connectionID))
			gate.Unlock()
		}
	}
}

// broadcast sends the connection's current set to every plugin the connection is addressable to.
//
// The set is never captured before the send gate is taken. Reading it inside the gate is what makes
// the last write win: whichever caller enters first publishes whatever the set says at that moment,
// and any caller queued behind it finds that version already delivered and says nothing. The
// alternative — capture the set, queue, then send — is exactly the interleaving that leaves a plugin
// watching a set the host abandoned.
//
// mu is still never held across Notify. The gate orders sends; mu guards state. Two locks with two
// jobs, and only the one that touches no state is held across IPC (ADR-009).
func (o *DiscoveryObserver) broadcast(connectionID string) {
	gate := o.sendGate(connectionID)
	gate.Lock()
	defer gate.Unlock()

	o.mu.Lock()
	leader := o.leader
	set, tracked := o.observed[connectionID]
	if leader == nil || !tracked || set.version <= set.sent {
		o.mu.Unlock()
		return
	}
	version := set.version
	nodeIDs := sortedNodeIDs(set.nodeIDs)
	o.mu.Unlock()

	sessionID, protocol, live := leader.Leading(connectionID)
	if !live {
		// No ready session: nothing to address this to. The version stays unmarked, so the next
		// leader still gets the set through ConnectionChanged.
		return
	}
	for _, target := range o.plugins.DiscoveryPlugins() {
		if !discoveryAddressable(target, protocol) {
			continue
		}
		o.send(target.PluginID, sessionID, nodeIDs)
	}

	o.mu.Lock()
	if current, ok := o.observed[connectionID]; ok && current.sent < version {
		current.sent = version
	}
	o.mu.Unlock()
}

// sendGate returns the connection's send lock, creating it on first use.
func (o *DiscoveryObserver) sendGate(connectionID string) *sync.Mutex {
	o.mu.Lock()
	defer o.mu.Unlock()
	gate, ok := o.sending[connectionID]
	if !ok {
		gate = &sync.Mutex{}
		o.sending[connectionID] = gate
	}
	return gate
}

// setLocked returns the connection's observed set, creating an empty one on first use. Callers must
// hold mu.
func (o *DiscoveryObserver) setLocked(connectionID string) *discoveryObservedSet {
	set, ok := o.observed[connectionID]
	if !ok {
		set = &discoveryObservedSet{nodeIDs: make(map[string]struct{})}
		o.observed[connectionID] = set
	}
	return set
}

func (o *DiscoveryObserver) nodeIDs(connectionID string) []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	set, ok := o.observed[connectionID]
	if !ok {
		return nil
	}
	return sortedNodeIDs(set.nodeIDs)
}

func sortedNodeIDs(set map[string]struct{}) []string {
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	// Sorted so the payload is deterministic: the same observed set produces the same bytes,
	// which keeps logs and tests readable and makes a genuine change visible.
	sort.Strings(ids)
	return ids
}

// discoveryObservePayload is the discovery.observe wire shape. sessionId appears here and nowhere
// nearer the frontend: it is the plugin's transport address, not an identity anything else uses.
type discoveryObservePayload struct {
	SessionID string   `json:"sessionId"`
	NodeIDs   []string `json:"nodeIds"`
}

func (o *DiscoveryObserver) send(pluginID, sessionID string, nodeIDs []string) {
	if o.notifier == nil {
		return
	}
	if nodeIDs == nil {
		nodeIDs = []string{}
	}
	params, err := json.Marshal(discoveryObservePayload{SessionID: sessionID, NodeIDs: nodeIDs})
	if err != nil {
		slog.Error("discovery: encode observe failed", "component", "discovery", "pluginId", pluginID, "err", err)
		return
	}
	if err := o.notifier.Notify(context.Background(), pluginID, discoveryObserveMethod, params); err != nil {
		// A plugin that is not running cannot be told anything, and does not need to be: it will
		// be told the full set again the moment it starts (PluginStarted).
		slog.Debug("discovery: observe not delivered", "component", "discovery", "pluginId", pluginID, "err", err)
	}
}

// discoveryAddressable reports whether a plugin may be told about a connection of this protocol.
// parentProtocols is a real addressing filter, not documentation: a plugin that declared "ssh" is
// never asked about a connection protocol it never claimed to understand (ADR-014 "manifest").
func discoveryAddressable(target DiscoveryPluginTarget, protocol string) bool {
	if protocol == "" {
		return false
	}
	return slices.Contains(target.ParentProtocols, protocol)
}
