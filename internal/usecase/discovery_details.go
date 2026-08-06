package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

// Host->plugin node details methods (ADR-015 §3).
const (
	discoveryDescribeNodeMethod = "discovery.describeNode"
	discoveryApplyDetailsMethod = "discovery.applyDetails"
	// MethodDiscoveryPublishDetails is the plugin->host half, named like the other inbound methods
	// so the gate, the dispatcher and the tests share one spelling.
	MethodDiscoveryPublishDetails = "discovery.publishDetails"
)

// ErrDiscoveryDetailsUnavailable reports that a node's owner cannot be reached right now — no
// leading session, or a plugin that did not answer.
var ErrDiscoveryDetailsUnavailable = errors.New("discovery: node details unavailable")

// DiscoveryDetailsService implements the three node-details verbs.
//
// It is a separate service from DiscoveryInvoker even though both address a node through the
// leading session, because they answer different questions: one relays an action the core cannot
// interpret, the other renders a form the core CAN validate. Sharing a type would put the field
// schema in front of the action path, which has no use for it.
type DiscoveryDetailsService struct {
	store  *DiscoveryStore
	leader DiscoveryLeaderLookup
	caller DiscoveryCaller
	audit  domainplugin.DiscoveryAuditRecorder
	caps   SurfaceCapabilityLookup
	// pace is the discovery publish budget, shared rather than duplicated: publishDetails is a
	// discovery verb naming a (plugin, connection), and giving it a second allowance would let a
	// plugin at its publish limit keep spending through this door (ADR-015 §Limits).
	pace *DiscoveryPace
	// emit announces a pushed snapshot to the frontend.
	emit func(connectionID, pluginID, nodeID string)
}

// NewDiscoveryDetailsService wires the node-details use case.
func NewDiscoveryDetailsService(
	store *DiscoveryStore,
	leader DiscoveryLeaderLookup,
	caller DiscoveryCaller,
	audit domainplugin.DiscoveryAuditRecorder,
	caps SurfaceCapabilityLookup,
	pace *DiscoveryPace,
) *DiscoveryDetailsService {
	return &DiscoveryDetailsService{
		store:  store,
		leader: leader,
		caller: caller,
		audit:  audit,
		caps:   caps,
		pace:   pace,
	}
}

// SetEmitter late-binds the frontend notification, like discoveryEmitHolder does for the tree.
func (s *DiscoveryDetailsService) SetEmitter(emit func(connectionID, pluginID, nodeID string)) {
	s.emit = emit
}

type describeNodePayload struct {
	SessionID string `json:"sessionId"`
	NodeID    string `json:"nodeId"`
}

type applyDetailsPayload struct {
	SessionID string            `json:"sessionId"`
	NodeID    string            `json:"nodeId"`
	Values    map[string]string `json:"values"`
}

type publishDetailsPayload struct {
	SessionID string                    `json:"sessionId"`
	NodeID    string                    `json:"nodeId"`
	Sections  []domainplugin.FieldGroup `json:"sections"`
	Values    map[string]string         `json:"values"`
	Editable  bool                      `json:"editable"`
}

// Describe asks a node's owner for its property panel.
//
// The node must exist in the tree the host currently holds. Asking the plugin about a node the
// host has never seen would let a frontend address anything at all through this path, and the
// answer would be about a node nobody is looking at.
func (s *DiscoveryDetailsService) Describe(ctx context.Context, connectionID, pluginID, nodeID string) (domainplugin.NodeDetails, error) {
	if !s.store.HasNode(connectionID, pluginID, nodeID) {
		return domainplugin.NodeDetails{}, ErrDiscoveryNodeNotFound
	}
	sessionID, _, ok := s.leader.Leading(connectionID)
	if !ok {
		return domainplugin.NodeDetails{}, ErrDiscoveryNoLeadingSession
	}
	params, err := json.Marshal(describeNodePayload{SessionID: sessionID, NodeID: nodeID})
	if err != nil {
		return domainplugin.NodeDetails{}, fmt.Errorf("discovery: encode describeNode: %w", err)
	}
	raw, err := s.caller.CallWithTimeout(ctx, pluginID, discoveryDescribeNodeMethod, params, domainplugin.NodeDetailsAckTimeout)
	if err != nil {
		return domainplugin.NodeDetails{}, fmt.Errorf("%w: %v", ErrDiscoveryDetailsUnavailable, err)
	}
	var details domainplugin.NodeDetails
	if err := json.Unmarshal(raw, &details); err != nil {
		return domainplugin.NodeDetails{}, fmt.Errorf("discovery: decode node details: %w", err)
	}
	if err := details.Validate(); err != nil {
		return domainplugin.NodeDetails{}, err
	}
	// The panel is drawn beside the tree the user trusts, so its labels are cleaned like a node's.
	details.Sections = sanitizeFieldGroups(details.Sections)
	return details, nil
}

