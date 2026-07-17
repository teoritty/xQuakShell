package main

import (
	"fmt"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
)

// errChannelExecConsentUnavailable and errChannelEmbedSinkUnavailable name the two purposes this
// composition root cannot construct honestly yet, so a channel.open for them fails at the door
// (rule 9) instead of returning an id wired to nothing.
//
// exec: NewChannelExecBackend takes consentGranted, which is documented as the plugin's
// install-time exec consent. That grant does not exist anywhere -- not in the install DTO, not in
// AppAPI.InstallPlugin's gate, not in PluginManager.Install, and InstalledPlugin persists no
// grants. Passing true would make an install-time security gate decorative, which is the same
// defect class as a manifest field enforced nowhere; passing false would deny every exec channel
// while pretending the wiring exists. Neither is honest, so exec is withheld until the grant is
// real end to end (ADR-011 readiness D3).
//
// embed-stream: NewChannelEmbedBackend requires an EmbedFrameSink, and EmbedTunnelService does not
// implement it -- SubscribeOutbound, the browser->plugin half, exists only on a test fake. Wiring
// the sink means implementing that method, which carries its own decision about routing input to
// exactly one transport, and is not this change's to make (readiness D1).
var (
	errChannelExecConsentUnavailable = fmt.Errorf(
		"%w: exec channels need an install-time consent grant, which is not plumbed yet",
		domainplugin.ErrNotImplemented)
	errChannelEmbedSinkUnavailable = fmt.Errorf(
		"%w: embed-stream needs an EmbedFrameSink; EmbedTunnelService has no SubscribeOutbound",
		domainplugin.ErrNotImplemented)
)

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
func newChannelResolverFor(audit domainplugin.ChannelAuditRecorder) func(domainplugin.InstalledPlugin, string) capability.ChannelBackendResolver {
	return func(plugin domainplugin.InstalledPlugin, _ string) capability.ChannelBackendResolver {
		// Closed over per process: everything a backend needs beyond the purpose string.
		pluginID := plugin.Manifest.ID
		network := plugin.Manifest.Capabilities.Network

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
				return nil, errChannelEmbedSinkUnavailable
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
