package wails

// Event name constants emitted from Go to the frontend.
const (
	EventSessionStateChanged = "SessionStateChanged"
	EventSessionClosed       = "SessionClosed"
	EventTerminalOutput      = "TerminalOutput"
	EventRemoteTreeUpdated   = "RemoteTreeUpdated"
	EventTransferProgress    = "TransferProgress"
	EventVaultLocked         = "VaultLocked"
	EventFoldersUpdated      = "FoldersUpdated"
	EventConnectionsUpdated  = "ConnectionsUpdated"
	EventHostKeyRequired     = "HostKeyRequired"
	EventSFTPReady           = "SFTPReady"
	EventTerminalReady       = "TerminalReady"
	EventPingUpdated         = "PingUpdated"
	EventTransferCompleted   = "TransferCompleted"
	EventFileEdited          = "FileEdited"
	EventRDPProcessExited    = "RDPProcessExited"
)

// TerminalOutputPayload carries terminal output data for a specific session.
type TerminalOutputPayload struct {
	SessionID string `json:"sessionId"`
	Output    string `json:"output"`
}

// TransferProgressPayload carries file transfer progress data.
type TransferProgressPayload struct {
	ID         string `json:"id"`
	SessionID  string `json:"sessionId"`
	Direction  string `json:"direction"`
	LocalPath  string `json:"localPath"`
	RemotePath string `json:"remotePath"`
	Done       int64  `json:"done"`
	Total      int64  `json:"total"`
	State      string `json:"state"`
}
