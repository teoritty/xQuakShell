package plugin

import "time"

const (
	// MaxNetConnectionsPerPlugin limits concurrent outbound handles per plugin process.
	MaxNetConnectionsPerPlugin = 8
	// MaxNetReadBytes is the maximum payload returned by net.read per call.
	MaxNetReadBytes = MaxFrameBytes
	// MaxNetWriteBytes is the maximum payload accepted by net.write per call.
	MaxNetWriteBytes = MaxFrameBytes
	// NetDialTimeout is the maximum time to establish an outbound TCP connection.
	NetDialTimeout = 10 * time.Second
	// NetWriteTimeout is the maximum time for a single net.write on an established connection.
	NetWriteTimeout = 10 * time.Second

	// MaxPluginLogLinesPerSecond limits log.write RPC calls per plugin process.
	MaxPluginLogLinesPerSecond = 50

	// MaxConcurrentPluginRequests bounds how many inbound RPCs from one plugin the host will have
	// in flight at once.
	//
	// Every request the plugin sends is dispatched on its own host goroutine, and only log.write
	// was rate limited - so a plugin could pump requests at line rate and the host would grow a
	// goroutine for each. The 30s context deadline bounds how long a WELL-BEHAVED handler runs; a
	// proxy blocked on a syscall does not observe its context, and neither does the goroutine
	// holding it. The plugin process is memory-capped by its job object or rlimit. The host is not,
	// which is what makes this the host's problem rather than the plugin's.
	//
	// 64 is far above what any legitimate plugin needs - a batch of parallel fs reads is a handful
	// - and far below the point where goroutines and their proxy state cost anything noticeable.
	MaxConcurrentPluginRequests = 64

	// MaxPluginProcessMemoryBytes is the cross-platform memory cap for plugin processes.
	MaxPluginProcessMemoryBytes = 128 << 20
	// MaxPluginProcessOpenFiles is the cross-platform open-files cap for plugin processes.
	MaxPluginProcessOpenFiles = 64
	// MaxPluginProcessThreads is the cross-platform thread cap for plugin processes.
	MaxPluginProcessThreads = 64

	// DefaultMaxTunnelChannels is the default concurrent SSH channels per plugin for tunnel.dial.
	DefaultMaxTunnelChannels = 16
	// PreBindTunnelTimeout is how long a local client may wait before bind/close.
	PreBindTunnelTimeout = 10 * time.Second
)
