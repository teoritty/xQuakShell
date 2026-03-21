package usecase

import "ssh-client/internal/domain"

// SSHSessionDeps groups dependencies for SSH session setup (implementations are wired in main).
type SSHSessionDeps struct {
	PassphraseCache         domain.PassphraseCache
	HostKeyCallbackBuilder  domain.HostKeyCallbackBuilder
	JumpTransportBuilder    domain.JumpTransportBuilder
	PrivateKeySignerFactory domain.PrivateKeySignerFactory
}
