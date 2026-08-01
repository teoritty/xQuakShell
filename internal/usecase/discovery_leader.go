package usecase

import (
	"context"
	"log/slog"
	"sort"
	"sync"

	"xquakshell/internal/pkg/safego"
)

// DiscoveryPluginRuntime starts and authorizes the plugin processes a connection's discovery
// traffic rides on. Satisfied by *PluginManager.
//
// It exists because a discovery plugin is bound to a session nobody else would ever bind it to. The
// session bridge binds a plugin to the session it OWNS — the plugin that provides the protocol —
// and a discovery plugin owns nothing: it draws a subtree under a core SSH connection it did not
// create. Without this, every discovery.publish it sent would be refused by the IDOR check in
// PluginSessionRPCHandler, and the subtree would never appear.
type DiscoveryPluginRuntime interface {
	EnsureRunning(ctx context.Context, pluginID string) error
	BindSession(pluginID, sessionID string) error
	UnbindSession(pluginID, sessionID string)
}

// discoveryBinding is one (plugin, session) authorization this leader created and is therefore
// responsible for releasing.
type discoveryBinding struct {
	pluginID  string
	sessionID string
}

// DiscoverySessionTracker is what session lifecycle sees of discovery: two calls, no return
// values, no tree. Late-bound into SessionLifecycleService exactly like ChannelSessionCloser.
type DiscoverySessionTracker interface {
	SessionReady(sessionID, connectionID string)
	SessionClosed(sessionID, connectionID string)
}

// DiscoverySessionProtocols reports a live session's connection protocol. Satisfied by
// *SessionRegistry.
type DiscoverySessionProtocols interface {
	ProtocolForSession(sessionID string) (string, bool)
}

// DiscoveryLeader picks which session carries a connection's discovery traffic, and is the ONLY
// place in the codebase where a connectionID and a sessionID meet.
//
// That confinement is the point. A plugin can only enumerate resources through an authenticated
// transport, so it must be handed a sessionID; everything the user sees is a connection, of which
// there is one subtree however many tabs are open. Every other file in the discovery usecase deals
// in connectionIDs alone, and the frontend never learns that sessions exist at all (ADR-014).
//
// The leader is the earliest ready session, ordered by arrival rather than by session ID, since
// IDs are random and carry no ordering.
type DiscoveryLeader struct {
	protocols DiscoverySessionProtocols
	plugins   DiscoveryPluginLookup
	runtime   DiscoveryPluginRuntime
	store     *DiscoveryStore
	observer  *DiscoveryObserver
	pace      *DiscoveryPace
	onChange  func(connectionID, nodeID string)

	mu          sync.Mutex
	conns       map[string]*discoveryConnection
	sessionConn map[string]string
	// bound records the authorizations this leader created, per connection, so the same code that
	// granted them is the code that takes them back. Nothing else in the app knows they exist.
	bound map[string][]discoveryBinding
	// reconciling tracks in-flight reconciliations. It is registered BEFORE the goroutine starts, so
	// a caller that has returned from SessionReady or SessionClosed knows the work is either done or
	// counted — which is what lets a test wait for it instead of polling, and what a shutdown would
	// need if this ever grows one.
	reconciling sync.WaitGroup
	// binding serializes reconciliation per connection. It is a second lock for the same reason the
	// observer keeps one: mu guards state and must never be held across an IPC call (ADR-009),
	// while starting and binding plugin processes must not interleave for one connection.
	binding map[string]*sync.Mutex
}

type discoveryConnection struct {
	protocol string
	// ready holds the connection's ready session IDs in arrival order, so the head is always the
	// earliest — the leader. Append-and-remove keeps that invariant without storing a sequence
	// number nobody would otherwise read.
	ready []string
}

