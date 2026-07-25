package domain

import "time"

// DebugLogEntry is a single log line for the debug log window.
type DebugLogEntry struct {
	Time    time.Time         `json:"time"`
	Level   string            `json:"level"`
	Source  string            `json:"source"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// LogStream is the read-side port for subscribing to debug log entries.
type LogStream interface {
	Subscribe(buffer int) (id int, backlog []DebugLogEntry, live <-chan DebugLogEntry)
	Unsubscribe(id int)
}

// LogLevelController is the write-side port for the debug log level. It takes
// the level as the same string the settings carry ("debug"/"info"/"warn"/
// "error"), so the domain stays free of any logging-backend vocabulary.
type LogLevelController interface {
	SetLevel(name string)
}
