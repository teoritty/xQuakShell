package domain

// SessionState represents the lifecycle state of a connection session (tab).
type SessionState string

const (
	// SessionConnecting means the SSH handshake is in progress.
	SessionConnecting SessionState = "connecting"
	// SessionHostKeyRequired means a host key decision is needed from the user.
	SessionHostKeyRequired SessionState = "hostkey-required"
	// SessionReady means SSH, PTY and SFTP are initialized and usable.
	SessionReady SessionState = "ready"
	// SessionError means the session encountered an unrecoverable error.
	SessionError SessionState = "error"
	// SessionClosed means the session has been terminated.
	SessionClosed SessionState = "closed"
)

// ConnectionSession tracks the runtime state of a single session (tab).
type ConnectionSession struct {
	// SessionID uniquely identifies this session.
	SessionID string `json:"sessionId"`
	// ConnectionID links to the Connection this session was opened for.
	ConnectionID string `json:"connectionId"`
	// ConnectionName is a cached display name for the tab.
	ConnectionName string `json:"connectionName"`
	// Protocol is the connection protocol (ssh, rdp, telnet, serial, http).
	Protocol string `json:"protocol,omitempty"`
	// State is the current lifecycle state.
	State SessionState `json:"state"`
	// ErrorMessage holds a human-readable error when State == SessionError.
	ErrorMessage string `json:"errorMessage,omitempty"`
}
