package domain

// SessionState represents the lifecycle state of a connection session (tab).
type SessionState string

const (
	// SessionConnecting means the SSH handshake is in progress.
	SessionConnecting SessionState = "connecting"
	// SessionHostKeyRequired means a host key decision is needed from the user.
	SessionHostKeyRequired SessionState = "hostkey-required"
	// SessionTrustRequired means a plugin session is waiting on the user's
	// decision about the identity of the remote peer.
	//
	// A state of its own rather than a reuse of SessionHostKeyRequired: that
	// one belongs to the SSH path, and sharing it would mean a change to one
	// dialog silently altered the other.
	SessionTrustRequired SessionState = "trust-required"
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
	// Protocol is the connection protocol (ssh by default; plugin connectors may use other values).
	Protocol string `json:"protocol,omitempty"`
	// Surface is the session UI type: "terminal" or "embed" for plugin sessions.
	Surface string `json:"surface,omitempty"`
	// State is the current lifecycle state.
	State SessionState `json:"state"`
	// ErrorMessage holds a human-readable error when State == SessionError.
	ErrorMessage string `json:"errorMessage,omitempty"`
}
