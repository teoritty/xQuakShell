package plugin

import "errors"

var (
	// ErrInvalidManifest indicates plugin.json failed validation.
	ErrInvalidManifest = errors.New("invalid plugin manifest")

	// ErrPluginNotFound indicates no plugin with the given ID is registered.
	ErrPluginNotFound = errors.New("plugin not found")

	// ErrPluginNotRunning indicates the plugin process is not active.
	ErrPluginNotRunning = errors.New("plugin process not running")

	// ErrPluginAlreadyRunning indicates Start was called on a running plugin.
	ErrPluginAlreadyRunning = errors.New("plugin process already running")

	// ErrCapabilityDenied indicates the plugin lacks permission for an RPC method.
	ErrCapabilityDenied = errors.New("plugin capability denied")

	// ErrRPC indicates a JSON-RPC protocol or transport failure.
	ErrRPC = errors.New("plugin rpc error")

	// ErrNotImplemented indicates a capability is declared but not yet available.
	ErrNotImplemented = errors.New("plugin method not implemented")

	// ErrRateLimited indicates a plugin exceeded a resource rate limit.
	ErrRateLimited = errors.New("plugin event rate limited")

	// ErrHandleNotFound indicates a net handle is unknown or not owned by the caller.
	ErrHandleNotFound = errors.New("plugin handle not found")

	// ErrPluginDisabled indicates the user disabled the plugin.
	ErrPluginDisabled = errors.New("plugin disabled")

	// ErrTerminalBackpressure indicates plugin terminal output could not be delivered in time.
	ErrTerminalBackpressure = errors.New("plugin terminal output backpressure")

	// ErrNetworkDialFailed indicates a permitted net.dial target could not be reached.
	ErrNetworkDialFailed = errors.New("plugin network dial failed")

	// ErrIncompatibleCore indicates the host core version is below manifest minCoreVersion.
	ErrIncompatibleCore = errors.New("plugin incompatible with host core version")

	// ErrSessionNotBound indicates the plugin process is not authorized for the target session.
	ErrSessionNotBound = errors.New("plugin session not bound")

	// ErrVaultAuditFailed indicates a vault access audit record could not be persisted.
	ErrVaultAuditFailed = errors.New("plugin vault audit write failed")

	// ErrViewRelayTokenInvalid indicates a plugin view relay token is missing or expired.
	ErrViewRelayTokenInvalid = errors.New("plugin view relay token invalid")

	// ErrSessionScopeRequired indicates host IPC requires a session-scoped process key (per-session isolation).
	ErrSessionScopeRequired = errors.New("plugin session scope required")
)
