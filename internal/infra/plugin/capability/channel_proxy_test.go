package capability

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

type fakeChannelBackend struct {
	authorizeErr error
	wireErr      error
	wireCalls    atomic.Int32
	closeCalls   atomic.Int32
}

func (f *fakeChannelBackend) Authorize(purpose, parentSessionID, hint string) error {
	return f.authorizeErr
}

func (f *fakeChannelBackend) Wire(ctx context.Context, ch *domainplugin.ChannelHandle) error {
	f.wireCalls.Add(1)
	return f.wireErr
}

func (f *fakeChannelBackend) CloseRemote() error {
	f.closeCalls.Add(1)
	return nil
}

// fakeDataPathOpener stands in for the ipc Conn: it hands out data paths and records what it
// opened and closed, which is how the rollback-on-failure assertions observe leaks.
type fakeDataPathOpener struct {
	err error

	mu     sync.Mutex
	opened []uint32
	paths  map[uint32]*fakeChannelDataPath
}

func newFakeDataPathOpener() *fakeDataPathOpener {
	return &fakeDataPathOpener{paths: make(map[uint32]*fakeChannelDataPath)}
}

func (o *fakeDataPathOpener) OpenDataPath(id uint32, _ string) (domainplugin.ChannelDataPath, error) {
	if o.err != nil {
		return nil, o.err
	}
	p := newFakeChannelDataPath()
	o.mu.Lock()
	o.opened = append(o.opened, id)
	o.paths[id] = p
	o.mu.Unlock()
	return p, nil
}

// openCount reports how many data paths were handed out.
func (o *fakeDataPathOpener) openCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.opened)
}

// allClosed reports whether every path handed out has since been closed — the property that
// separates a clean teardown from a leaked channel.
func (o *fakeDataPathOpener) allClosed() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, p := range o.paths {
		select {
		case <-p.closed:
		default:
			return false
		}
	}
	return true
}

func newTestChannelProxy(t *testing.T, caps *domainplugin.ChannelCaps, backend *fakeChannelBackend) *ChannelProxy {
	t.Helper()
	p, _ := newTestChannelProxyWithOpener(t, caps, backend)
	return p
}

func newTestChannelProxyWithOpener(t *testing.T, caps *domainplugin.ChannelCaps, backend *fakeChannelBackend) (*ChannelProxy, *fakeDataPathOpener) {
	t.Helper()
	resolve := func(purpose string) (domainplugin.ChannelPurposeBackend, error) {
		return backend, nil
	}
	proxy := NewChannelProxy("p1", caps, resolve, nil)
	opener := newFakeDataPathOpener()
	proxy.AttachDataPathOpener(opener)
	return proxy, opener
}

func openParams(parentSessionID, purpose, hint string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{
		"parentSessionId": parentSessionID,
		"purpose":         purpose,
		"hint":            hint,
	})
	return b
}

func TestChannelProxy_OpenRejectsUndeclaredPurpose(t *testing.T) {
	backend := &fakeChannelBackend{}
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}}
	proxy := newTestChannelProxy(t, caps, backend)

	_, err := proxy.Open(context.Background(), openParams("sess-1", "tcp-relay", "example.com:80"))
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("expected ErrCapabilityDenied, got %v", err)
	}
	if backend.wireCalls.Load() != 0 {
		t.Fatal("backend.Wire must not be called for an undeclared purpose")
	}
}

func TestChannelProxy_MaxConcurrentEnforcedBeforeResourceCreation(t *testing.T) {
	backend := &fakeChannelBackend{}
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}, MaxConcurrent: 2}
	proxy := newTestChannelProxy(t, caps, backend)

	for i := 0; i < 2; i++ {
		if _, err := proxy.Open(context.Background(), openParams("sess-1", "exec", "")); err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
	}
	if backend.wireCalls.Load() != 2 {
		t.Fatalf("expected 2 Wire calls, got %d", backend.wireCalls.Load())
	}

	_, err := proxy.Open(context.Background(), openParams("sess-1", "exec", ""))
	if !errors.Is(err, domainplugin.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited on N+1 open, got %v", err)
	}
	if backend.wireCalls.Load() != 2 {
		t.Fatalf("backend.Wire must not be called for the rejected N+1 open, got %d calls", backend.wireCalls.Load())
	}
	proxy.mu.Lock()
	nextID := proxy.nextID
	proxy.mu.Unlock()
	if nextID != 2 {
		t.Fatalf("channelId counter must not advance for the rejected open, got %d", nextID)
	}
}

func TestChannelProxy_ChannelIDsMonotonicNeverReused(t *testing.T) {
	backend := &fakeChannelBackend{}
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}, MaxConcurrent: 4}
	proxy := newTestChannelProxy(t, caps, backend)

	resA, err := proxy.Open(context.Background(), openParams("sess-1", "exec", ""))
	if err != nil {
		t.Fatal(err)
	}
	var openA channelOpenResult
	if err := json.Unmarshal(resA, &openA); err != nil {
		t.Fatal(err)
	}

	if _, err := proxy.Close(context.Background(), mustMarshal(channelCloseParams{ChannelID: openA.ChannelID})); err != nil {
		t.Fatalf("close A: %v", err)
	}

	resB, err := proxy.Open(context.Background(), openParams("sess-1", "exec", ""))
	if err != nil {
		t.Fatal(err)
	}
	var openB channelOpenResult
	if err := json.Unmarshal(resB, &openB); err != nil {
		t.Fatal(err)
	}

	if openB.ChannelID == openA.ChannelID {
		t.Fatalf("channelId reused after close: A=%d B=%d", openA.ChannelID, openB.ChannelID)
	}
	if openB.ChannelID <= openA.ChannelID {
		t.Fatalf("channelId not monotonic: A=%d B=%d", openA.ChannelID, openB.ChannelID)
	}
}

func TestChannelProxy_AuditsOpenAndClose(t *testing.T) {
	backend := &fakeChannelBackend{}
	caps := &domainplugin.ChannelCaps{Purposes: []string{"exec"}}
	var entries []domainplugin.ChannelAuditEntry
	audit := func(e domainplugin.ChannelAuditEntry) { entries = append(entries, e) }
	resolve := func(purpose string) (domainplugin.ChannelPurposeBackend, error) { return backend, nil }
	proxy := NewChannelProxy("p1", caps, resolve, audit)
	proxy.AttachDataPathOpener(newFakeDataPathOpener())

	res, err := proxy.Open(context.Background(), openParams("sess-1", "exec", "docker exec target"))
	if err != nil {
		t.Fatal(err)
	}
	var openRes channelOpenResult
	if err := json.Unmarshal(res, &openRes); err != nil {
		t.Fatal(err)
	}
	if _, err := proxy.Close(context.Background(), mustMarshal(channelCloseParams{ChannelID: openRes.ChannelID})); err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 audit entries (open+close), got %d", len(entries))
	}
	open := entries[0]
	if open.Action != "channel.open" || open.Purpose != "exec" || open.Target != "docker exec target" || !open.Success {
		t.Fatalf("unexpected open audit entry: %+v", open)
	}
	closeEntry := entries[1]
	if closeEntry.Action != "channel.close" || closeEntry.ChannelID != openRes.ChannelID || !closeEntry.Success {
		t.Fatalf("unexpected close audit entry: %+v", closeEntry)
	}
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
