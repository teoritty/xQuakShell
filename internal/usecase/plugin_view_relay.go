package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

const viewRelayTokenTTL = 30 * time.Minute

type viewPanelToken struct {
	pluginID  string
	panelID   string
	expiresAt time.Time
}

// PluginViewRelay issues short-lived tokens for trusted WebView panel relay.
type PluginViewRelay struct {
	manager  *PluginManager
	registry *PluginRegistry
	mu       sync.Mutex
	tokens   map[string]viewPanelToken
}

// NewPluginViewRelay creates a view relay over the plugin manager.
func NewPluginViewRelay(manager *PluginManager, registry *PluginRegistry) *PluginViewRelay {
	return &PluginViewRelay{
		manager:  manager,
		registry: registry,
		tokens:   make(map[string]viewPanelToken),
	}
}

// PreparePanel registers a mounted panel and returns a relay token valid until first use or release.
func (r *PluginViewRelay) PreparePanel(pluginID, panelID string) (string, error) {
	if r == nil || r.manager == nil {
		return "", fmt.Errorf("plugin view relay unavailable")
	}
	if _, err := r.registry.ViewEntry(pluginID, panelID); err != nil {
		return "", err
	}
	token, err := newViewRelayToken()
	if err != nil {
		return "", err
	}

	r.mu.Lock()
	r.tokens[token] = viewPanelToken{
		pluginID:  pluginID,
		panelID:   panelID,
		expiresAt: time.Now().Add(viewRelayTokenTTL),
	}
	r.mu.Unlock()

	r.manager.RegisterViewPanel(pluginID, panelID)
	return token, nil
}

// RelayMessage forwards a UI message to the plugin using a valid relay token (consumed on success).
func (r *PluginViewRelay) RelayMessage(ctx context.Context, token string, message json.RawMessage) error {
	pluginID, panelID, err := r.consumeToken(token)
	if err != nil {
		return err
	}
	return r.manager.PostViewMessage(ctx, pluginID, panelID, message)
}

// ReleasePanel unregisters a panel and invalidates its relay token.
func (r *PluginViewRelay) ReleasePanel(token string) {
	if r == nil {
		return
	}
	pluginID, panelID, err := r.revokeToken(token)
	if err != nil {
		return
	}
	r.manager.UnregisterViewPanel(pluginID, panelID)
}

func (r *PluginViewRelay) validateToken(token string) (pluginID, panelID string, err error) {
	if token == "" {
		return "", "", domainplugin.ErrViewRelayTokenInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.tokens[token]
	if !ok {
		return "", "", domainplugin.ErrViewRelayTokenInvalid
	}
	if time.Now().After(entry.expiresAt) {
		delete(r.tokens, token)
		return "", "", domainplugin.ErrViewRelayTokenInvalid
	}
	return entry.pluginID, entry.panelID, nil
}

func (r *PluginViewRelay) consumeToken(token string) (pluginID, panelID string, err error) {
	if token == "" {
		return "", "", domainplugin.ErrViewRelayTokenInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.tokens[token]
	if !ok {
		return "", "", domainplugin.ErrViewRelayTokenInvalid
	}
	delete(r.tokens, token)
	if time.Now().After(entry.expiresAt) {
		return "", "", domainplugin.ErrViewRelayTokenInvalid
	}
	return entry.pluginID, entry.panelID, nil
}

func (r *PluginViewRelay) revokeToken(token string) (pluginID, panelID string, err error) {
	if token == "" {
		return "", "", domainplugin.ErrViewRelayTokenInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.tokens[token]
	if !ok {
		return "", "", domainplugin.ErrViewRelayTokenInvalid
	}
	delete(r.tokens, token)
	return entry.pluginID, entry.panelID, nil
}

func newViewRelayToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
