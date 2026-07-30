package usecase

import (
	"log/slog"
	"sync"
)

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
	store     *DiscoveryStore
	observer  *DiscoveryObserver
	pace      *DiscoveryPace
	onChange  func(connectionID, nodeID string)

	mu          sync.Mutex
	conns       map[string]*discoveryConnection
	sessionConn map[string]string
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
func NewDiscoveryLeader(protocols DiscoverySessionProtocols, store *DiscoveryStore, observer *DiscoveryObserver, pace *DiscoveryPace, onChange func(connectionID, nodeID string)) *DiscoveryLeader {
	return &DiscoveryLeader{
		protocols:   protocols,
		store:       store,
		observer:    observer,
		pace:        pace,
		onChange:    onChange,
		conns:       make(map[string]*discoveryConnection),
		sessionConn: make(map[string]string),
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
		l.observer.ConnectionChanged(connectionID)
	}
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
		l.store.ClearConnection(connectionID)
		l.observer.ClearConnection(connectionID)
		// Pace state is keyed by connection too, and nothing else would ever drop it: without this
		// a long-lived process accumulates one window per (plugin, connection) and per node it ever
		// rendered, for connections that closed hours ago.
		l.pace.ForgetConnection(connectionID)
	case wasLeader:
		slog.Info("discovery: leading session handover",
			"component", "discovery", "connectionId", connectionID)
		l.store.MarkConnectionStale(connectionID)
		l.observer.ConnectionChanged(connectionID)
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
