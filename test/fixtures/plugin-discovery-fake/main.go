// Command plugin-discovery-fake is a technology-neutral discovery plugin used by the end-to-end
// test (ADR-014). It draws a small tree of made-up resources so the whole path — capability,
// observe, publish, icon inheritance, actions, teardown — can be exercised against a real plugin
// process rather than a fake in-process object.
//
// It deliberately does not pretend to be Docker or Kubernetes. The core knows nothing about either,
// and a fixture named after one would quietly teach the reader that discovery is a container
// feature.
package main

import (
	"encoding/json"
	"log"
	"maps"
	"sort"
	"sync"
	"time"

	"xquakshell/test/fixtures/pluginhost"
)

// plugin holds everything the fixture learns at runtime: the session the host addresses it on, the
// set of nodes the user has expanded, and the tones its own resources currently carry.
type plugin struct {
	host *pluginhost.Host

	mu        sync.Mutex
	sessionID string
	observed  map[string]struct{}
	tones     map[string]string
	published map[string]int

	// pubMu serializes outbound publish sequences. Publishes are issued from goroutines (see
	// observe below), and a branch must never reach the host before the branch containing it.
	pubMu sync.Mutex
}

func main() {
	p := &plugin{
		host:      pluginhost.NewHost(),
		observed:  make(map[string]struct{}),
		tones:     make(map[string]string),
		published: make(map[string]int),
	}
	maps.Copy(p.tones, initialTones)

	p.host.Register("initialize", func(json.RawMessage) (any, error) { return map[string]bool{"ok": true}, nil })
	p.host.Register("activate", func(json.RawMessage) (any, error) { return map[string]bool{"ok": true}, nil })
	p.host.Register("ping", func(json.RawMessage) (any, error) { return map[string]string{"pong": "ok"}, nil })
	p.host.Register("shutdown", func(json.RawMessage) (any, error) { return map[string]bool{"ok": true}, nil })

	p.host.RegisterNotification("discovery.observe", p.observe)
	p.host.Register("discovery.invokeAction", p.invokeAction)
	p.host.Register("fake.stats", p.stats)

	if err := p.host.Run(); err != nil {
		log.Fatal(err)
	}
}

type observeParams struct {
	SessionID string   `json:"sessionId"`
	NodeIDs   []string `json:"nodeIds"`
}

// observe records the full set of expanded nodes and publishes a snapshot for each branch in it.
//
// It publishes ONLY what is observed. That restraint is the entire point of a level-triggered
// protocol: a plugin that published its whole tree on the first notification would work, and would
// also poll resources nobody is looking at — the load the design exists to remove.
//
// The work runs on its own goroutine because the notification handler is called from the plugin's
// read loop, and discovery.publish is a request: publishing inline would block the read loop on a
// response only that loop can deliver.
func (p *plugin) observe(params json.RawMessage) {
	var req observeParams
	if err := json.Unmarshal(params, &req); err != nil {
		log.Printf("discovery.observe: %v", err)
		return
	}

	p.mu.Lock()
	p.sessionID = req.SessionID
	p.observed = make(map[string]struct{}, len(req.NodeIDs))
	for _, id := range req.NodeIDs {
		p.observed[id] = struct{}{}
	}
	p.mu.Unlock()

	go p.publishObserved()
}

// publishObserved sends one snapshot per observed branch, parents first.
func (p *plugin) publishObserved() {
	p.pubMu.Lock()
	defer p.pubMu.Unlock()
	for _, b := range layout {
		if !p.isObserved(b.parent) {
			continue
		}
		p.publish(b.parent)
	}
}

// publish sends the current children of one branch.
func (p *plugin) publish(parent string) {
	p.mu.Lock()
	sessionID := p.sessionID
	children := p.childrenLocked(parent)
	p.published[parent]++
	p.mu.Unlock()

	if sessionID == "" {
		return
	}
	if _, err := p.host.CallCore("discovery.publish", map[string]any{
		"sessionId": sessionID,
		"nodeId":    parent,
		"state":     "ready",
		"children":  children,
	}); err != nil {
		log.Printf("discovery.publish %q: %v", parent, err)
	}
}