// Apply hands edited values back to the node's owner, which persists them.
//
// The host stores nothing: it cannot name a discovered resource stably across restarts, and a
// plugin's opinion about a remote object is not core state (ADR-015 §3). What the host does do is
// audit the attempt and refuse values for fields the plugin never declared.
func (s *DiscoveryDetailsService) Apply(ctx context.Context, connectionID, pluginID, nodeID string, values map[string]string) error {
	if !s.store.HasNode(connectionID, pluginID, nodeID) {
		return ErrDiscoveryNodeNotFound
	}
	sessionID, _, ok := s.leader.Leading(connectionID)
	if !ok {
		return ErrDiscoveryNoLeadingSession
	}
	// Re-asking the plugin for the declaration costs one round trip and buys the only thing that
	// makes filtering meaningful: the frontend's idea of which fields exist is not authoritative,
	// and a stale panel would otherwise send values for fields that no longer exist.
	details, err := s.Describe(ctx, connectionID, pluginID, nodeID)
	if err != nil {
		return err
	}
	if !details.Editable {
		return fmt.Errorf("%w: node is read-only", domainplugin.ErrCapabilityDenied)
	}
	accepted := details.FilterValues(values)
	params, err := json.Marshal(applyDetailsPayload{SessionID: sessionID, NodeID: nodeID, Values: accepted})
	if err != nil {
		return fmt.Errorf("discovery: encode applyDetails: %w", err)
	}
	// Audited before the call, matching InvokeAction: an apply that reached the plugin and then
	// timed out has still been dispatched.
	s.record(domainplugin.DiscoveryAuditDispatch, connectionID, pluginID, sessionID, nodeID, nil)
	if _, err := s.caller.CallWithTimeout(ctx, pluginID, discoveryApplyDetailsMethod, params, domainplugin.NodeDetailsAckTimeout); err != nil {
		err = fmt.Errorf("%w: %v", ErrDiscoveryDetailsUnavailable, err)
		s.record(domainplugin.DiscoveryAuditResult, connectionID, pluginID, sessionID, nodeID, err)
		return err
	}
	s.record(domainplugin.DiscoveryAuditResult, connectionID, pluginID, sessionID, nodeID, nil)
	return nil
}

// PublishDetails accepts a snapshot a plugin pushed, so a panel already on screen refreshes
// without the user reopening it.
//
// Level-triggered and a full snapshot, matching discovery.publish: a delta protocol here would
// desync against a tree that is itself snapshot-based. The IDOR check has already run in
// PluginSessionRPCHandler, on the same path discovery.publish takes.
func (s *DiscoveryDetailsService) PublishDetails(_ context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	caps := s.capsFor(pluginID)
	if caps == nil || !caps.NodeDetails {
		return nil, fmt.Errorf("%w: nodeDetails not declared", domainplugin.ErrCapabilityDenied)
	}
	var payload publishDetailsPayload
	if err := json.Unmarshal(params, &payload); err != nil {
		return nil, err
	}
	connectionID, ok := s.leader.ConnectionForSession(payload.SessionID)
	// The budget is charged before the snapshot is examined, and against the connection the session
	// belongs to — the same key discovery.publish is metered by. A push costs the host a round trip
	// back to the plugin when the panel re-reads, so an unmetered one is an amplifier.
	if ok && s.pace != nil && !s.pace.AllowPublish(pluginID, connectionID) {
		return nil, fmt.Errorf("%w: node details publish rate exceeded", domainplugin.ErrRateLimited)
	}
	details := domainplugin.NodeDetails{
		Sections: payload.Sections,
		Values:   payload.Values,
		Editable: payload.Editable,
	}
	if err := details.Validate(); err != nil {
		return nil, err
	}
	// A snapshot for a session that has stopped leading is accepted and dropped, the same way a
	// publish for a collapsed branch is: the plugin is racing a handover, which ADR-014 treats as
	// normal rather than as an error worth reporting.
	if ok && s.emit != nil {
		s.emit(connectionID, pluginID, payload.NodeID)
	}
	return json.Marshal(map[string]bool{"ok": true})
}

func (s *DiscoveryDetailsService) capsFor(pluginID string) *domainplugin.UICaps {
	if s.caps == nil {
		return nil
	}
	return s.caps(pluginID)
}

func (s *DiscoveryDetailsService) record(phase, connectionID, pluginID, sessionID, nodeID string, err error) {
	if s.audit == nil {
		return
	}
	entry := domainplugin.DiscoveryAuditEntry{
		Timestamp:    time.Now(),
		Action:       phase,
		PluginID:     pluginID,
		ConnectionID: connectionID,
		SessionID:    sessionID,
		NodeIDs:      []string{nodeID},
		ActionID:     discoveryApplyDetailsMethod,
		Success:      err == nil,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	s.audit(entry)
}
