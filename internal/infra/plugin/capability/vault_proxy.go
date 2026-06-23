package capability

import (
	"context"
	"encoding/json"

	domainplugin "ssh-client/internal/domain/plugin"
)

// VaultProxy forwards vault RPC to the usecase inbound port.
type VaultProxy struct {
	vault domainplugin.VaultInboundPort
}

// NewVaultProxy creates a vault RPC proxy.
func NewVaultProxy(vault domainplugin.VaultInboundPort) *VaultProxy {
	return &VaultProxy{vault: vault}
}

// GetConnection forwards vault.getConnection.
func (p *VaultProxy) GetConnection(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	if p.vault == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return p.vault.GetConnection(ctx, pluginID, params)
}

// GetSecret forwards vault.getSecret.
func (p *VaultProxy) GetSecret(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	if p.vault == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return p.vault.GetSecret(ctx, pluginID, params)
}
