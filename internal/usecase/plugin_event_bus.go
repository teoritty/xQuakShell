package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

const (
	defaultPublishRateLimit = 100 // max events.publish per second per plugin
)

// PluginSessionOwnershipChecker verifies plugin ownership of a session for event delivery.
type PluginSessionOwnershipChecker interface {
	PluginOwnsSession(pluginID, sessionID string) bool
}

// PluginEventNotifier delivers host→plugin notifications with session-aware process scope.
type PluginEventNotifier func(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) error

// PluginEventBus routes hub-and-spoke events between core and plugins (Phase 4).
type PluginEventBus struct {
	registry *PluginRegistry
	notify   PluginEventNotifier

	mu              sync.RWMutex
	subscriptions   map[string]map[string]struct{} // pluginID -> channels
	publishCount    map[string]int
	publishWindow   time.Time
	sessionActive   func(pluginID string) bool
	sessionOwner    PluginSessionOwnershipChecker
}

// NewPluginEventBus creates an event bus over the plugin registry and notify func.
func NewPluginEventBus(registry *PluginRegistry, notify PluginEventNotifier) *PluginEventBus {
	return &PluginEventBus{
		registry:      registry,
		notify:        notify,
		subscriptions: make(map[string]map[string]struct{}),
		publishCount:  make(map[string]int),
		publishWindow: time.Now(),
	}
}

// Subscribe implements domainplugin.EventInboundPort.
func (b *PluginEventBus) Subscribe(_ context.Context, pluginID, channel string) error {
	if b == nil {
		return domainplugin.ErrCapabilityDenied
	}
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return fmt.Errorf("%w: empty channel", domainplugin.ErrRPC)
	}

	plugin, err := b.registry.Get(pluginID)
	if err != nil {
		return err
	}
	caps := plugin.Manifest.Capabilities.Events
	if caps == nil || !channelAllowed(caps.Subscribe, channel) {
		return domainplugin.ErrCapabilityDenied
	}
	if !domainplugin.OwnsPluginEventChannel(pluginID, channel) {
		return domainplugin.ErrCapabilityDenied
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	channels, ok := b.subscriptions[pluginID]
	if !ok {
		channels = make(map[string]struct{})
		b.subscriptions[pluginID] = channels
	}
	channels[channel] = struct{}{}
	return nil
}

// PublishFromPlugin implements domainplugin.EventInboundPort.
func (b *PluginEventBus) PublishFromPlugin(ctx context.Context, pluginID, channel string, payload json.RawMessage) error {
	if b == nil {
		return domainplugin.ErrCapabilityDenied
	}
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return fmt.Errorf("%w: empty channel", domainplugin.ErrRPC)
	}

	plugin, err := b.registry.Get(pluginID)
	if err != nil {
		return err
	}
	caps := plugin.Manifest.Capabilities.Events
	if caps == nil || !channelAllowed(caps.Publish, channel) {
		return domainplugin.ErrCapabilityDenied
	}
	if !domainplugin.MayPublishToEventChannel(pluginID, channel) {
		return domainplugin.ErrCapabilityDenied
	}
	if !b.allowPublish(pluginID) {
		slog.Warn("plugin event publish rate limited", "pluginId", pluginID, "channel", channel)
		return domainplugin.ErrRateLimited
	}

	return b.deliver(ctx, channel, payload)
}

// PublishCore emits a core event to subscribed plugins.
func (b *PluginEventBus) PublishCore(ctx context.Context, channel string, payload any) {
	if b == nil {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("encode core event failed", "channel", channel, "err", err)
		return
	}
	if err := b.deliver(ctx, channel, raw); err != nil {
		slog.Debug("deliver core event", "channel", channel, "err", err)
	}
}

// SetSessionActiveChecker filters core.session.* delivery to plugins with active sessions.
func (b *PluginEventBus) SetSessionActiveChecker(fn func(pluginID string) bool) {
	b.mu.Lock()
	b.sessionActive = fn
	b.mu.Unlock()
}

// SetSessionOwnershipChecker filters core.session.* delivery to the owning plugin only.
func (b *PluginEventBus) SetSessionOwnershipChecker(checker PluginSessionOwnershipChecker) {
	b.mu.Lock()
	b.sessionOwner = checker
	b.mu.Unlock()
}

// ClearPlugin removes subscriptions when a plugin process stops.
func (b *PluginEventBus) ClearPlugin(pluginID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscriptions, pluginID)
	delete(b.publishCount, pluginID)
}

func (b *PluginEventBus) deliver(ctx context.Context, channel string, payload json.RawMessage) error {
	targets := b.subscribersFor(channel, payload)
	if len(targets) == 0 {
		return nil
	}

	params, err := json.Marshal(map[string]any{
		"channel": channel,
		"payload": json.RawMessage(payload),
	})
	if err != nil {
		return err
	}

	for _, pluginID := range targets {
		if b.notify == nil {
			continue
		}
		sessionID := ""
		if strings.HasPrefix(channel, "core.session.") {
			sessionID = extractCoreSessionID(payload)
		}
		if err := b.notify(ctx, pluginID, sessionID, "event", params); err != nil {
			slog.Debug("plugin event delivery failed", "pluginId", pluginID, "sessionId", sessionID, "channel", channel, "err", err)
		}
	}
	return nil
}

func (b *PluginEventBus) subscribersFor(channel string, payload json.RawMessage) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	requireActiveSession := strings.HasPrefix(channel, "core.session.")
	sessionID := extractCoreSessionID(payload)
	seen := make(map[string]struct{})
	var out []string
	for pluginID, channels := range b.subscriptions {
		if requireActiveSession {
			if sessionID == "" {
				continue
			}
			if b.sessionActive != nil && !b.sessionActive(pluginID) {
				continue
			}
			if b.sessionOwner != nil && !b.sessionOwner.PluginOwnsSession(pluginID, sessionID) {
				continue
			}
		}
		for sub := range channels {
			if channelMatches(sub, channel) {
				if _, ok := seen[pluginID]; !ok {
					seen[pluginID] = struct{}{}
					out = append(out, pluginID)
				}
				break
			}
		}
	}
	return out
}

func extractCoreSessionID(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var body struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return ""
	}
	return strings.TrimSpace(body.SessionID)
}

func (b *PluginEventBus) allowPublish(pluginID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if now.Sub(b.publishWindow) >= time.Second {
		b.publishWindow = now
		b.publishCount = make(map[string]int)
	}
	b.publishCount[pluginID]++
	return b.publishCount[pluginID] <= defaultPublishRateLimit
}

var _ domainplugin.EventInboundPort = (*PluginEventBus)(nil)
