package plugin_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"ssh-client/internal/infra/plugin/ipc"
)

func TestConnParseErrorReturns32700(t *testing.T) {
	var hostOut bytes.Buffer
	conn := ipc.NewConn(strings.NewReader("{broken\n"), &hostOut, nil, nil)
	defer conn.Close()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(hostOut.String(), "-32700") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected -32700 in host response %q", hostOut.String())
}

func TestNewParseErrorResponse(t *testing.T) {
	msg := ipc.NewParseErrorResponse()
	if msg.Error == nil || msg.Error.Code != -32700 {
		t.Fatalf("unexpected parse error response: %+v", msg)
	}
	if msg.ID != nil {
		t.Fatalf("parse error id must be null")
	}
}
