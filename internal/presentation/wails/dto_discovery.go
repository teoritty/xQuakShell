package wails

import (
	"xquakshell/internal/domain/discovery"
	"xquakshell/internal/usecase"
)

// The wire contract for plugin-drawn subtrees (ADR-014), restated here rather than forwarded from
// the usecase read model.
//
// The read model exists to serve the store's readers and will change when the tree's logic does;
// this is what the frontend compiles against, and it must change only when someone decides it
// should. The duplication is the point — the mappers below are where an unintended change is forced
// to become visible.
//
// One rule holds across every type in this file: the frontend addresses everything by connectionId.
// sessionId is a backend transport detail, resolved in the leader, and it appears in no field here
// and no handler argument. TestDiscoveryDTOCarriesNoSessionID checks that by marshalling a fully
// populated snapshot and searching the bytes, because "I looked and it wasn't there" does not
// survive the next field somebody adds.

// DiscoveryStatusDTO is the plugin's opinion about a node's health. Filled only by the plugin;
// its host-owned counterpart is DiscoveryBranchDTO.
type DiscoveryStatusDTO struct {
	Tone    string `json:"tone"`
	Color   string `json:"color,omitempty"`
	Tooltip string `json:"tooltip,omitempty"`
}

// DiscoveryActionDTO is one opaque, plugin-defined operation offered on a node. The core draws the
// menu and relays the click; what the action does is known only to the plugin.
type DiscoveryActionDTO struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	IconID string `json:"iconId,omitempty"`
	Danger bool   `json:"danger,omitempty"`
	// Confirm is the confirmation prompt the plugin wants shown before the action runs. Empty means
	// no prompt.
	Confirm string `json:"confirm,omitempty"`
	// Multi marks the action eligible for a multi-selection of siblings. The frontend offers it only
	// when every selected node carries the same actionId with multi set.
	Multi bool `json:"multi,omitempty"`
}

// DiscoveryNodeDTO is one row of a subtree.
//
// IconID is the RESOLVED icon: the usecase already walked the node's ancestors, because a node
// without its own icon inherits the nearest ancestor's and only the store holds that chain. The
// frontend looks it up in PluginDTO.discoveryIcons and renders it as <img src="data:...">.
type DiscoveryNodeDTO struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	// Kind is "group" (may have children) or "instance" (a leaf).
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	IconID string `json:"iconId,omitempty"`
	// Order is the plugin's sort key; ties break on label, then pluginId.
	Order int `json:"order"`
	// Status is a pointer because "the plugin reported no status" must stay distinct from a
	// present-but-neutral tone: the first renders no dot at all, the second renders a grey one.
	Status          *DiscoveryStatusDTO  `json:"status,omitempty"`
	Actions         []DiscoveryActionDTO `json:"actions"`
	DefaultActionID string               `json:"defaultActionId,omitempty"`
}

// DiscoveryTruncatedDTO reports that the host cut a publish down to the per-snapshot child limit.
// Shown out of Total is what lets the row say the list is incomplete instead of quietly lying.
type DiscoveryTruncatedDTO struct {
	Shown int `json:"shown"`
	Total int `json:"total"`
}

// DiscoveryBranchDTO is the host's own state for one node's children — never the plugin's.
// State is "loading", "ready", "error" or "stale"; the last means the leading session handed over
// and the branch on screen is the previous answer.
type DiscoveryBranchDTO struct {
	State     string                 `json:"state"`
	Error     string                 `json:"error,omitempty"`
	Truncated *DiscoveryTruncatedDTO `json:"truncated,omitempty"`
}

// DiscoveryPluginTreeDTO is one plugin's slice of a connection's tree.
//
// PluginID is not decoration and not merely a grouping caption: it is the third sort key, and it is
// the addressee InvokeDiscoveryAction must be given. Node IDs are plugin-chosen, so two plugins may
// legitimately publish a node called "containers" under the same connection — an action routed by
// node ID alone would pick an owner by map order, which is randomized in Go.
//
// Nodes are pre-order: every node follows its parent, so the frontend builds the tree in one pass.
// Branches is keyed by node ID, with "" standing for the connection root.
type DiscoveryPluginTreeDTO struct {
	PluginID string                        `json:"pluginId"`
	Nodes    []DiscoveryNodeDTO            `json:"nodes"`
	Branches map[string]DiscoveryBranchDTO `json:"branches"`
}