// NewDiscoveryLeader creates a leader tracker. onChange may be nil while the presentation layer is
// not wired; it is called when a handover or teardown changes what the frontend should render, with
// "" for the node — a whole-connection change, addressed at the connection root like everywhere
// else in this package. It bypasses the coalescer deliberately: a teardown deferred behind a 100 ms
// window would leave the user looking at a tree that no longer exists.
//
// plugins and runtime are constructor arguments rather than optional setters on purpose: a leader
// missing either one compiles, runs, and silently authorizes nobody, which is exactly the failure
// this signature exists to make impossible to write by accident.
func NewDiscoveryLeader(protocols DiscoverySessionProtocols, plugins DiscoveryPluginLookup, runtime DiscoveryPluginRuntime, store *DiscoveryStore, observer *DiscoveryObserver, pace *DiscoveryPace, onChange func(connectionID, nodeID string)) *DiscoveryLeader {
	return &DiscoveryLeader{
		protocols:   protocols,
		plugins:     plugins,
		runtime:     runtime,
		store:       store,
		observer:    observer,
		pace:        pace,
		onChange:    onChange,
		conns:       make(map[string]*discoveryConnection),
		sessionConn: make(map[string]string),
		bound:       make(map[string][]discoveryBinding),
		binding:     make(map[string]*sync.Mutex),
	}
}

// SessionReady records a session that has reached ready state. If it becomes the connection's
// leader, the current observed set is sent to it — a level resend, not a special case: a brand new
// connection simply has an empty set to send.
func (l *DiscoveryLeader) SessionReady(sessionID, connectionID string) {
	protocol := ""
	if l.protocols != nil {
		protocol, _ = l.protocols.ProtocolForSession(sessionID)
	}

	l.mu.Lock()
	conn, ok := l.conns[connectionID]
	if !ok {
		conn = &discoveryConnection{}
		l.conns[connectionID] = conn
	}
	if protocol != "" {
		conn.protocol = protocol
	}
	if indexOfReadySession(conn.ready, sessionID) >= 0 {
		l.mu.Unlock()
		return
	}
	conn.ready = append(conn.ready, sessionID)
	l.sessionConn[sessionID] = connectionID
	leads := conn.ready[0] == sessionID
	l.mu.Unlock()

	if leads {
		// No ConnectionChanged here. It used to run alongside the reconciliation, and if the plugin
		// process happened to be up already the observe reached it BEFORE it held a binding — the
		// publish it triggered was refused with -32001, and nothing replayed it, because broadcast had
		// already marked that version delivered. The observe now goes out at the end of the
		// reconciliation, once the authorizations it depends on exist.
		l.reconcileBindings(connectionID, true)
	}
}

// reconcileBindings brings one connection's plugin authorizations in line with its CURRENT leading
// session, on a background goroutine.
//
// Level, not edge, like everything else here: it is handed a connection and nothing else, and works
// out both what to grant and what to release from the state at the moment it runs. Two events
// racing therefore cannot leave a stale grant behind — whichever reconciliation runs last is the
// one that decides, and it decides from the truth rather than from the event that woke it.
//
// It runs in the background because starting a plugin process spawns a child and handshakes with
// it. SessionReady is on the path that reports a connection as usable, and a plugin that takes ten
// seconds to come up must not delay that by ten seconds.
// thenObserve says whether the connection's observed set must be resent once the authorizations are
// in place. It is false only on teardown, where there is no transport left to send it over and
// where re-creating the observed set would resurrect the very state the caller is about to delete.
func (l *DiscoveryLeader) reconcileBindings(connectionID string, thenObserve bool) {
	if l.runtime == nil || l.plugins == nil {
		// Nothing to authorize, but the observe is still owed: without it a leader with no plugin
		// runtime wired — every test that does not care about processes — would stop telling plugins
		// anything at all.
		if thenObserve {
			l.observer.ConnectionChanged(connectionID)
		}
		return
	}
	l.reconciling.Add(1)
	safego.GoNamed("discovery.bindings", func() {
		defer l.reconciling.Done()
		l.syncBindings(connectionID)
		if thenObserve {
			l.observer.ConnectionChanged(connectionID)
		}
	})
}

