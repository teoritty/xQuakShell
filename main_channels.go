package main

import (
	"fmt"
	"sync"

	domainplugin "xquakshell/internal/domain/plugin"
	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/infra/plugin/capability"
	"xquakshell/internal/usecase"
)

// errChannelExecSessionsUnavailable reports an exec channel.open that arrived before the session
// registry was wired in. It is a startup-ordering guard, not a capability decision: an exec channel
// cannot find its parent session's SSH client without the registry, and rule 9 says say so at
// channel.open rather than let the plugin discover it on its first frame.
var errChannelExecSessionsUnavailable = fmt.Errorf(
	"%w: exec channels are not available until the session registry is wired",
	domainplugin.ErrNotImplemented)

// sessionRegistryHolder makes the session registry reachable from the channel resolver despite
// being constructed after it.
//
// The ordering: newPluginRuntime -- and with it ChannelResolverFor -- is built before
// NewSessionManager, which constructs the *usecase.SessionRegistry privately and exposes no
// accessor. The exec backend needs that registry to find its parent session's authenticated SSH
// client. Rather than reorder the composition root or widen SessionRegistry's exposure, the
// registry is PUSHED here once it exists, exactly as SessionManager already pushes it to
// EmbedTunnelService (SetEmbedTunnelService -> WireSessionContext), and the resolver reads it at
// call time -- the same shape as processChannelCloseNotifier below.
//
// A miss means the registry is not wired yet, which the resolver turns into a refused channel.open
// rather than a backend that dereferences nil on its first frame.
type sessionRegistryHolder struct {
	mu sync.Mutex
	r  *usecase.SessionRegistry
}

func newSessionRegistryHolder() *sessionRegistryHolder { return &sessionRegistryHolder{} }

func (h *sessionRegistryHolder) set(r *usecase.SessionRegistry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.r = r
}

func (h *sessionRegistryHolder) get() *usecase.SessionRegistry {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.r
}

// ProtocolForSession lets the discovery leader ask which protocol a session speaks (ADR-014
// parentProtocols addressing) through the same push-me-later indirection the exec backend uses:
// the leader is built with the plugin runtime, long before NewSessionManager creates the registry.
// A miss simply means no plugin is addressable for that connection yet, which is the correct
// answer while the session is still coming up.
func (h *sessionRegistryHolder) ProtocolForSession(sessionID string) (string, bool) {
	registry := h.get()
	if registry == nil {
		return "", false
	}
	return registry.ProtocolForSession(sessionID)
}

// ConnectionForSession lets the surface service resolve the connection a new surface belongs to,
// through the same push-me-later indirection. A miss means the session is not (or no longer) live,
// so no surface may be opened against it.
func (h *sessionRegistryHolder) ConnectionForSession(sessionID string) (string, bool) {
	registry := h.get()
	if registry == nil {
		return "", false
	}
	return registry.ConnectionForSession(sessionID)
}

// processChannelCloseNotifier is ONE plugin process's channel.close notifier, and the meeting point
// for two halves of a cycle neither side can close over.
//
// The cycle: ChannelResolverFor is called while the process's Conn is still being built, so a
// resolver cannot be handed Conn.Notify; the host delivers it moments later, after the Conn exists.
// A backend built by the resolver therefore gets a stable indirection — resolved at CALL time, long
// after channel.open — rather than the notifier value itself.
//
// It is allocated inside the factory call that builds the process's resolver, and reachable only
// from that call's two return values. That is deliberate, and it is what replaced a map keyed by
// (pluginID, sessionID): process keys are REUSED — a plugin stopped and started again comes back
// under the same key — so a shared map made the pairing depend on timing. A start that was
// overtaken would write its notifier over the live process's entry, and every channel.close of the
// running plugin would then be aimed at a dead Conn: silently, since a dropped notification fails
// nothing. Nothing was ever removed from that map either, so a stale entry outlived its process by
// design. With one holder per factory call there is no key to collide on, nothing to clean up, and
// no ordering to get right — the same shape the rest of the host already uses, where each process
// owns its own ChannelProxy rather than a share of a common structure.
//
// A notifier that has not arrived yet is a no-op rather than nil: a backend reporting a close
// reason has nothing left to do about a process whose Conn is already gone, and a closed channel
// must not depend on the notification landing (7.5).
type processChannelCloseNotifier struct {
	mu     sync.Mutex
	notify infraplugin.ChannelCloseNotify
}

// attach is handed to the host as infraplugin.AttachChannelCloseNotify.
func (n *processChannelCloseNotifier) attach(notify infraplugin.ChannelCloseNotify) {
	n.mu.Lock()
	n.notify = notify
	n.mu.Unlock()
}

