package wails

import (
	"context"
	"errors"
	"strings"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	domainplugin "xquakshell/internal/domain/plugin"
)

var errNodeDetailsUnavailable = errors.New("node details unavailable")

// NodeDetailsService is the slice of the node-details use case this layer needs. Both methods take
// connectionId, never a sessionId — the frontend addresses connections (ADR-014).
type NodeDetailsService interface {
	Describe(ctx context.Context, connectionID, pluginID, nodeID string) (domainplugin.NodeDetails, error)
	Apply(ctx context.Context, connectionID, pluginID, nodeID string, values map[string]string) error
}

// NodeDetailsDTO is a discovery node's property panel as the frontend sees it.
type NodeDetailsDTO struct {
	Sections []DialogSectionDTO `json:"sections"`
	Values   map[string]string  `json:"values"`
	Editable bool               `json:"editable"`
}

// NodeDetailsChangedPayload tells the frontend a plugin pushed a newer panel for a node it may be
// showing. It carries no snapshot: the frontend re-reads through DescribeDiscoveryNode, so one
// reader path serves both the first open and every refresh — the rule DiscoveryTreeChanged follows.
type NodeDetailsChangedPayload struct {
	ConnectionID string `json:"connectionId"`
	PluginID     string `json:"pluginId"`
	NodeID       string `json:"nodeId"`
}

// SetNodeDetailsService wires the node-details use case.
func (a *AppAPI) SetNodeDetailsService(svc NodeDetailsService) {
	if a == nil {
		return
	}
	a.nodeDetails = svc
}

// DescribeDiscoveryNode returns the property panel for one node of a plugin's subtree.
//
// pluginId is required rather than inferred, for the reason InvokeDiscoveryAction states: node ids
// are plugin-chosen and two plugins may publish the same one under a connection.
func (a *AppAPI) DescribeDiscoveryNode(connectionId string, pluginId string, nodeId string) (NodeDetailsDTO, error) {
	if a == nil || a.nodeDetails == nil {
		return NodeDetailsDTO{}, errNodeDetailsUnavailable
	}
	if strings.TrimSpace(connectionId) == "" || strings.TrimSpace(pluginId) == "" {
		return NodeDetailsDTO{}, errNodeDetailsUnavailable
	}
	details, err := a.nodeDetails.Describe(a.reqCtx(), connectionId, pluginId, nodeId)
	if err != nil {
		return NodeDetailsDTO{}, err
	}
	return nodeDetailsToDTO(details), nil
}

// ApplyDiscoveryNodeDetails hands edited values to the node's owner, which persists them. The host
// stores nothing (ADR-015 §3).
func (a *AppAPI) ApplyDiscoveryNodeDetails(connectionId string, pluginId string, nodeId string, values map[string]string) error {
	if a == nil || a.nodeDetails == nil {
		return errNodeDetailsUnavailable
	}
	if strings.TrimSpace(connectionId) == "" || strings.TrimSpace(pluginId) == "" {
		return errNodeDetailsUnavailable
	}
	if values == nil {
		values = map[string]string{}
	}
	return a.nodeDetails.Apply(a.reqCtx(), connectionId, pluginId, nodeId, values)
}

// EmitNodeDetailsChanged announces that a plugin pushed a newer panel for a node.
func (a *AppAPI) EmitNodeDetailsChanged(connectionID, pluginID, nodeID string) {
	if a == nil || a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventPluginNodeDetails, NodeDetailsChangedPayload{
		ConnectionID: connectionID,
		PluginID:     pluginID,
		NodeID:       nodeID,
	})
}

func nodeDetailsToDTO(details domainplugin.NodeDetails) NodeDetailsDTO {
	values := details.Values
	if values == nil {
		values = map[string]string{}
	}
	return NodeDetailsDTO{
		Sections: fieldGroupsToDTO(details.Sections),
		Values:   values,
		Editable: details.Editable,
	}
}
