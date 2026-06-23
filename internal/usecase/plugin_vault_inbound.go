package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

// PluginSettingsReader supplies plugin install policy from vault settings.
type PluginSettingsReader interface {
	PluginSettings() (domain.PluginSettings, error)
}

// PluginVaultInbound enforces vault IDOR and secret consent for plugin RPC.
type PluginVaultInbound struct {
	mu              sync.RWMutex
	registry        *PluginRegistry
	connRepo        domain.ConnectionRepository
	passwordRepo    domain.PasswordRepository
	identRepo       domain.IdentityRepository
	passphraseCache domain.PassphraseCache
	authorizer      PluginVaultAuthorizer
	settings        PluginSettingsReader
	auditLogger     domainplugin.VaultAccessAuditLogger
}

// NewPluginVaultInbound creates a vault inbound adapter.
func NewPluginVaultInbound(
	registry *PluginRegistry,
	connRepo domain.ConnectionRepository,
	passwordRepo domain.PasswordRepository,
	identRepo domain.IdentityRepository,
	settings PluginSettingsReader,
	passphraseCache domain.PassphraseCache,
) *PluginVaultInbound {
	return &PluginVaultInbound{
		registry:        registry,
		connRepo:        connRepo,
		passwordRepo:    passwordRepo,
		identRepo:       identRepo,
		passphraseCache: passphraseCache,
		settings:        settings,
	}
}

// SetAuthorizer binds the session manager after composition.
func (p *PluginVaultInbound) SetAuthorizer(a PluginVaultAuthorizer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.authorizer = a
}

// SetAuditLogger binds the immutable vault access audit logger (required for successful vault RPC).
func (p *PluginVaultInbound) SetAuditLogger(logger domainplugin.VaultAccessAuditLogger) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.auditLogger = logger
}

type vaultConnectionParams struct {
	ConnectionID string `json:"connectionId"`
}

type vaultSecretParams struct {
	ConnectionID string `json:"connectionId"`
	Field        string `json:"field"`
}

// GetConnection implements domainplugin.VaultInboundPort.
func (p *PluginVaultInbound) GetConnection(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	allowed, err := p.allowedConnectionFields(pluginID)
	if err != nil {
		return nil, err
	}
	if len(allowed) == 0 {
		return nil, domainplugin.ErrCapabilityDenied
	}

	var req vaultConnectionParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid vault.getConnection params: %w", err)
	}
	if req.ConnectionID == "" {
		return nil, fmt.Errorf("connectionId is required")
	}
	if !p.checkOwnership(pluginID, req.ConnectionID) {
		return nil, domainplugin.ErrCapabilityDenied
	}

	conn, err := p.connRepo.GetByID(ctx, req.ConnectionID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, domain.ErrConnectionNotFound
	}

	out := filterConnectionFields(conn, allowed)
	if err := p.recordVaultAccess(ctx, pluginID, req.ConnectionID, domainplugin.VaultAccessGetConnection, ""); err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// GetSecret implements domainplugin.VaultInboundPort.
func (p *PluginVaultInbound) GetSecret(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	allowed, err := p.allowedSecretFields(pluginID)
	if err != nil {
		return nil, err
	}
	if len(allowed) == 0 {
		return nil, domainplugin.ErrCapabilityDenied
	}
	if !p.secretAccessGranted(pluginID) {
		return nil, domainplugin.ErrCapabilityDenied
	}

	var req vaultSecretParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid vault.getSecret params: %w", err)
	}
	if req.ConnectionID == "" || req.Field == "" {
		return nil, fmt.Errorf("connectionId and field are required")
	}
	if !containsString(allowed, req.Field) {
		return nil, domainplugin.ErrCapabilityDenied
	}
	if !p.checkOwnership(pluginID, req.ConnectionID) {
		return nil, domainplugin.ErrCapabilityDenied
	}

	conn, err := p.connRepo.GetByID(ctx, req.ConnectionID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, domain.ErrConnectionNotFound
	}

	secret, err := p.resolveSecret(ctx, conn, req.Field)
	if err != nil {
		return nil, err
	}
	if err := p.recordVaultAccess(ctx, pluginID, req.ConnectionID, domainplugin.VaultAccessGetSecret, req.Field); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{
		"field":         req.Field,
		"valueBase64":   base64.StdEncoding.EncodeToString(secret),
	})
}

func (p *PluginVaultInbound) allowedConnectionFields(pluginID string) ([]string, error) {
	plugin, err := p.registry.Get(pluginID)
	if err != nil {
		return nil, err
	}
	if plugin.Manifest.Capabilities.Vault == nil {
		return nil, nil
	}
	return append([]string(nil), plugin.Manifest.Capabilities.Vault.ReadConnectionFields...), nil
}