// notifier converts to the usecase side. infra's ChannelCloseNotify and usecase's
// ChannelCloseNotifier are structurally identical but deliberately distinct types — infra must not
// import usecase — so this conversion is the composition root's job, and happens only here.
func (n *processChannelCloseNotifier) notifier() usecase.ChannelCloseNotifier {
	return func(channelID uint32, reason, message string) {
		n.mu.Lock()
		notify := n.notify
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
// Each call returns the process's resolver together with the attach that delivers that process's
// channel.close notifier once its Conn exists; see processChannelCloseNotifier for why the notifier
// cannot simply be passed in as a value, and why it is per call rather than looked up by key.
//
// sessions supplies the session registry the exec backend runs its commands over, once
// NewSessionManager has built it; see sessionRegistryHolder for the same reason.
func newChannelResolverFor(audit domainplugin.ChannelAuditRecorder, embedSink usecase.EmbedFrameSink, sessions *sessionRegistryHolder) func(domainplugin.InstalledPlugin, string) (capability.ChannelBackendResolver, infraplugin.AttachChannelCloseNotify) {
	return func(plugin domainplugin.InstalledPlugin, sessionID string) (capability.ChannelBackendResolver, infraplugin.AttachChannelCloseNotify) {
		// Closed over per process: everything a backend needs beyond the purpose string.
		pluginID := plugin.Manifest.ID
		network := plugin.Manifest.Capabilities.Network
		hasEmbedCap := plugin.Manifest.Capabilities.Session != nil && plugin.Manifest.Capabilities.Session.Embed
		// The exec backend's consentGranted traces to the user's install-time grant, and this is
		// the whole of that trace: every install path (AppAPI.InstallPlugin and the GitHub path,
		// both through PluginManager.Install) REFUSES to install a plugin whose manifest declares
		// the exec purpose unless the user granted exec access, so a plugin that is both installed
		// and declaring exec is one whose exec access was granted explicitly. That refusal is why
		// no grant is persisted (ADR-011 D3) -- and it is what makes this predicate a real gate
		// rather than the hardcoded true rule 8 forbids: remove the gate from Install and this
		// value stops meaning consent.
		var execCommands []domainplugin.ExecCommandTemplate
		if plugin.Manifest.Capabilities.Channel != nil {
			execCommands = plugin.Manifest.Capabilities.Channel.ExecCommands
		}
		execConsent := plugin.Manifest.RequiresChannelExecConsent()
		// embed-stream closes with a reason when its wait ceiling expires; channel.close is the
		// only way a plugin can learn why (ADR-011 has no binary error frame), so a nil notifier
		// here would make that reason unobservable. One holder per call: it belongs to the process
		// whose resolver is being built right here, and to nothing else.
		closeNotifier := &processChannelCloseNotifier{}
		notifyClose := closeNotifier.notifier()

		// A NEW backend on every call, never one shared per process or per purpose. All four are
		// stateful and single-use: they store the resolved target/argv/tunnelId in fields during
		// Authorize and refuse reuse after CloseRemote, so a shared instance lets the second
		// channel.open overwrite the first's target and both channels silently converge on one
		// peer. This is pinned by TestChannelResolverBuildsAFreshBackendPerOpen, not by this
		// comment.
		resolve := func(purpose string) (domainplugin.ChannelPurposeBackend, error) {
			switch purpose {
			case domainplugin.PurposeTCPRelay:
				// Dials directly from the host, not through the parent SSH connection, and
				// validates the hint through the same dial policy NetProxy enforces for net.dial.
				return capability.NewChannelRelayBackend(pluginID, network, audit), nil
			case domainplugin.PurposeUDPRelay:
				return capability.NewChannelUDPRelayBackend(pluginID, network, audit), nil
			case domainplugin.PurposeExec:
				// The close notifier is not optional here: a remote process that exits reports its
				// reason and captured stderr only through channel.close, and exec was the
				// motivating consumer B8 built it for (3.10).
				var registry *usecase.SessionRegistry
				if sessions != nil {
					registry = sessions.get()
				}
				if registry == nil {
					// Only reachable if a plugin process opened an exec channel before
					// wireEmbed pushed the registry in. Refusing at channel.open (rule 9) beats
					// handing back a backend whose first act is to dereference nil.
					return nil, errChannelExecSessionsUnavailable
				}
				return usecase.NewChannelExecBackend(pluginID, execCommands, execConsent, registry, audit, notifyClose), nil
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
		return resolve, closeNotifier.attach
	}
}
