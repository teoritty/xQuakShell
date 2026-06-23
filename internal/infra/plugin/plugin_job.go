package plugin

// pluginJob is a platform-specific sandbox handle for a plugin child process.
type pluginJob struct {
	handle uintptr
}
