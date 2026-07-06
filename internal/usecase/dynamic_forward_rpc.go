package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"

	domainplugin "ssh-client/internal/domain/plugin"
)

// TunnelDial implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelDial(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		RuleID     string `json:"ruleId"`
		TargetHost string `json:"targetHost"`
		TargetPort int    `json:"targetPort"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	sf, rule, err := c.sessionForRule(pluginID, req.RuleID)
	if err != nil {
		return nil, err
	}
	if rule.ProviderID == "" {
		return nil, domainplugin.ErrTunnelNotFound
	}
	tunnelID, err := newTunnelHandleID()
	if err != nil {
		return nil, err
	}
	if err := sf.service.Dial(ctx, tunnelID, req.TargetHost, req.TargetPort); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.tunnelOwners[tunnelID] = tunnelHandleOwner{sessionID: sf.sessionID, pluginID: pluginID}
	c.mu.Unlock()
	c.armChannelTimeout(pluginID, tunnelID, sf)
	return json.Marshal(map[string]string{"tunnelId": tunnelID})
}

// TunnelClose implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelClose(_ context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		TunnelID string `json:"tunnelId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	svc, err := c.serviceForChannel(pluginID, req.TunnelID)
	if err != nil {
		return nil, err
	}
	if err := svc.CloseChannel(req.TunnelID); err != nil {
		return nil, err
	}
	c.mu.Lock()
	owner := c.tunnelOwners[req.TunnelID]
	c.mu.Unlock()
	c.releaseTunnelOwner(req.TunnelID)
	if c.dialSlotRelease != nil && owner.sessionID != "" {
		c.dialSlotRelease(pluginID, owner.sessionID)
	}
	return json.Marshal(map[string]bool{"ok": true})
}

// TunnelLocalWrite implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelLocalWrite(_ context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		LocalConnID string `json:"localConnId"`
		DataBase64  string `json:"dataBase64"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(req.DataBase64)
	if err != nil {
		return nil, err
	}
	svc, err := c.serviceForLocal(pluginID, req.LocalConnID)
	if err != nil {
		return nil, err
	}
	if err := svc.WriteLocal(req.LocalConnID, data); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"ok": true})
}

// TunnelLocalClose implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelLocalClose(_ context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		LocalConnID string `json:"localConnId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	svc, err := c.serviceForLocal(pluginID, req.LocalConnID)
	if err != nil {
		return nil, err
	}
	if err := svc.CloseLocal(req.LocalConnID); err != nil {
		return nil, err
	}
	c.mu.Lock()
	delete(c.localOwners, req.LocalConnID)
	if done, ok := c.preBindDone[req.LocalConnID]; ok {
		close(done)
		delete(c.preBindDone, req.LocalConnID)
	}
	c.mu.Unlock()
	return json.Marshal(map[string]bool{"ok": true})
}

// TunnelBind implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelBind(_ context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		LocalConnID string `json:"localConnId"`
		TunnelID    string `json:"tunnelId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	svc, err := c.serviceForLocal(pluginID, req.LocalConnID)
	if err != nil {
		return nil, err
	}
	if _, err := c.serviceForChannel(pluginID, req.TunnelID); err != nil {
		return nil, err
	}
	if !svc.HasChannel(req.TunnelID) {
		return nil, domainplugin.ErrTunnelNotFound
	}
	if err := svc.Bind(req.LocalConnID, req.TunnelID); err != nil {
		return nil, err
	}
	c.markLocalBound(req.LocalConnID)
	c.releaseTunnelOwner(req.TunnelID)
	return json.Marshal(map[string]bool{"ok": true})
}

var _ domainplugin.TunnelInboundPort = (*DynamicForwardCoordinator)(nil)
