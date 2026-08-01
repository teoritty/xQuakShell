package usecase

import (
	"log/slog"

	"xquakshell/internal/domain/discovery"
)

// DiscoveryIconLookup reports the icon IDs a plugin declared in contributions.discoveryIcons.
// Satisfied by *PluginRegistry.
type DiscoveryIconLookup interface {
	DeclaredDiscoveryIcons(pluginID string) map[string]struct{}
}

// stripUndeclaredIcons blanks every IconID a plugin never registered in its manifest, returning the
// snapshot's children with only that field changed.
//
// This is the one check in discovery that needs both the manifest and the snapshot, and it lives
// here — in the usecase, on the publish path, one step before the store — for three reasons:
//
//   - the domain cannot host it: internal/domain/discovery is deliberately manifest-blind, and
//     handing it the icon set would make the tree model depend on the install contract;
//   - the capability layer cannot host it without duplicating work: discovery.publish params travel
//     from ipc straight into this package, so infra would have to decode the whole snapshot a
//     second time purely to inspect a field it does not otherwise touch;
//   - the store must not host it: it would put a registry lookup inside the tree mutex, and the
//     store is by design the one place that knows nothing about plugins beyond their ID.
//
// An unknown icon is NOT a rejection (ADR-014 "Security model"): the node is published without an
// icon and the fact is logged. A plugin that renamed an icon and forgot its manifest is careless,
// not hostile, and dropping a whole branch of real resources over a decorative field would cost the
// user far more than the missing glyph.
func stripUndeclaredIcons(icons DiscoveryIconLookup, pluginID string, children []discovery.Node) []discovery.Node {
	if icons == nil || len(children) == 0 {
		return children
	}
	declared := icons.DeclaredDiscoveryIcons(pluginID)
	out := children
	copied := false
	for i, child := range children {
		nodeUnknown := child.IconID != "" && !isDeclaredIcon(declared, child.IconID)
		actionsUnknown := hasUndeclaredActionIcon(declared, child.Actions)
		if !nodeUnknown && !actionsUnknown {
			continue
		}
		if !copied {
			// Copy on first offence only: the overwhelmingly common snapshot declares every icon it
			// uses, and it must not pay for a defensive clone it never needs.
			out = append([]discovery.Node(nil), children...)
			copied = true
		}
		if nodeUnknown {
			slog.Warn("discovery: node references an undeclared iconId, publishing without an icon",
				"component", "discovery", "pluginId", pluginID, "nodeId", child.ID, "iconId", child.IconID)
			out[i].IconID = ""
		}
		if actionsUnknown {
			out[i].Actions = strippedActionIcons(declared, pluginID, child.ID, child.Actions)
		}
	}
	return out
}

func isDeclaredIcon(declared map[string]struct{}, iconID string) bool {
	_, ok := declared[iconID]
	return ok
}

func hasUndeclaredActionIcon(declared map[string]struct{}, actions []discovery.Action) bool {
	for _, action := range actions {
		if action.IconID != "" && !isDeclaredIcon(declared, action.IconID) {
			return true
		}
	}
	return false
}

// strippedActionIcons returns a copy of actions with undeclared icons blanked. Actions are copied
// rather than mutated in place because the caller's slice header is shared with the decoded payload
// — the node copy above does not deep-copy it.
func strippedActionIcons(declared map[string]struct{}, pluginID, nodeID string, actions []discovery.Action) []discovery.Action {
	out := append([]discovery.Action(nil), actions...)
	for i, action := range out {
		if action.IconID == "" || isDeclaredIcon(declared, action.IconID) {
			continue
		}
		slog.Warn("discovery: action references an undeclared iconId, publishing without an icon",
			"component", "discovery", "pluginId", pluginID, "nodeId", nodeID,
			"actionId", action.ID, "iconId", action.IconID)
		out[i].IconID = ""
	}
	return out
}
