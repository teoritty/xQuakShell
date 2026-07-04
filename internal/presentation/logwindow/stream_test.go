package logwindow

import (
	"testing"
	"time"

	"ssh-client/internal/infra/loghub"
)

func TestEncodeDecodeLine(t *testing.T) {
	entry := loghub.Entry{
		Time:    time.Now().UTC(),
		Level:   "info",
		Source:  "core",
		Message: "hello",
		Fields:  map[string]string{"k": "v"},
	}
	line, err := EncodeLine(entry)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeLine(line[:len(line)-1])
	if err != nil {
		t.Fatal(err)
	}
	if got.Message != entry.Message || got.Source != entry.Source {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}
