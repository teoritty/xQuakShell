//go:build !windows

package plugin

func createPluginJob() (pluginJob, error) {
	return pluginJob{}, nil
}

func assignProcessToJob(_ pluginJob, pid int) error {
	trackPluginPID(pid)
	return nil
}

func closePluginJob(_ pluginJob) {}