// syncBindings is reconcileBindings' body, called directly by tests that need it to have finished.
func (l *DiscoveryLeader) syncBindings(connectionID string) {
	gate := l.bindGate(connectionID)
	gate.Lock()
	defer gate.Unlock()

	desired := l.desiredBindings(connectionID)

	l.mu.Lock()
	previous := l.bound[connectionID]
	l.mu.Unlock()

	// Released FIRST, before anything new is granted. A plugin still holding a binding for a session
	// that stopped leading may publish into it, which is precisely the IDOR the gate exists to
	// refuse — and this is the one place in the app that could hand it out.
	kept := make([]discoveryBinding, 0, len(desired))
	for _, existing := range previous {
		if containsBinding(desired, existing) {
			kept = append(kept, existing)
			continue
		}
		l.runtime.UnbindSession(existing.pluginID, existing.sessionID)
	}

	for _, want := range desired {
		if containsBinding(kept, want) {
			continue
		}
		// Authorized BEFORE it is started, and the order is load-bearing. Starting a plugin fires the
		// host's process-started hook, which replays the observed set to it synchronously — so a
		// plugin started first receives an observe it is not yet allowed to answer, publishes, and is
		// refused with -32001. Binding first costs nothing: the authorization is a record in the
		// session authorizer and needs no process to exist.
		if err := l.runtime.BindSession(want.pluginID, want.sessionID); err != nil {
			slog.Warn("discovery: could not authorize a plugin for this connection's session",
				"component", "discovery", "pluginId", want.pluginID, "connectionId", connectionID, "err", err)
			continue
		}
		// context.Background(), deliberately, with no deadline attached. The context handed to
		// EnsureRunning reaches exec.CommandContext, so it owns the CHILD PROCESS's lifetime, not
		// just the start call: cancelling it — including the cancel that a WithTimeout requires on
		// the success path — kills the plugin the moment it has finished starting. The start call
		// itself is already bounded by the host's own handshake timeout.
		if err := l.runtime.EnsureRunning(context.Background(), want.pluginID); err != nil {
			// The binding goes back: an authorization held by a process that never started is a grant
			// nobody can use and nothing would take away until the session closed. Not fatal to the
			// connection — the plugin simply draws nothing, and the next reconciliation retries.
			l.runtime.UnbindSession(want.pluginID, want.sessionID)
			slog.Warn("discovery: could not start a plugin for this connection",
				"component", "discovery", "pluginId", want.pluginID, "connectionId", connectionID, "err", err)
			continue
		}
		kept = append(kept, want)
	}

	l.mu.Lock()
	if len(kept) == 0 {
		delete(l.bound, connectionID)
	} else {
		l.bound[connectionID] = kept
	}
	l.mu.Unlock()
}

// desiredBindings is every (plugin, session) authorization this connection should currently hold:
// one per discovery plugin that declared the connection's protocol, all naming the leading session.
// A connection with no ready session wants none, which is what makes teardown fall out of the same
// reconciliation rather than needing a path of its own.
func (l *DiscoveryLeader) desiredBindings(connectionID string) []discoveryBinding {
	sessionID, protocol, live := l.Leading(connectionID)
	if !live || protocol == "" {
		return nil
	}
	var want []discoveryBinding
	for _, target := range l.plugins.DiscoveryPlugins() {
		if !discoveryAddressable(target, protocol) {
			continue
		}
		want = append(want, discoveryBinding{pluginID: target.PluginID, sessionID: sessionID})
	}
	// Sorted because DiscoveryPlugins reads a map: without this, which plugin is started first
	// varies per run, and so does the order of a failure log during an incident.
	sort.Slice(want, func(i, j int) bool { return want[i].pluginID < want[j].pluginID })
	return want
}

// bindGate returns the connection's reconciliation lock, creating it on first use. Unlike the
// observer's send gate it is never deleted: a connection that closes and reopens must reuse the
// same lock, or the reconciliation releasing the old session's bindings could interleave with the
// one granting the new session's. One mutex per connection ever opened is a cheaper price than that
// window.
func (l *DiscoveryLeader) bindGate(connectionID string) *sync.Mutex {
	l.mu.Lock()
	defer l.mu.Unlock()
	gate, ok := l.binding[connectionID]
	if !ok {
		gate = &sync.Mutex{}
		l.binding[connectionID] = gate
	}
	return gate
}

