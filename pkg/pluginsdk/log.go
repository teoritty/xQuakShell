package pluginsdk

import (
	"fmt"
)

// LogInfo writes a structured log line to the core host without embedding secrets in free text.
func LogInfo(host *Host, message string, fields map[string]string) error {
	if host == nil {
		return fmt.Errorf("plugin host unavailable")
	}
	_, err := host.CallCore("log.write", map[string]any{
		"level":   "info",
		"message": message,
		"fields":  fields,
	})
	return err
}

// LogWarn writes a structured warning log line to the core host.
func LogWarn(host *Host, message string, fields map[string]string) error {
	if host == nil {
		return fmt.Errorf("plugin host unavailable")
	}
	_, err := host.CallCore("log.write", map[string]any{
		"level":   "warn",
		"message": message,
		"fields":  fields,
	})
	return err
}
