package usecase

import (
	"context"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// Deciding on what basis a plugin may reach an object is its own subject, separate from the RPC
// handlers that ask. It lives here so the gate file stays about what the verbs do and this one stays
// about who is allowed to call them.

// VaultLockState reports whether the vault is open.
//
// The scope anchor needs this and the session anchor does not, which is the whole difference between
// them: a session ends when the vault closes, while a scope is a position in a folder tree and
// survives a lock without any effort at all.
type VaultLockState interface {
	IsUnlocked() bool
}

// SetLockState binds the vault, so the scope anchor can tell whether it is open.
func (p *PluginVaultInbound) SetLockState(v VaultLockState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lockState = v
}

// anchorFor names the basis on which this plugin may reach this connection, or AnchorNone.
//
// Two bases, and no third. An active session the plugin owns is the original one and is unchanged.
// The scope is the second: the user put the object where this plugin can see it. What is deliberately
// absent is "the plugin asked" - a request is not a reason.
func (p *PluginVaultInbound) anchorFor(ctx context.Context, pluginID, connectionID string) domainplugin.Anchor {
	if p.checkOwnership(pluginID, connectionID) {
		return domainplugin.AnchorSession
	}
	if p.scopeAllows(ctx, pluginID, connectionID) {
		return domainplugin.AnchorScope
	}
	return domainplugin.AnchorNone
}

// scopeAllows reports whether the user placed this connection in the plugin's scope, with the vault
// open.
//
// The lock check is first and is not incidental. A scope survives a lock trivially, so without it
// this anchor would hand a plugin access after the vault closed that no plugin has today - through
// the mechanism added to constrain them. An unwired lock state refuses for the same reason: with no
// way to tell whether the vault is open, the safe reading is that it is not.
func (p *PluginVaultInbound) scopeAllows(ctx context.Context, pluginID, connectionID string) bool {
	p.mu.RLock()
	lock := p.lockState
	p.mu.RUnlock()
	if lock == nil || !lock.IsUnlocked() || p.settings == nil {
		return false
	}
	settings, err := p.settings.PluginSettings()
	if err != nil || len(settings.ScopeRoots) == 0 {
		return false
	}
	folders, err := p.connRepo.GetAllFolders(ctx)
	if err != nil {
		return false
	}
	index, err := domain.NewScopeIndex(folders, settings.ScopeRoots)
	if err != nil {
		return false
	}
	conn, err := p.connRepo.GetByID(ctx, connectionID)
	if err != nil || conn == nil {
		return false
	}
	return index.Contains(pluginID, conn.FolderID)
}
