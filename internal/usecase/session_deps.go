package usecase

import "xquakshell/internal/domain"

// SSHSessionDeps groups dependencies for SSH session setup (implementations are wired in main).
type SSHSessionDeps struct {
	PassphraseCache        domain.PassphraseCache
	HostKeyCallbackBuilder domain.HostKeyCallbackBuilder
	JumpTransportBuilder   domain.JumpTransportBuilder
	Keys                   *KeyManagerService
	MigrationDeps          domain.MigrationDeps
	PTYBridgeFactory       domain.PTYBridgeFactory
	SFTPClientFactory      domain.SFTPClientFactory
}
