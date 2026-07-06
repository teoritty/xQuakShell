package usecase

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDynamicForwardCoordinator_StopSessionClosesPreBindLocals(t *testing.T) {
	coord := NewDynamicForwardCoordinator(nil, nil)
	const sessionID = "sess-stop-prebind"

	local, peer := net.Pipe()
	t.Cleanup(func() { local.Close(); peer.Close() })

	ctx := context.Background()
	coord.StartDynamicForwardSessionForTest(ctx, sessionID, stubTunnelDialer{}, nil)

	coord.mu.Lock()
	sf := coord.sessions[sessionID]
	coord.mu.Unlock()
	if sf == nil || sf.service == nil {
		t.Fatal("session service not started")
	}

	sf.service.SetPreBindTimeoutForTest(time.Hour)

	const localConnID = "lc-stop-test"
	if err := sf.service.RegisterLocal(ctx, "plugin-a", "rule-1", "socks5", localConnID, local); err != nil {
		t.Fatalf("RegisterLocal: %v", err)
	}
	coord.mu.Lock()
	coord.localOwners[localConnID] = tunnelHandleOwner{sessionID: sessionID, pluginID: "plugin-a"}
	coord.mu.Unlock()

	if !sf.service.HasLocal(localConnID) {
		t.Fatal("expected pre-bind local before StopSession")
	}

	coord.StopSession(sessionID)

	if sf.service.HasLocal(localConnID) {
		t.Fatal("pre-bind local still registered after StopSession")
	}

	buf := make([]byte, 1)
	if _, err := peer.Read(buf); err == nil {
		t.Fatal("expected peer read error after StopSession")
	}
}
