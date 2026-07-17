package main

import (
	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
)

// newChannelResolverFor builds the per-plugin-process factory HostConfig.ChannelResolverFor
// expects (ADR-011 Stage 6-8).
//
// It lives in the composition root, and only there, because it is the one place allowed to know
// both halves of the channel bus: the exec and embed-stream backends live in usecase, the relay
// backends live in infra/plugin/capability, and capability must never import usecase (rule 1,
// enforced by test/unit/plugin/architecture_security_test.go). Everything a backend needs beyond
// the purpose string — the manifest, the parent session — is closed over here, per process.
//
// Real purpose backends land in ADR-011 Stages 6-8; until then every channel.open is rejected
// after purpose/session validation, same as any other declared-but-unimplemented capability.
func newChannelResolverFor() func(domainplugin.InstalledPlugin, string) capability.ChannelBackendResolver {
	return func(domainplugin.InstalledPlugin, string) capability.ChannelBackendResolver {
		return func(string) (domainplugin.ChannelPurposeBackend, error) {
			return nil, domainplugin.ErrNotImplemented
		}
	}
}
