package logwindow

import (
	"encoding/json"

	"ssh-client/internal/infra/loghub"
)

// EncodeLine serializes an entry as NDJSON.
func EncodeLine(e loghub.Entry) ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// DecodeLine parses one NDJSON log entry.
func DecodeLine(data []byte) (loghub.Entry, error) {
	var e loghub.Entry
	err := json.Unmarshal(data, &e)
	return e, err
}
