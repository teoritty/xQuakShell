package wails

// Event name constants emitted from Go to the frontend.
const (
	EventSessionStateChanged        = "SessionStateChanged"
	EventSessionClosed              = "SessionClosed"
	EventTerminalOutput             = "TerminalOutput"
	EventRemoteTreeUpdated          = "RemoteTreeUpdated"
	EventTransferProgress           = "TransferProgress"
	EventVaultLocked                = "VaultLocked"
	EventFoldersUpdated             = "FoldersUpdated"
	EventConnectionsUpdated         = "ConnectionsUpdated"
	EventHostKeyRequired            = "HostKeyRequired"
	EventSFTPReady                  = "SFTPReady"
	EventTerminalReady              = "TerminalReady"
	EventPingUpdated                = "PingUpdated"
	EventTransferCompleted          = "TransferCompleted"
	EventFileEdited                 = "FileEdited"
	EventPluginContributionsChanged = "PluginContributionsChanged"
	EventPluginViewMessage          = "PluginViewMessage"
	EventPluginStateChanged         = "PluginStateChanged"
	EventSessionEmbedReady          = "SessionEmbedReady"
	EventDebugLogWindowChanged      = "DebugLogWindowChanged"
	EventDiscoveryTreeChanged       = "DiscoveryTreeChanged"
	EventPluginSurfaceOpened        = "PluginSurfaceOpened"
	EventPluginSurfaceOutput        = "PluginSurfaceOutput"
	EventPluginSurfaceChanged       = "PluginSurfaceChanged"
	EventPluginSurfaceClosed        = "PluginSurfaceClosed"
	EventPluginDialogOpened         = "PluginDialogOpened"
	EventPluginDialogClosed         = "PluginDialogClosed"
	EventPluginDialogError          = "PluginDialogError"
	EventPluginNodeDetails          = "PluginNodeDetails"
)

// DiscoveryTreeChangedPayload names the connection and the node whose branch changed (ADR-014).
// There is no sessionId here or anywhere else the frontend can see: sessions are how the host
// reaches a plugin, connections are how the user sees the tree.
type DiscoveryTreeChangedPayload struct {
	ConnectionID string `json:"connectionId"`
	// NodeID is the node whose children changed; "" means the connection root, i.e. the whole
	// subtree. It is present because updates are coalesced per node, not per connection.
	NodeID string `json:"nodeId"`
}

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
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Kind      string `json:"kind"`
	LocalPath string `json:"localPath"`
	// RemotePath is a display caption ("3 items" for a batch) — the frontend
	// must never parse it as a path. RefreshDir is the machine-readable
	// directory to reload and is always populated; see usecase.TransferProgress.
	RemotePath string `json:"remotePath"`
	RefreshDir string `json:"refreshDir"`
	Done       int64  `json:"done"`
	Total      int64  `json:"total"`
	State      string `json:"state"`
}
