//go:build linux

package plugin

func applyPluginResourceLimits(pid int, _ pluginJob) error {
	return applyLinuxResourceLimits(pid)
}