func (p *PluginVaultInbound) allowedSecretFields(pluginID string) ([]string, error) {
	plugin, err := p.registry.Get(pluginID)
	if err != nil {
		return nil, err
	}
	if plugin.Manifest.Capabilities.Vault == nil {
		return nil, nil
	}
	return append([]string(nil), plugin.Manifest.Capabilities.Vault.GetSecret...), nil
}

func (p *PluginVaultInbound) secretAccessGranted(pluginID string) bool {
	if p.settings == nil {
		return false
	}
	settings, err := p.settings.PluginSettings()
	if err != nil {
		return false
	}
	if settings.SecretAccessGranted == nil {
		return false
	}
	return settings.SecretAccessGranted[pluginID]
}

func (p *PluginVaultInbound) recordVaultAccess(
	ctx context.Context,
	pluginID, connectionID string,
	method domainplugin.VaultAccessMethod,
	field string,
) error {
	p.mu.RLock()
	logger := p.auditLogger
	p.mu.RUnlock()
	if logger == nil {
		return domainplugin.ErrVaultAuditFailed
	}
	return logger.RecordVaultAccess(ctx, domainplugin.VaultAccessEvent{
		Timestamp:    time.Now().UTC(),
		PluginID:     pluginID,
		ConnectionID: connectionID,
		Method:       method,
		Field:        field,
	})
}

func (p *PluginVaultInbound) checkOwnership(pluginID, connectionID string) bool {
	p.mu.RLock()
	auth := p.authorizer
	p.mu.RUnlock()
	if auth == nil {
		return false
	}
	return auth.PluginOwnsConnection(pluginID, connectionID)
}

func filterConnectionFields(conn *domain.Connection, allowed []string) map[string]any {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, f := range allowed {
		allowedSet[f] = struct{}{}
	}
	out := make(map[string]any)
	for field := range allowedSet {
		switch field {
		case "id":
			out["id"] = conn.ID
		case "name":
			out["name"] = conn.Name
		case "host":
			out["host"] = conn.Host
		case "port":
			out["port"] = conn.Port
		case "protocol":
			out["protocol"] = conn.GetProtocol()
		case "folderId":
			out["folderId"] = conn.FolderID
		}
	}
	return out
}

func (p *PluginVaultInbound) resolveSecret(ctx context.Context, conn *domain.Connection, field string) ([]byte, error) {
	user := conn.DefaultUser()
	switch field {
	case "password":
		if user == nil || user.PassAuth == nil || user.PassAuth.PasswordID == "" {
			return nil, domainplugin.ErrCapabilityDenied
		}
		return p.passwordRepo.Get(ctx, user.PassAuth.PasswordID)
	case "privateKey":
		if user == nil || user.KeyAuth == nil || len(user.KeyAuth.IdentityIDs) == 0 {
			return nil, domainplugin.ErrCapabilityDenied
		}
		return p.identRepo.GetKeyBlob(ctx, user.KeyAuth.IdentityIDs[0])
	case "passphrase":
		return p.resolveKeyPassphrase(ctx, conn)
	default:
		return nil, domainplugin.ErrCapabilityDenied
	}
}

func (p *PluginVaultInbound) resolveKeyPassphrase(ctx context.Context, conn *domain.Connection) ([]byte, error) {
	user := conn.DefaultUser()
	if user == nil || user.KeyAuth == nil || len(user.KeyAuth.IdentityIDs) == 0 {
		return nil, domainplugin.ErrCapabilityDenied
	}
	identityID := user.KeyAuth.IdentityIDs[0]
	encrypted, err := p.identityEncrypted(ctx, identityID)
	if err != nil {
		return nil, err
	}
	if !encrypted {
		return nil, domainplugin.ErrCapabilityDenied
	}
	if p.passphraseCache == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	passphrase, ok := p.passphraseCache.Get(identityID)
	if !ok || passphrase == "" {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return []byte(passphrase), nil
}

func (p *PluginVaultInbound) identityEncrypted(ctx context.Context, identityID string) (bool, error) {
	idents, err := p.identRepo.GetAll(ctx)
	if err != nil {
		return false, err
	}
	for _, ident := range idents {
		if ident.ID == identityID {
			return ident.Encrypted, nil
		}
	}
	return false, domainplugin.ErrCapabilityDenied
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

var _ domainplugin.VaultInboundPort = (*PluginVaultInbound)(nil)
