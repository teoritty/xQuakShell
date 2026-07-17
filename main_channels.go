package main

import (
	"fmt"
	"sync"

	domainplugin "ssh-client/internal/domain/plugin"
	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/infra/plugin/capability"
	"ssh-client/internal/usecase"
)

// errChannelExecConsentUnavailable names the one purpose this composition root cannot construct
// honestly yet, so a channel.open for it fails at the door (rule 9) instead of returning an id
// wired to nothing.
//
// exec: NewChannelExecBackend takes consentGranted, which is documented as the plugin's
// install-time exec consent. That grant does not exist anywhere -- not in the install DTO, not in
// AppAPI.InstallPlugin's gate, not in PluginManager.Install, and InstalledPlugin persists no
// grants. Passing true would make an install-time security gate decorative, which is the same
// defect class as a manifest field enforced nowhere; passing false would deny every exec channel
// while pretending the wiring exists. Neither is honest, so exec is withheld until the grant is
// real end to end (ADR-011 readiness D3).
var errChannelExecConsentUnavailable = fmt.Errorf(
	"%w: exec channels need an install-time consent grant, which is not plumbed yet",
	domainplugin.ErrNotImplemented)

// channelCloseNotifiers holds one channel.close notifier per plugin process, so the two halves of
// a cycle neither side can close over can meet.
//
// The cycle: ChannelResolverFor is called while the process's Conn is still being built, so a
// resolver cannot be handed Conn.Notify; AttachChannelCloseNotifier delivers it moments later,
// after the Conn exists. A backend built by the resolver therefore gets a stable indirection —
// looked up at CALL time, long after channel.open — rather than the notifier value itself. Keyed
// per (plugin, session) because that is the granularity a plugin process has.
//
// A miss returns a no-op rather than nil: a backend reporting a close reason has nothing left to
// do about a process whose Conn is already gone, and a closed channel must not depend on the
// notification landing (7.5).
type channelCloseNotifiers struct {
	mu sync.Mutex
	by map[string]infraplugin.ChannelCloseNotify
}

func newChannelCloseNotifiers() *channelCloseNotifiers {
	return &channelCloseNotifiers{by: make(map[string]infraplugin.ChannelCloseNotify)}
}

func channelProcessKey(pluginID, sessionID string) string { return pluginID + "\x00" + sessionID }

func (n *channelCloseNotifiers) attach(plugin domainplugin.InstalledPlugin, sessionID string, notify infraplugin.ChannelCloseNotify) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.by[channelProcessKey(plugin.Manifest.ID, sessionID)] = notify
}

// notifierFor returns a usecase.ChannelCloseNotifier that resolves the process's real notifier at
// call time. infra's ChannelCloseNotify and usecase's ChannelCloseNotifier are structurally
// identical but deliberately distinct types — infra must not import usecase — so this conversion
// is the composition root's job, and happens only here.
func (n *channelCloseNotifiers) notifierFor(pluginID, sessionID string) usecase.ChannelCloseNotifier {
	return func(channelID uint32, reason, message string) {
		n.mu.Lock()
		notify := n.by[channelProcessKey(pluginID, sessionID)]
		n.mu.Unlock()
		if notify != nil {
			notify(channelID, reason, message)
		}
	}
}

// newChannelResolverFor builds the per-plugin-process factory HostConfig.ChannelResolverFor
// expects (ADR-011 Stages 6-8).
//
// It lives in the composition root, and only there, because it is the one place allowed to know
// both halves of the channel bus: the exec and embed-stream backends live in usecase, the relay
// backends live in infra/plugin/capability, and capability must never import usecase (rule 1,
// enforced by test/unit/plugin/architecture_security_test.go).
//
// audit is the same ChannelAuditRecorder the proxy records channel.open/close on: the backends
// additionally record the target they actually reached, which the proxy cannot know (it sees the
// plugin's hint, not the resolved address).
//
// embedSink is the host's one EmbedTunnelService: the embed-stream backend does not own the embed
// surface, it attaches a channel to the surface session.registerEmbed already established, and
// Authorize refuses a session this plugin never registered for.
//
// notifiers supplies each process's channel.close notifier once its Conn exists; see
// channelCloseNotifiers for why it cannot simply be passed in as a value.
func newChannelResolverFor(audit domainplugin.ChannelAuditRecorder, embedSink usecase.EmbedFrameSink, notifiers *channelCloseNotifiers) func(domainplugin.InstalledPlugin, string) capability.ChannelBackendResolver {
	return func(plugin domainplugin.InstalledPlugin, sessionID string) capability.ChannelBackendResolver {
		// Closed over per process: everything a backend needs beyond the purpose string.
		pluginID := plugin.Manifest.ID
		network := plugin.Manifest.Capabilities.Network
		hasEmbedCap := plugin.Manifest.Capabilities.Session != nil && plugin.Manifest.Capabilities.Session.Embed
		// embed-stream closes with a reason when its wait ceiling expires; channel.close is the
		// only way a plugin can learn why (ADR-011 has no binary error frame), so a nil notifier
		// here would make that reason unobservable.
		var notifyClose usecase.ChannelCloseNotifier
		if notifiers != nil {
			notifyClose = notifiers.notifierFor(pluginID, sessionID)
		}

		// A NEW backend on every call, never one shared per process or per purpose. All four are
		// stateful and single-use: they store the resolved target/argv/tunnelId in fields during
		// Authorize and refuse reuse after CloseRemote, so a shared instance lets the second
		// channel.open overwrite the first's target and both channels silently converge on one
		// peer. This is pinned by TestChannelResolverBuildsAFreshBackendPerOpen, not by this
		// comment.
		return func(purpose string) (domainplugin.ChannelPurposeBackend, error) {
			switch purpose {
			case domainplugin.PurposeTCPRelay:
				// Dials directly from the host, not through the parent SSH connection, and
				// validates the hint through the same dial policy NetProxy enforces for net.dial.
				return capability.NewChannelRelayBackend(pluginID, network, audit), nil
			case domainplugin.PurposeUDPRelay:
				return capability.NewChannelUDPRelayBackend(pluginID, network, audit), nil
			case domainplugin.PurposeExec:
				return nil, errChannelExecConsentUnavailable
			case domainplugin.PurposeEmbedStream:
				return usecase.NewChannelEmbedBackend(pluginID, hasEmbedCap, embedSink, audit, notifyClose), nil
			default:
				// The proxy already refuses a purpose the manifest does not declare, and refuses
				// it before reaching here. This is the other case: a purpose that is declarable
				// but that this host has no backend for at all.
				return nil, fmt.Errorf("%w: no backend for channel purpose %q",
					domainplugin.ErrNotImplemented, purpose)
			}
		}
	}
}
