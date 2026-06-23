package plugin

import (
	"context"
	"encoding/json"
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
	PluginID     string          `json:"pluginId"`
	APIVersion   string          `json:"apiVersion"`
	Capabilities CapabilitySet   `json:"capabilities"`
	DataDir      string          `json:"dataDir"`
	CoreVersion  string          `json:"coreVersion"`
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

// ProcessHost manages out-of-process plugin lifecycles (infra implements this port).
type ProcessHost interface {
	Start(ctx context.Context, plugin InstalledPlugin, sessionID string) error
	Stop(ctx context.Context, pluginID, sessionID string) error
	StopAll(ctx context.Context)
	Call(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) (json.RawMessage, error)
	Notify(ctx context.Context, pluginID, sessionID, method string, params json.RawMessage) error
	State(pluginID, sessionID string) ProcessState
	RunningInstances() []ProcessInstance
	BindSession(pluginID, sessionID string) error
	UnbindSession(pluginID, sessionID string)
}
