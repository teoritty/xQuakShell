package logwindow

import (
	"encoding/json"

	"xquakshell/internal/domain"
)

// EncodeLine serializes an entry as NDJSON.
func EncodeLine(e domain.DebugLogEntry) ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// DecodeLine parses one NDJSON log entry.
func DecodeLine(data []byte) (domain.DebugLogEntry, error) {
	var e domain.DebugLogEntry
	err := json.Unmarshal(data, &e)
	return e, err
}