func containsBinding(list []discoveryBinding, want discoveryBinding) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// SessionClosed removes a session from a connection's ready list and settles the consequences.
//
// A session that merely left ready state calls this too — there is deliberately no second entry
// point for it. "Closed" and "no longer ready" mean the same thing to discovery: the transport can
// no longer carry an enumeration, and which of the two happened would not change a single decision
// below.
//
// The two outcomes:
//   - some ready session remains: the role hands over. The tree survives, but every branch goes
//     stale, because what is on screen is the previous leader's answer and nothing has re-confirmed
//     it. The new leader's plugins get the full observed set and refill it.
//   - none remains: the tree is deleted outright. Nothing is cached (ADR-014 alternative 4) —
//     discovery reflects remote reality, and a tree with no transport behind it can no longer be
//     checked against anything.
func (l *DiscoveryLeader) SessionClosed(sessionID, connectionID string) {
	l.mu.Lock()
	conn, ok := l.conns[connectionID]
	if !ok {
		l.mu.Unlock()
		return
	}
	index := indexOfReadySession(conn.ready, sessionID)
	if index < 0 {
		l.mu.Unlock()
		return
	}
	wasLeader := index == 0
	conn.ready = append(conn.ready[:index], conn.ready[index+1:]...)
	delete(l.sessionConn, sessionID)
	abandoned := len(conn.ready) == 0
	if abandoned {
		delete(l.conns, connectionID)
	}
	l.mu.Unlock()

	switch {
	case abandoned:
		slog.Info("discovery: connection has no ready session, dropping tree",
			"component", "discovery", "connectionId", connectionID)
		// Reconciled before the state is torn down, not after: with no ready session left the desired
		// set is empty, so this is what releases every authorization the connection held. Skipping it
		// would leave plugins entitled to publish into a session that is gone. No observe follows —
		// there is no transport left to carry one.
		l.reconcileBindings(connectionID, false)
		l.store.ClearConnection(connectionID)
		l.observer.ClearConnection(connectionID)
		// Pace state is keyed by connection too, and nothing else would ever drop it: without this
		// a long-lived process accumulates one window per (plugin, connection) and per node it ever
		// rendered, for connections that closed hours ago.
		l.pace.ForgetConnection(connectionID)
	case wasLeader:
		slog.Info("discovery: leading session handover",
			"component", "discovery", "connectionId", connectionID)
		// The same reconciliation moves every binding to the new leader and drops the old one's: the
		// plugin must not keep publishing through a session that no longer leads. The observe for the
		// new leader goes out at the end of it, for the ordering reason SessionReady explains.
		l.store.MarkConnectionStale(connectionID)
		l.reconcileBindings(connectionID, true)
	default:
		// A non-leading session left. Nothing about the tree or its transport changed.
		return
	}
	l.notifyChange(connectionID)
}

func (l *DiscoveryLeader) notifyChange(connectionID string) {
	if l.onChange != nil {
		l.onChange(connectionID, "")
	}
}

// Leading returns the session a connection's discovery traffic must be addressed to, and that
// connection's protocol for the parentProtocols filter.
func (l *DiscoveryLeader) Leading(connectionID string) (string, string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	conn, ok := l.conns[connectionID]
	if !ok || len(conn.ready) == 0 {
		return "", "", false
	}
	return conn.ready[0], conn.protocol, true
}

// ConnectionForSession maps an inbound publish's sessionId back to the connection it belongs to.
func (l *DiscoveryLeader) ConnectionForSession(sessionID string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	connectionID, ok := l.sessionConn[sessionID]
	return connectionID, ok
}

// awaitReconcile blocks until every reconciliation started so far has finished. Tests only: the
// reconciliation is deliberately asynchronous, and a test that polled for its effects would be
// asserting on a timeout as much as on the behaviour.
func (l *DiscoveryLeader) awaitReconcile() {
	l.reconciling.Wait()
}

// HoldsBindings reports whether this plugin currently holds a discovery authorization on any
// connection.
//
// It is what stops the host from suspending or abandoning a plugin that is drawing a subtree
// somebody is looking at. Nothing else in the app would know: a discovery plugin serves no session
// of its own and owns no view panel, so every "is anyone using this?" check in the host answered no
// and the idle sweeper reclaimed it after five quiet minutes — quiet being the normal state of a
// tree the user has finished expanding.
func (l *DiscoveryLeader) HoldsBindings(pluginID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, bindings := range l.bound {
		for _, binding := range bindings {
			if binding.pluginID == pluginID {
				return true
			}
		}
	}
	return false
}

// Connections lists every connection with at least one ready session — what a restarted plugin
// must be re-told the observed set for.
func (l *DiscoveryLeader) Connections() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	ids := make([]string, 0, len(l.conns))
	for connectionID := range l.conns {
		ids = append(ids, connectionID)
	}
	return ids
}

func indexOfReadySession(sessions []string, sessionID string) int {
	for i, session := range sessions {
		if session == sessionID {
			return i
		}
	}
	return -1
}

var _ DiscoverySessionTracker = (*DiscoveryLeader)(nil)
var _ DiscoveryLeaderLookup = (*DiscoveryLeader)(nil)
