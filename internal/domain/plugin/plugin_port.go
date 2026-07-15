package plugin

import (
	"context"
	"encoding/json"
	"time"
)

// ProcessState describes a plugin OS process lifecycle.
type ProcessState string

const (
	ProcessDiscovered ProcessState = "discovered"
	ProcessStarting   ProcessState = "starting"
	ProcessRunning    ProcessState = "running"
	ProcessStopping   ProcessState = "stopping"
	ProcessStopped    ProcessState = "stopped"
	ProcessCrashed    ProcessState = "crashed"
	ProcessSuspended  ProcessState = "suspended"
)

// InitializeParams is sent to a plugin on first start.
type InitializeParams struct {
	PluginID string `json:"pluginId"`
	// APIVersion is the frozen protocol envelope version (PluginAPIVersion). Kept for plugins
	// that read the scalar; API carries the full versioned surface.
	APIVersion string `json:"apiVersion"`
	// API is the host's advertised versioning descriptor (ADR-012): the envelope version plus
	// every capability's version and feature flags. The host — never the plugin's echo — is the
	// authority; the plugin negotiates against this.
	API          APIDescriptor `json:"api"`
	Capabilities CapabilitySet `json:"capabilities"`
	DataDir      string        `json:"dataDir"`
	// CoreVersion is the informational/legacy core build version.
	CoreVersion string `json:"coreVersion"`
}

// ProcessInstance identifies a running plugin OS process tracked by the host.
type ProcessInstance struct {
	PluginID  string
	SessionID string
	State     ProcessState
}

// SessionRPCAuthorizer enforces plugin session RPC scope and bound sessions (usecase implements).
type SessionRPCAuthorizer interface {
	BindSession(pluginID, sessionID string) error
	UnbindSession(pluginID, sessionID string)
	AuthorizeSessionRPC(pluginID, processSessionID string, isolation IsolationMode, allowMultiSession bool, targetSessionID string) error
}

// AuthAttemptAuthorizer validates in-flight SSH auth attempts before auth.* RPC (usecase implements).
// Authorize returns ErrSessionNotBound when the attempt is missing or not owned by the caller.
type AuthAttemptAuthorizer interface {
	Authorize(pluginID, attemptID, authMethodID, connectionID string) error
}

// ProcessHost manages out-of-process plugin lifecycles (infra implements this port).
type ProcessHost interface {
	Start(ctx context.Context, plugin InstalledPlugin, sessionID string) error
	Stop(ctx context.Context, pluginID, sessionID string) error
	StopAll(ctx context.Context)
	Call(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) (json.RawMessage, error)
	CallWithTimeout(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage, timeout time.Duration) (json.RawMessage, error)
	Notify(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) error
	State(pluginID, sessionID string) ProcessState
	RunningInstances() []ProcessInstance
	BindSession(pluginID, sessionID string) error
	UnbindSession(pluginID, sessionID string)
}
