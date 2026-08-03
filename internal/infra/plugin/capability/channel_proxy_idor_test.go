package capability

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

// TestChannelProxy_CloseRejectsForeignChannelID is the required IDOR test, modeled 1:1 on
// TestTunnelLocalProxy_RejectUnknownLocal: channel.close for a channelId this plugin process
// does not own must fail with ErrHandleNotFound and must never reach the backend's CloseRemote.
func TestChannelProxy_CloseRejectsForeignChannelID(t *testing.T) {
	backend := &fakeChannelBackend{}
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}, MaxConcurrent: 4}
	proxy := newTestChannelProxy(t, caps, backend)

	// Open one legitimate channel so the proxy has real state, then attempt to close an id
	// that was never allocated to this plugin (the classic IDOR: a foreign/guessed handle).
	if _, err := proxy.Open(context.Background(), openParams("sess-1", "exec", "")); err != nil {
		t.Fatalf("open: %v", err)
	}

	foreignParams, _ := json.Marshal(map[string]uint32{"channelId": 9999})
	_, err := proxy.Close(context.Background(), foreignParams)
	if !errors.Is(err, domainplugin.ErrHandleNotFound) {
		t.Fatalf("close foreign channelId = %v, want ErrHandleNotFound", err)
	}
	if backend.closeCalls.Load() != 0 {
		t.Fatal("backend.CloseRemote must not be called for a channelId this plugin does not own")
	}
}
