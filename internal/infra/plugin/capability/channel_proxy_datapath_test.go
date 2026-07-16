package capability

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// ctxCapturingBackend records the context Wire was handed, so a test can outlive the call and
// ask whether the pumps Wire started would still be alive.
type ctxCapturingBackend struct {
	wired chan context.Context
}

func newCtxCapturingBackend() *ctxCapturingBackend {
	return &ctxCapturingBackend{wired: make(chan context.Context, 1)}
}

func (b *ctxCapturingBackend) Authorize(string, string, string) error { return nil }

func (b *ctxCapturingBackend) Wire(ctx context.Context, _ *domainplugin.ChannelHandle) error {
	b.wired <- ctx
	return nil
}

func (b *ctxCapturingBackend) CloseRemote() error { return nil }

// TestChannelProxy_WireContextOutlivesTheOpenRequest is the contract that makes the bus work at
// all in production.
//
// Every backend's Wire starts long-lived pump goroutines that hold onto the ctx it was given
// and gate every frame on it (WaitForCapacity, Send, Ack). The ctx arriving at Open belongs to
// the channel.open JSON-RPC request, and the ipc layer cancels it the moment that request is
// answered — so a Wire ctx borrowed from the request is dead before the first frame moves,
// while Open still reports success. Wire's ctx is the *channel's* lifetime, not the request's.
func TestChannelProxy_WireContextOutlivesTheOpenRequest(t *testing.T) {
	backend := newCtxCapturingBackend()
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}}
	resolve := func(string) (domainplugin.ChannelPurposeBackend, error) { return backend, nil }
	proxy := NewChannelProxy("p1", caps, resolve, nil)
	proxy.AttachDataPathOpener(newFakeDataPathOpener())

	rpcCtx, cancelRPC := context.WithCancel(context.Background())
	if _, err := proxy.Open(rpcCtx, openParams("sess-1", "exec", "")); err != nil {
		t.Fatalf("open: %v", err)
	}
	wireCtx := <-backend.wired

	// The host answers channel.open and drops the request context, exactly as ipc.Conn does.
	cancelRPC()

	select {
	case <-wireCtx.Done():
		t.Fatal("Wire's context died with the channel.open request; every pump goroutine would " +
			"exit before moving a byte, while the plugin holds a channelId that looks fine")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestChannelProxy_WireContextIsCancelledOnChannelClose is the other half: the pumps must not
// outlive the channel either, or closing a channel leaks every goroutine wired to it.
func TestChannelProxy_WireContextIsCancelledOnChannelClose(t *testing.T) {
	backend := newCtxCapturingBackend()
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}}
	resolve := func(string) (domainplugin.ChannelPurposeBackend, error) { return backend, nil }
	proxy := NewChannelProxy("p1", caps, resolve, nil)
	proxy.AttachDataPathOpener(newFakeDataPathOpener())

	res, err := proxy.Open(context.Background(), openParams("sess-1", "exec", ""))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	wireCtx := <-backend.wired

	var open channelOpenResult
	if err := json.Unmarshal(res, &open); err != nil {
		t.Fatal(err)
	}
	if _, err := proxy.Close(context.Background(), mustMarshal(channelCloseParams{ChannelID: open.ChannelID})); err != nil {
		t.Fatalf("close: %v", err)
	}

	select {
	case <-wireCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Wire's context survived channel.close; the pump goroutines it gates leak")
	}
}

// TestChannelProxy_OpenFailsWhenDataPathCannotOpen: returning a channelId the plugin cannot use
// is the worst available contract — the plugin gets a success, sends its first frame, and dies.
// The failure has to surface in the call that caused it.
func TestChannelProxy_OpenFailsWhenDataPathCannotOpen(t *testing.T) {
	backend := &fakeChannelBackend{}
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}}
	proxy, opener := newTestChannelProxyWithOpener(t, caps, backend)
	opener.err = errors.New("no credit policy for purpose")

	_, err := proxy.Open(context.Background(), openParams("sess-1", "exec", ""))
	if err == nil {
		t.Fatal("Open returned success though the channel has no data path; the plugin would be killed on its first frame")
	}
	if backend.wireCalls.Load() != 0 {
		t.Fatal("backend.Wire must not be reached when the data path could not be opened")
	}
}

