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
	observed map[string]map[string]struct{}
}

// NewDiscoveryObserver creates an observer. The leader is bound afterwards with SetLeader because
// the leader needs the observer too — the same late-binding shape SessionLifecycleService uses for
// the channel bus.
func NewDiscoveryObserver(plugins DiscoveryPluginLookup, notifier DiscoveryNotifier) *DiscoveryObserver {
	return &DiscoveryObserver{
		plugins:  plugins,
		notifier: notifier,
		observed: make(map[string]map[string]struct{}),
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
	set := make(map[string]struct{}, len(nodeIDs))
	for _, id := range nodeIDs {
		set[id] = struct{}{}
	}
	o.observed[connectionID] = set
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
			if _, watched := set[id]; watched {
				delete(set, id)
				changed = true
			}
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
	_, ok := o.observed[connectionID][nodeID]
	return ok
}

// ConnectionChanged resends the current set for a connection. Called when the session carrying the
// traffic changes, so the new leader's plugins learn what to watch.
func (o *DiscoveryObserver) ConnectionChanged(connectionID string) {
	o.broadcast(connectionID)
}

// ClearConnection forgets a connection's observed set. No notification follows: this runs when the
// last ready session is gone, so there is no transport left to notify over.
func (o *DiscoveryObserver) ClearConnection(connectionID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.observed, connectionID)
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
			o.send(target.PluginID, sessionID, o.nodeIDs(connectionID))
		}
	}
}

// broadcast sends the connection's current set to every plugin the connection is addressable to.
//
// The observed set is read under the lock and the notifications go out after it is released. That
// ordering is not incidental: Notify crosses an IPC boundary into another process, and holding a
// state lock across it is how this codebase has produced deadlocks before (ADR-009).
func (o *DiscoveryObserver) broadcast(connectionID string) {
	o.mu.Lock()
	leader := o.leader
	o.mu.Unlock()
	if leader == nil {
		return
	}
	sessionID, protocol, ok := leader.Leading(connectionID)
	if !ok {
		// No ready session: nothing to address the notification to. The set survives, and the next
		// leader gets it through ConnectionChanged.
		return
	}
	nodeIDs := o.nodeIDs(connectionID)
	for _, target := range o.plugins.DiscoveryPlugins() {
		if !discoveryAddressable(target, protocol) {
			continue
		}
		o.send(target.PluginID, sessionID, nodeIDs)
	}
}

func (o *DiscoveryObserver) nodeIDs(connectionID string) []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	set := o.observed[connectionID]
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
