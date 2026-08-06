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

	// ErrIncompatibleAPI indicates the host cannot satisfy a plugin's declared pluginApi or
	// capability version requirement (wrong major, or host minor too low). Maps to RPC -32009.
	ErrIncompatibleAPI = errors.New("plugin incompatible with host API version")

	// ErrMissingFeature indicates the host does not offer a capability feature flag the plugin
	// requires. Maps to RPC -32010.
	ErrMissingFeature = errors.New("plugin requires an unavailable feature")

	// ErrSessionNotBound indicates the calling plugin is not authorized for the target
	// scoped resource: session RPC (sessionId) or in-flight SSH auth (attemptId).
	// AuthAttemptAuthorizer.Authorize MUST return this sentinel on IDOR (TZ 1.4).
	ErrSessionNotBound = errors.New("plugin session not bound")

	// ErrVaultAuditFailed indicates a vault access audit record could not be persisted.
	ErrVaultAuditFailed = errors.New("plugin vault audit write failed")

	// ErrViewRelayTokenInvalid indicates a plugin view relay token is missing or expired.
	ErrViewRelayTokenInvalid = errors.New("plugin view relay token invalid")

	// ErrSessionScopeRequired indicates host IPC requires a session-scoped process key (per-session isolation).
	ErrSessionScopeRequired = errors.New("plugin session scope required")

	// ErrAuthProviderBusy indicates too many concurrent auth attempts for one plugin.
	ErrAuthProviderBusy = errors.New("auth provider busy")

	// ErrAuthChallengeTimeout indicates a keyboard-interactive auth round exceeded the host timeout.
	ErrAuthChallengeTimeout = errors.New("auth challenge timeout")

	// ErrTunnelNotFound indicates a tunnel or local connection handle is unknown.
	ErrTunnelNotFound = errors.New("tunnel not found")

	// ErrTunnelAlreadyExists indicates a tunnelId is already registered in the pre-bind registry.
	ErrTunnelAlreadyExists = errors.New("tunnel already exists")

	// ErrInvalidRepositoryURL indicates a malformed or unsupported GitHub repository URL.
	ErrInvalidRepositoryURL = errors.New("invalid GitHub repository URL")

	// ErrRepositoryNotFound indicates the GitHub repository does not exist or is inaccessible.
	ErrRepositoryNotFound = errors.New("GitHub repository not found")

	// ErrNoReleases indicates the repository exists but has no published GitHub releases.
	ErrNoReleases = errors.New("no GitHub releases found")

	// ErrPluginManifestNotFound indicates xqsp.json was not found in the repository.
	ErrPluginManifestNotFound = errors.New("xqsp.json not found in repository")

	// ErrInvalidPluginMetadata indicates plugin metadata from GitHub is invalid.
	ErrInvalidPluginMetadata = errors.New("invalid plugin metadata")

	// ErrPlatformNotSupported indicates no binary exists for the current platform.
	ErrPlatformNotSupported = errors.New("plugin does not support current platform")

	// ErrBundleIdentityMismatch indicates a downloaded release bundle declares a different plugin
	// than the repository's xqsp.json did.
	ErrBundleIdentityMismatch = errors.New("release bundle does not match the repository manifest")

	// ErrUIPluginRequiresBundle indicates a plugin that ships ui/ assets is published for this
	// platform only as a bare binary, which cannot carry them.
	ErrUIPluginRequiresBundle = errors.New("plugin with a UI must be published as an .xqsp bundle")

	// ErrUIAssetMissing indicates a manifest declares a ui/ asset the plugin tree does not contain.
	ErrUIAssetMissing = errors.New("plugin declares a UI asset that is missing from the bundle")

	// ErrReleaseAssetNotFound indicates the requested release asset was not found.
	ErrReleaseAssetNotFound = errors.New("release asset not found")

	// ErrInvalidReleaseTag indicates the requested release tag is not published for the plugin.
	ErrInvalidReleaseTag = errors.New("invalid GitHub release tag")

	// ErrChecksumMismatch indicates downloaded binary checksum verification failed.
	ErrChecksumMismatch = errors.New("checksum verification failed")

	// ErrRepositoryNotTrusted indicates the repository is not marked as trusted.
	ErrRepositoryNotTrusted = errors.New("repository is not trusted")

	// ErrGitHubRateLimitExceeded indicates GitHub API rate limit was exceeded.
	ErrGitHubRateLimitExceeded = errors.New("GitHub API rate limit exceeded")
)