// TestChannelProxy_OpenDeniedWithoutAttachedOpener: a proxy whose composition root never wired
// it must refuse, not hand out channels that cannot carry bytes.
func TestChannelProxy_OpenDeniedWithoutAttachedOpener(t *testing.T) {
	backend := &fakeChannelBackend{}
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}}
	resolve := func(string) (domainplugin.ChannelPurposeBackend, error) { return backend, nil }
	proxy := NewChannelProxy("p1", caps, resolve, nil) // deliberately not attached

	if _, err := proxy.Open(context.Background(), openParams("sess-1", "exec", "")); err == nil {
		t.Fatal("Open succeeded with no data path opener attached")
	}
}

// TestChannelProxy_WireFailureReleasesDataPath is the rollback: Wire runs after the data path is
// registered on the bus, so a Wire error that skipped the release would strand that channel on
// the bus for the life of the plugin process.
func TestChannelProxy_WireFailureReleasesDataPath(t *testing.T) {
	backend := &fakeChannelBackend{wireErr: errors.New("dial refused")}
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}}
	proxy, opener := newTestChannelProxyWithOpener(t, caps, backend)

	if _, err := proxy.Open(context.Background(), openParams("sess-1", "exec", "")); err == nil {
		t.Fatal("expected Open to fail when the backend fails to wire")
	}

	if opener.openCount() != 1 {
		t.Fatalf("data paths opened = %d, want 1", opener.openCount())
	}
	if !opener.allClosed() {
		t.Fatal("the data path opened for a failed Wire was left on the bus; it can never be closed by anyone now")
	}
}

// TestChannelProxy_CloseReleasesDataPath: closing only the backend leaves its pumps parked in
// Recv forever.
func TestChannelProxy_CloseReleasesDataPath(t *testing.T) {
	backend := &fakeChannelBackend{}
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}}
	proxy, opener := newTestChannelProxyWithOpener(t, caps, backend)

	res, err := proxy.Open(context.Background(), openParams("sess-1", "exec", ""))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var open channelOpenResult
	if err := json.Unmarshal(res, &open); err != nil {
		t.Fatal(err)
	}
	if opener.allClosed() {
		t.Fatal("data path closed while the channel is still open")
	}

	if _, err := proxy.Close(context.Background(), mustMarshal(channelCloseParams{ChannelID: open.ChannelID})); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !opener.allClosed() {
		t.Fatal("channel.close left the data path open; the backend's pump goroutines never exit")
	}
}

// TestChannelProxy_CloseAllReleasesDataPaths covers the plugin-crash teardown path.
func TestChannelProxy_CloseAllReleasesDataPaths(t *testing.T) {
	backend := &fakeChannelBackend{}
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}, MaxConcurrent: 4}
	proxy, opener := newTestChannelProxyWithOpener(t, caps, backend)

	for i := 0; i < 3; i++ {
		if _, err := proxy.Open(context.Background(), openParams("sess-1", "exec", "")); err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
	}

	proxy.CloseAll()

	if opener.openCount() != 3 {
		t.Fatalf("data paths opened = %d, want 3", opener.openCount())
	}
	if !opener.allClosed() {
		t.Fatal("CloseAll left data paths open")
	}
}

// TestChannelProxy_CloseSessionReleasesOnlyThatSessionsDataPaths: the session cascade must not
// reach a sibling session's channels on the same plugin process.
func TestChannelProxy_CloseSessionReleasesOnlyThatSessionsDataPaths(t *testing.T) {
	backend := &fakeChannelBackend{}
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}, MaxConcurrent: 4}
	proxy, opener := newTestChannelProxyWithOpener(t, caps, backend)

	if _, err := proxy.Open(context.Background(), openParams("sess-doomed", "exec", "")); err != nil {
		t.Fatalf("open doomed: %v", err)
	}
	res, err := proxy.Open(context.Background(), openParams("sess-survivor", "exec", ""))
	if err != nil {
		t.Fatalf("open survivor: %v", err)
	}
	var survivor channelOpenResult
	if err := json.Unmarshal(res, &survivor); err != nil {
		t.Fatal(err)
	}

	proxy.CloseSession("sess-doomed")

	opener.mu.Lock()
	survivorPath := opener.paths[survivor.ChannelID]
	opener.mu.Unlock()

	select {
	case <-survivorPath.closed:
		t.Fatal("closing one session released another session's data path")
	default:
	}
	if opener.allClosed() {
		t.Fatal("expected only the doomed session's path to be closed")
	}
}