// childrenLocked builds a branch's children with the tones the plugin currently believes in. The
// caller holds mu.
func (p *plugin) childrenLocked(parent string) []node {
	for _, b := range layout {
		if b.parent != parent {
			continue
		}
		out := make([]node, 0, len(b.children))
		for _, child := range b.children {
			child.ParentID = parent
			if tone, ok := p.tones[child.ID]; ok && tone != toneNone {
				child.Status = &status{Tone: tone, Tooltip: tooltips[tone]}
			}
			out = append(out, child)
		}
		return out
	}
	return nil
}

func (p *plugin) isObserved(nodeID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.observed[nodeID]
	return ok
}

type invokeParams struct {
	SessionID string   `json:"sessionId"`
	NodeIDs   []string `json:"nodeIds"`
	ActionID  string   `json:"actionId"`
}

// invokeAction acknowledges the request and does the work afterwards.
//
// This is the contract ADR-014 states in one line and that only a live plugin can demonstrate: the
// ack must arrive within 5 s, so real work reports back through publish instead of holding the RPC
// open. The fixture moves the affected nodes to "busy", publishes that, then finishes and publishes
// the outcome — the same two-step a plugin stopping fifty containers would perform.
func (p *plugin) invokeAction(params json.RawMessage) (any, error) {
	var req invokeParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	go p.applyAction(req.NodeIDs, req.ActionID)
	return map[string]bool{"ok": true}, nil
}

func (p *plugin) applyAction(nodeIDs []string, actionID string) {
	if actionID == actionInspect {
		// Inspect changes nothing: an action whose result is a read is still a legitimate action,
		// and it must not fabricate a status change.
		return
	}
	p.setTones(nodeIDs, "busy")
	p.publishBranchesOf(nodeIDs)

	// Standing in for work that takes time. Short enough not to slow the suite, long enough that
	// the ack provably does not wait for it.
	time.Sleep(20 * time.Millisecond)

	p.setTones(nodeIDs, "ok")
	p.publishBranchesOf(nodeIDs)
}

func (p *plugin) setTones(nodeIDs []string, tone string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, id := range nodeIDs {
		if _, known := p.tones[id]; known {
			p.tones[id] = tone
		}
	}
}

// publishBranchesOf republishes every observed branch touched by a set of nodes, each one once.
func (p *plugin) publishBranchesOf(nodeIDs []string) {
	seen := make(map[string]struct{}, len(nodeIDs))
	var branches []string
	for _, id := range nodeIDs {
		parent, ok := parentOf(id)
		if !ok {
			continue
		}
		if _, dup := seen[parent]; dup {
			continue
		}
		seen[parent] = struct{}{}
		branches = append(branches, parent)
	}

	p.pubMu.Lock()
	defer p.pubMu.Unlock()
	for _, b := range layout {
		for _, parent := range branches {
			if b.parent == parent && p.isObserved(parent) {
				p.publish(parent)
			}
		}
	}
}

// stats reports what the plugin has been asked to watch and how many snapshots it has sent per
// branch.
//
// It exists so the end-to-end test can assert the plugin's own behaviour rather than only the
// host's view of it. "The host dropped the publish" and "the plugin never sent one" are
// indistinguishable from the outside, and it is the second that a collapsed branch must produce.
func (p *plugin) stats(json.RawMessage) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	published := make(map[string]int, len(p.published))
	maps.Copy(published, p.published)
	observed := make([]string, 0, len(p.observed))
	for id := range p.observed {
		observed = append(observed, id)
	}
	sort.Strings(observed)
	return map[string]any{"published": published, "observed": observed}, nil
}
