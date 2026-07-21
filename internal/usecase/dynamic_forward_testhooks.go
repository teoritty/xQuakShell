package usecase

import (
	"context"
	"net"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/domain"
)

// StartDynamicForwardSessionForTest activates dynamic forward rules in integration tests.
func (c *DynamicForwardCoordinator) StartDynamicForwardSessionForTest(ctx context.Context, sessionID string, dialer domain.TunnelChannelDialer, rules []domain.ForwardRule) {
	if c == nil {
		return
	}
	if c.tunnelGrant == nil {
		c.tunnelGrant = testTunnelGrantAlways{}
	}
	c.StartSession(ctx, sessionID, dialer, rules)
}

type testTunnelGrantAlways struct{}

func (testTunnelGrantAlways) IsTunnelProviderGranted(string) bool { return true }

// RegisterPreBindLocalForTest registers a pre-bind local connection and owner index for tests.
func (c *DynamicForwardCoordinator) RegisterPreBindLocalForTest(ctx context.Context, sessionID, pluginID, ruleID, providerID, localConnID string, conn net.Conn) error {
	if c == nil {
		return domainplugin.ErrTunnelNotFound
	}
	c.mu.Lock()
	sf := c.sessions[sessionID]
	c.mu.Unlock()
	if sf == nil || sf.service == nil {
		return domainplugin.ErrTunnelNotFound
	}
	if err := sf.service.RegisterLocal(ctx, pluginID, ruleID, providerID, localConnID, conn); err != nil {
		return err
	}
	c.mu.Lock()
	c.localOwners[localConnID] = tunnelHandleOwner{sessionID: sessionID, pluginID: pluginID}
	c.mu.Unlock()
	return nil
}

// RegisterTunnelOwnerForTest records tunnel handle ownership without dialing.
func (c *DynamicForwardCoordinator) RegisterTunnelOwnerForTest(sessionID, pluginID, tunnelID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.tunnelOwners[tunnelID] = tunnelHandleOwner{sessionID: sessionID, pluginID: pluginID}
	c.mu.Unlock()
}
