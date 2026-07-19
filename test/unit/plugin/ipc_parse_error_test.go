package plugin_test

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"ssh-client/internal/infra/plugin/ipc"
)

func TestConnParseErrorReturns32700(t *testing.T) {
	var hostOut bytes.Buffer

	broken := []byte("{broken")
	var hdr [9]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(broken)))
	hdr[4] = 0x01
	frame := append(hdr[:], broken...)

	conn := ipc.NewConn(bytes.NewReader(frame), &hostOut, nil, nil, 0)
	conn.Close()

	if !strings.Contains(hostOut.String(), "-32700") {
		t.Fatalf("expected -32700 in host response %q", hostOut.String())
	}
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
