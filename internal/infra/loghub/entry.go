package loghub

import "time"

// Entry is a single log line for the debug log window.
type Entry struct {
	Time    time.Time         `json:"time"`
	Level   string            `json:"level"`
	Source  string            `json:"source"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}
