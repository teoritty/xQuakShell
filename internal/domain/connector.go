package domain

import "context"

// SessionConnector establishes a session for a given protocol.
// Each protocol (SSH, Telnet, Serial, RDP, HTTP) has its own connector implementation.
type SessionConnector interface {
	// Protocol returns the protocol this connector handles (e.g. ProtocolSSH).
	Protocol() string

	// Connect establishes the session. It may run asynchronously; on success it calls
	// setSSHClient/setRemoteFS/setPTYBridge as needed and updateState(Ready).
	// On error it calls updateState(Error, errMsg).
	Connect(ctx context.Context, conn *Connection, deps ConnectorDeps, hooks ConnectorHooks) error
}

// ConnectorHooks are callbacks the connector uses to update session state.
type ConnectorHooks struct {
	SetSSHClient   func(SSHClient)
	SetRemoteFS    func(RemoteFS)
	SetPTYBridge   func(TerminalPTYBridge)
	UpdateState    func(state SessionState, errMsg string)
	SetHostKeyInfo func(*HostKeyInfo)
	// OnStreamReady is called when a stream-based connector (Telnet, Serial) has started
	// the terminal bridge. The API uses this to begin streaming output to the frontend.
	OnStreamReady func(outputCh <-chan []byte)
}

// ConnectorDeps provides connectors with access to repositories and factories.
type ConnectorDeps struct {
	ConnRepo     ConnectionRepository
	VaultRepo    VaultRepository
	IdentRepo    IdentityRepository
	PasswordRepo PasswordRepository
	KnownHosts   KnownHostsRepository
	SSHFactory   SSHClientFactory
}
