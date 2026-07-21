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
	EventPluginContributionsChanged = "PluginContributionsChanged"
	EventPluginViewMessage          = "PluginViewMessage"
	EventPluginStateChanged         = "PluginStateChanged"
	EventSessionEmbedReady          = "SessionEmbedReady"
	EventDebugLogWindowChanged      = "DebugLogWindowChanged"
)

// TerminalOutputPayload carries terminal output data for a specific session.
type TerminalOutputPayload struct {
	SessionID string `json:"sessionId"`
	Output    string `json:"output"`
}

// TransferProgressPayload carries progress for a long-running operation.
// TECH DEBT: named "Transfer" for pragmatic reuse of the existing event/store/
// panel; it now also carries delete/chmod/chown operations (see Kind). A future
// refactor should rename this vocabulary to a generic "Operation".
type TransferProgressPayload struct {
	ID         string `json:"id"`
	SessionID  string `json:"sessionId"`
	Kind       string `json:"kind"`
	Direction  string `json:"direction"`
	LocalPath  string `json:"localPath"`
	RemotePath string `json:"remotePath"`
	RefreshDir string `json:"refreshDir,omitempty"`
	Done       int64  `json:"done"`
	Total      int64  `json:"total"`
	State      string `json:"state"`
}
