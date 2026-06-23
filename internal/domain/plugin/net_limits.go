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

	// MaxPluginLogLinesPerSecond limits log.write RPC calls per plugin process.
	MaxPluginLogLinesPerSecond = 50

	// MaxPluginProcessMemoryBytes is the cross-platform memory cap for plugin processes.
	MaxPluginProcessMemoryBytes = 128 << 20
	// MaxPluginProcessOpenFiles is the cross-platform open-files cap for plugin processes.
	MaxPluginProcessOpenFiles = 64
	// MaxPluginProcessThreads is the cross-platform thread cap for plugin processes.
	MaxPluginProcessThreads = 64
)
