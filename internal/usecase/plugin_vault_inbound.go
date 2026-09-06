package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
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
	lockState       VaultLockState
	settings        PluginSettingsReader
	auditLogger     domainplugin.VaultAccessAuditLogger
	keyAudit        KeyManagerAudit
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

// SetKeyAudit binds the key-event recorder after composition.
//
// It is a setter rather than a constructor parameter because the constructor is already at the
// five-parameter limit and grew past it before that limit existed; adding a seventh would make a
// baselined signature worse rather than better.
func (p *PluginVaultInbound) SetKeyAudit(a KeyManagerAudit) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keyAudit = a
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
	if p.anchorFor(ctx, pluginID, req.ConnectionID) == domainplugin.AnchorNone {
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
	granted := p.grantedTo(pluginID)

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
	// Consent is per field. Under the boolean this check did not exist, so a plugin whose update
	// added a field to its manifest could read it on the strength of a consent given for another.
	if !granted.Has(domainplugin.PermissionSecretField(req.Field)) {
		return nil, domainplugin.ErrCapabilityDenied
	}
	if p.anchorFor(ctx, pluginID, req.ConnectionID) == domainplugin.AnchorNone {
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
		"field":       req.Field,
		"valueBase64": base64.StdEncoding.EncodeToString(secret),
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

// grantedTo reads the permissions the user consented to for a plugin (ADR-022).
//
// Every failure reads as no permissions - settings that are unwired, a vault that cannot be read, a
// plugin nobody consented to. The caller turns this into a permission decision, so a missing record
// must never read as consent.
//
// The boolean maps this replaced are deliberately not consulted as a fallback. A vault still holding
// one, because the migration has not run or failed, must not open anything on its own, or the switch
// to grants would be cosmetic.
func (p *PluginVaultInbound) grantedTo(pluginID string) domainplugin.PermissionSet {
	if p.settings == nil {
		return domainplugin.PermissionSet{}
	}
	settings, err := p.settings.PluginSettings()
	if err != nil {
		return domainplugin.PermissionSet{}
	}
	grant, _ := settings.GrantFor(pluginID)
	return domainplugin.NewPermissionSet(grant.Granted)
}

// There is deliberately no coarse "does this plugin have any secret consent" check before the
// request is parsed, where the boolean it replaced used to sit. The per-field check that follows
// subsumes it entirely, so no test could tell the coarse one from its absence - and a branch nothing
// can falsify is one that rots. What it bought was denying a plugin before parsing its parameters,
// which is worth less than a guard whose correctness cannot be demonstrated.

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
		if user == nil || user.PassAuth == nil || user.PassAuth.VaultRef == "" {
			return nil, domainplugin.ErrCapabilityDenied
		}
		return p.passwordRepo.Get(ctx, user.PassAuth.VaultRef)
	case "privateKey":
		return p.resolvePrivateKey(ctx, conn)
	case "passphrase":
		return p.resolveKeyPassphrase(ctx, conn)
	default:
		return nil, domainplugin.ErrCapabilityDenied
	}
}

// resolvePrivateKey hands a plugin the stored key bytes, but only for a key whose owner has said
// so.
//
// Until the key manager existed there was no way to say otherwise: any plugin holding the vault
// capability could read any private key belonging to the connection it was invoked for. The
// capability grant is about the vault as a whole, which is far too coarse a decision for "this
// third-party binary may read this particular private key", so the answer now lives on the key.
//
// The default is off. A key that predates the flag, or whose owner never thought about it, is not
// handed over — the safe reading of silence is refusal, and a plugin that genuinely needs a key
// can say so and be granted it explicitly.
func (p *PluginVaultInbound) resolvePrivateKey(ctx context.Context, conn *domain.Connection) ([]byte, error) {
	user := conn.DefaultUser()
	if user == nil || user.KeyAuth == nil || len(user.KeyAuth.IdentityIDs) == 0 {
		return nil, domainplugin.ErrCapabilityDenied
	}
	identityID := user.KeyAuth.IdentityIDs[0]
	identity, err := p.identRepo.Get(ctx, identityID)
	if err != nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	if !identity.AllowPlugins {
		return nil, domainplugin.ErrCapabilityDenied
	}
	p.recordKeyRelease(ctx, identity)
	return p.identRepo.GetKeyBlob(ctx, identityID)
}

// recordKeyRelease notes that a private key left the vault for a plugin. It is the only trace of
// an event the user cannot otherwise observe, so it records the key by fingerprint - never the
// key, and never the passphrase.
func (p *PluginVaultInbound) recordKeyRelease(ctx context.Context, identity *domain.SSHIdentity) {
	p.mu.RLock()
	audit := p.keyAudit
	p.mu.RUnlock()
	if audit == nil {
		return
	}
	audit.RecordKeyEvent(ctx, KeyEventPluginAccess, identity.ID, identity.Fingerprint)
}

func (p *PluginVaultInbound) resolveKeyPassphrase(ctx context.Context, conn *domain.Connection) ([]byte, error) {
	user := conn.DefaultUser()
	if user == nil || user.KeyAuth == nil || len(user.KeyAuth.IdentityIDs) == 0 {
		return nil, domainplugin.ErrCapabilityDenied
	}
	identityID := user.KeyAuth.IdentityIDs[0]
	// The passphrase is gated on the same per-key flag as the key itself. Handing it over on its
	// own looks harmless only until you notice the plugin can ask for both in either order.
	identity, err := p.identRepo.Get(ctx, identityID)
	if err != nil || !identity.AllowPlugins {
		return nil, domainplugin.ErrCapabilityDenied
	}
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
