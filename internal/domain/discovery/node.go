// Package discovery models the plugin-drawn subtrees that appear inside a connection node
// (ADR-014). It is deliberately separate from domain/plugin: that package models the
// manifest contract a plugin installs with, this package models the UI tree a plugin draws
// once running. Neither imports the other, and this package imports nothing from the
// project — it is pure, technology-neutral data plus validation.
package discovery

// Kind distinguishes a container node from a leaf. The core does not otherwise care what a
// node represents — "docker container" and "postgres database" are both just an instance to
// the tree widget.
type Kind string

const (
	// KindGroup is a node that may have children (e.g. a "containers" or "volumes" group).
	KindGroup Kind = "group"
	// KindInstance is a leaf: a single discovered resource with no children of its own.
	KindInstance Kind = "instance"
)

// Node is one entry in a discovery subtree, as published by a plugin. Every field here is
// plugin-authored; the host never writes into a Node — the host's own state about a branch
// (loading/error/staleness) lives in Branch, a separate type, so the two provenances can
// never collide on the wire or in memory.
type Node struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	Kind     Kind   `json:"kind"`
	Label    string `json:"label"`
	IconID   string `json:"iconId,omitempty"`
	Order    int    `json:"order,omitempty"`
	// Status is a pointer because its absence ("the plugin reported no status") is a distinct
	// signal from a present-but-neutral tone: an instance the plugin has no opinion about
	// should render with no status dot at all, not a grey one.
	Status          *Status  `json:"status,omitempty"`
	Actions         []Action `json:"actions,omitempty"`
	DefaultActionID string   `json:"defaultActionId,omitempty"`
}

// Action is one opaque, plugin-defined operation offered on a Node. The core only ever draws
// the menu and relays the click as discovery.invokeAction — what the action does is known
// exclusively to the plugin (ADR-014 "Actions").
type Action struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	IconID  string `json:"iconId,omitempty"`
	Danger  bool   `json:"danger,omitempty"`
	Confirm string `json:"confirm,omitempty"`
	// Multi marks an action eligible for mass invocation over several selected siblings.
	// An action is offered for a multi-selection only when every selected node carries it.
	Multi bool `json:"multi,omitempty"`
}