// DiscoverySnapshotDTO is one connection's whole discovery tree.
type DiscoverySnapshotDTO struct {
	ConnectionID string                   `json:"connectionId"`
	Plugins      []DiscoveryPluginTreeDTO `json:"plugins"`
}

// discoverySnapshotToDTO maps the usecase read model to the wire contract.
//
// Every slice and map it produces is non-nil. A connection with no discovery plugins must reach the
// frontend as `{"connectionId":"x","plugins":[]}`, not as a null the tree component has to
// special-case before it can iterate — and "no tree" is a perfectly ordinary state here, not an
// error.
func discoverySnapshotToDTO(snapshot usecase.DiscoverySnapshot) DiscoverySnapshotDTO {
	dto := DiscoverySnapshotDTO{
		ConnectionID: snapshot.ConnectionID,
		Plugins:      make([]DiscoveryPluginTreeDTO, 0, len(snapshot.Plugins)),
	}
	for _, tree := range snapshot.Plugins {
		dto.Plugins = append(dto.Plugins, discoveryPluginTreeToDTO(tree))
	}
	return dto
}

func discoveryPluginTreeToDTO(tree usecase.DiscoveryPluginTreeView) DiscoveryPluginTreeDTO {
	dto := DiscoveryPluginTreeDTO{
		PluginID: tree.PluginID,
		Nodes:    make([]DiscoveryNodeDTO, 0, len(tree.Nodes)),
		Branches: make(map[string]DiscoveryBranchDTO, len(tree.Branches)),
	}
	for _, node := range tree.Nodes {
		dto.Nodes = append(dto.Nodes, discoveryNodeToDTO(node))
	}
	for nodeID, branch := range tree.Branches {
		dto.Branches[nodeID] = discoveryBranchToDTO(branch)
	}
	return dto
}

func discoveryNodeToDTO(view usecase.DiscoveryNodeView) DiscoveryNodeDTO {
	node := view.Node
	dto := DiscoveryNodeDTO{
		ID:       node.ID,
		ParentID: node.ParentID,
		Kind:     string(node.Kind),
		Label:    node.Label,
		// The resolved icon, not node.IconID: inheritance from the nearest ancestor already happened
		// upstream, and re-deriving it here would need the ancestor chain the frontend does not have.
		IconID:          view.Icon,
		Order:           node.Order,
		Status:          discoveryStatusToDTO(node.Status),
		Actions:         discoveryActionsToDTO(node.Actions),
		DefaultActionID: node.DefaultActionID,
	}
	return dto
}

func discoveryStatusToDTO(status *discovery.Status) *DiscoveryStatusDTO {
	if status == nil {
		return nil
	}
	return &DiscoveryStatusDTO{
		Tone:    string(status.Tone),
		Color:   status.Color,
		Tooltip: status.Tooltip,
	}
}

func discoveryActionsToDTO(actions []discovery.Action) []DiscoveryActionDTO {
	dtos := make([]DiscoveryActionDTO, 0, len(actions))
	for _, action := range actions {
		dtos = append(dtos, DiscoveryActionDTO{
			ID:      action.ID,
			Label:   action.Label,
			IconID:  action.IconID,
			Danger:  action.Danger,
			Confirm: action.Confirm,
			Multi:   action.Multi,
		})
	}
	return dtos
}

func discoveryBranchToDTO(branch discovery.Branch) DiscoveryBranchDTO {
	dto := DiscoveryBranchDTO{
		State: string(branch.State),
		Error: branch.Error,
	}
	if branch.Truncated != nil {
		dto.Truncated = &DiscoveryTruncatedDTO{Shown: branch.Truncated.Shown, Total: branch.Truncated.Total}
	}
	return dto
}
