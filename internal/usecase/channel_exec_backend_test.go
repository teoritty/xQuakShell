package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

func sessionInfoFor(sessionID string) domain.ConnectionSession {
	return domain.ConnectionSession{SessionID: sessionID, State: domain.SessionReady}
}

// fakeExecSession is a test double for execSession: it records the exact command string handed
// to Start (the crux of the shell-injection-inert assertion) without needing a live SSH server.
type fakeExecSession struct {
	mu          sync.Mutex
	startCmd    string
	startCalled bool
	startErr    error
	stdin       *fakeWriteCloser
	stdout      io.Reader
	stderr      io.Reader
	waitErr     error
	waitOnce    sync.Once
	waitCh      chan struct{}
	closeCalls  int
}

func newFakeExecSession(stdout, stderr io.Reader) *fakeExecSession {
	return &fakeExecSession{
		stdin:  &fakeWriteCloser{},
		stdout: stdout,
		stderr: stderr,
		waitCh: make(chan struct{}),
	}
}

func (s *fakeExecSession) StdinPipe() (io.WriteCloser, error) { return s.stdin, nil }
func (s *fakeExecSession) StdoutPipe() (io.Reader, error)     { return s.stdout, nil }
func (s *fakeExecSession) StderrPipe() (io.Reader, error)     { return s.stderr, nil }

func (s *fakeExecSession) Start(cmd string) error {
	s.mu.Lock()
	s.startCmd = cmd
	s.startCalled = true
	s.mu.Unlock()
	return s.startErr
}

func (s *fakeExecSession) Wait() error {
	s.waitOnce.Do(func() { close(s.waitCh) })
	return s.waitErr
}

func (s *fakeExecSession) Close() error {
	s.mu.Lock()
	s.closeCalls++
	s.mu.Unlock()
	return nil
}

func (s *fakeExecSession) command() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startCmd
}

func (s *fakeExecSession) wasStarted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startCalled
}

type fakeWriteCloser struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	closed bool
}

func (w *fakeWriteCloser) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *fakeWriteCloser) Close() error {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	return nil
}

// recordingReader counts Read calls and serves data once, then EOF — enough to prove whether a
// backend's read loop actually calls Read, without ever unblocking on its own accord (the test
// controls that entirely via the fake channel data path's capacity).
type recordingReader struct {
	reads int32
	data  []byte
	mu    sync.Mutex
	sent  bool
}

func (r *recordingReader) Read(p []byte) (int, error) {
	atomic.AddInt32(&r.reads, 1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.sent {
		r.sent = true
		n := copy(p, r.data)
		return n, nil
	}
	return 0, io.EOF
}

func (r *recordingReader) readCount() int32 { return atomic.LoadInt32(&r.reads) }

// fakeChannelDataPath is a minimal domainplugin.ChannelDataPath test double, mirroring the one
// in internal/infra/plugin/capability/channel_relay_backend_test.go: Recv delivers bytes "sent
// by the plugin", Send records bytes emitted to the plugin, and capacity is test-controlled to
// drive the credit-0 backpressure assertion.
type fakeChannelDataPath struct {
	inbound  chan []byte
	closed   chan struct{}
	closeErr sync.Once

	mu       sync.Mutex
	sent     [][]byte
	capacity chan struct{}
}

func newFakeChannelDataPath() *fakeChannelDataPath {
	return &fakeChannelDataPath{
		inbound:  make(chan []byte, 16),
		closed:   make(chan struct{}),
		capacity: closedCapacityChan(),
	}
}

func closedCapacityChan() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

func (f *fakeChannelDataPath) blockCapacity() {
	f.mu.Lock()
	f.capacity = make(chan struct{})
	f.mu.Unlock()
}

func (f *fakeChannelDataPath) releaseCapacity() {
	f.mu.Lock()
	close(f.capacity)
	f.mu.Unlock()
}

func (f *fakeChannelDataPath) Send(_ context.Context, payload []byte) error {
	f.mu.Lock()
	f.sent = append(f.sent, append([]byte(nil), payload...))
	f.mu.Unlock()
	return nil
}

func (f *fakeChannelDataPath) sentFrames() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.sent))
	copy(out, f.sent)
	return out
}

func (f *fakeChannelDataPath) Recv() ([]byte, bool) {
	select {
	case b, ok := <-f.inbound:
		return b, ok
	case <-f.closed:
		return nil, false
	}
}

func (f *fakeChannelDataPath) WaitForCapacity(ctx context.Context) error {
	f.mu.Lock()
	c := f.capacity
	f.mu.Unlock()
	select {
	case <-c:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func containerExecTemplates() []domainplugin.ExecCommandTemplate {
	return []domainplugin.ExecCommandTemplate{
		{Argv: []string{"docker", "system", "dial-stdio"}},
		{
			Argv:   []string{"docker", "exec", "-it", "{containerId}", "sh"},
			Params: map[string]string{"containerId": "^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$"},
		},
	}
}

// permissiveExecTemplates uses an intentionally permissive params regex so tests can drive a
// malicious value all the way to shellQuoteArgv/Start and inspect exactly what would be sent to
// the remote SSH session, proving the safety property lives in construction, not validation.
func permissiveExecTemplates() []domainplugin.ExecCommandTemplate {
	return []domainplugin.ExecCommandTemplate{
		{
			Argv:   []string{"docker", "exec", "-it", "{containerId}", "sh"},
			Params: map[string]string{"containerId": ".*"},
		},
	}
}

func hintJSON(t *testing.T, template int, params map[string]string) string {
	t.Helper()
	b, err := json.Marshal(domainplugin.ExecChannelRequest{Template: template, Params: params})
	if err != nil {
		t.Fatalf("marshal hint: %v", err)
	}
	return string(b)
}

func newTestExecBackend(t *testing.T, templates []domainplugin.ExecCommandTemplate, consentGranted bool, opener func(string) (execSession, error)) (*ChannelExecBackend, *SessionRegistry) {
	t.Helper()
	registry := NewSessionRegistry()
	var notified []struct{ reason, message string }
	var mu sync.Mutex
	backend := NewChannelExecBackend("com.test", templates, consentGranted, registry, nil, func(reason, message string) {
		mu.Lock()
		notified = append(notified, struct{ reason, message string }{reason, message})
		mu.Unlock()
	})
	if opener != nil {
		backend.sessionOpener = opener
	}
	return backend, registry
}

func TestChannelExecBackend_Authorize_ValidContainerID(t *testing.T) {
	backend, _ := newTestExecBackend(t, containerExecTemplates(), true, nil)
	hint := hintJSON(t, 1, map[string]string{"containerId": "abc123"})
	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", hint); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if len(backend.argv) != 5 || backend.argv[3] != "abc123" {
		t.Fatalf("unexpected argv: %v", backend.argv)
	}
}

func TestChannelExecBackend_Authorize_RegexFails_NoSessionOpened(t *testing.T) {
	var openerCalled int32
	opener := func(string) (execSession, error) {
		atomic.AddInt32(&openerCalled, 1)
		return newFakeExecSession(strings.NewReader(""), strings.NewReader("")), nil
	}
	backend, registry := newTestExecBackend(t, containerExecTemplates(), true, opener)
	registry.Put("sess-1", newSessionEntry(sessionInfoFor("sess-1"), context.Background(), func() {}, "conn-1"))

	hint := hintJSON(t, 1, map[string]string{"containerId": "x; rm -rf /"})
	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", hint); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("authorize err = %v, want ErrCapabilityDenied", err)
	}

	// Defense in depth: even if a caller wired anyway despite the Authorize failure, Wire must
	// refuse (no stored argv) and never reach the session opener.
	handle := &domainplugin.ChannelHandle{ChannelID: 1, PluginID: "com.test", Purpose: domainplugin.PurposeExec, ParentSessionID: "sess-1"}
	if err := backend.Wire(context.Background(), handle); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("wire err = %v, want ErrCapabilityDenied", err)
	}
	if atomic.LoadInt32(&openerCalled) != 0 {
		t.Fatal("session opener must not be called when the requested exec was rejected")
	}
}

func TestChannelExecBackend_Authorize_NoMatchingTemplate(t *testing.T) {
	backend, _ := newTestExecBackend(t, containerExecTemplates(), true, nil)
	hint := hintJSON(t, 9, map[string]string{"containerId": "abc"})
	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", hint); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("authorize err = %v, want ErrCapabilityDenied", err)
	}
}

func TestChannelExecBackend_Authorize_NoConsent_Denied(t *testing.T) {
	backend, _ := newTestExecBackend(t, containerExecTemplates(), false, nil)
	hint := hintJSON(t, 1, map[string]string{"containerId": "abc123"})
	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", hint); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("authorize err = %v, want ErrCapabilityDenied (no install consent)", err)
	}
}

// TestChannelExecBackend_ShellInjectionInert is the crux security test: a shell-metacharacter
// payload in a param must reach Start() only as a single, fully single-quoted argv element —
// never as unquoted text that could be interpreted as a second command by the remote shell.
func TestChannelExecBackend_ShellInjectionInert(t *testing.T) {
	sess := newFakeExecSession(strings.NewReader(""), strings.NewReader(""))
	opener := func(string) (execSession, error) { return sess, nil }
	backend, registry := newTestExecBackend(t, permissiveExecTemplates(), true, opener)
	registry.Put("sess-1", newSessionEntry(sessionInfoFor("sess-1"), context.Background(), func() {}, "conn-1"))

	malicious := "x; rm -rf / #"
	hint := hintJSON(t, 0, map[string]string{"containerId": malicious})
	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", hint); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	handle := &domainplugin.ChannelHandle{ChannelID: 1, PluginID: "com.test", Purpose: domainplugin.PurposeExec, ParentSessionID: "sess-1"}
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	defer backend.CloseRemote()

	want := `'docker' 'exec' '-it' 'x; rm -rf / #' 'sh'`
	got := sess.command()
	if got != want {
		t.Fatalf("command sent to Start = %q, want %q", got, want)
	}
	// The malicious substring must only ever appear inside its own single-quoted token —
	// specifically, the exact quoted form, not bare/unquoted in the string.
	if !strings.Contains(got, "'"+malicious+"'") {
		t.Fatalf("malicious payload not safely quoted in %q", got)
	}
}

func TestChannelExecBackend_ShellInjectionInert_EmbeddedSingleQuote(t *testing.T) {
	sess := newFakeExecSession(strings.NewReader(""), strings.NewReader(""))
	opener := func(string) (execSession, error) { return sess, nil }
	backend, registry := newTestExecBackend(t, permissiveExecTemplates(), true, opener)
	registry.Put("sess-1", newSessionEntry(sessionInfoFor("sess-1"), context.Background(), func() {}, "conn-1"))

	malicious := "x'; rm -rf / #"
	hint := hintJSON(t, 0, map[string]string{"containerId": malicious})
	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", hint); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	handle := &domainplugin.ChannelHandle{ChannelID: 1, PluginID: "com.test", Purpose: domainplugin.PurposeExec, ParentSessionID: "sess-1"}
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	defer backend.CloseRemote()

	want := `'docker' 'exec' '-it' 'x'\''; rm -rf / #' 'sh'`
	if got := sess.command(); got != want {
		t.Fatalf("command sent to Start = %q, want %q", got, want)
	}
}

func TestChannelExecBackend_Wire_ExitSurfacesViaCloseNotifier(t *testing.T) {
	sess := newFakeExecSession(strings.NewReader(""), strings.NewReader("some warning on stderr"))
	opener := func(string) (execSession, error) { return sess, nil }
	registry := NewSessionRegistry()
	registry.Put("sess-1", newSessionEntry(sessionInfoFor("sess-1"), context.Background(), func() {}, "conn-1"))

	var mu sync.Mutex
	var got []struct{ reason, message string }
	backend := NewChannelExecBackend("com.test", containerExecTemplates(), true, registry, nil, func(reason, message string) {
		mu.Lock()
		got = append(got, struct{ reason, message string }{reason, message})
		mu.Unlock()
	})
	backend.sessionOpener = opener

	hint := hintJSON(t, 1, map[string]string{"containerId": "abc123"})
	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", hint); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	handle := &domainplugin.ChannelHandle{ChannelID: 1, PluginID: "com.test", Purpose: domainplugin.PurposeExec, ParentSessionID: "sess-1"}
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for close notification")
		case <-time.After(10 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if got[0].reason != "exit" {
		t.Fatalf("reason = %q, want exit", got[0].reason)
	}
	if got[0].message != "some warning on stderr" {
		t.Fatalf("message = %q, want stderr content", got[0].message)
	}
}

func TestChannelExecBackend_Wire_ErrorSurfacesViaCloseNotifier(t *testing.T) {
	sess := newFakeExecSession(strings.NewReader(""), strings.NewReader(""))
	sess.waitErr = errors.New("process killed")
	opener := func(string) (execSession, error) { return sess, nil }
	registry := NewSessionRegistry()
	registry.Put("sess-1", newSessionEntry(sessionInfoFor("sess-1"), context.Background(), func() {}, "conn-1"))

	var mu sync.Mutex
	var got []struct{ reason, message string }
	backend := NewChannelExecBackend("com.test", containerExecTemplates(), true, registry, nil, func(reason, message string) {
		mu.Lock()
		got = append(got, struct{ reason, message string }{reason, message})
		mu.Unlock()
	})
	backend.sessionOpener = opener

	hint := hintJSON(t, 1, map[string]string{"containerId": "abc123"})
	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", hint); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	handle := &domainplugin.ChannelHandle{ChannelID: 1, PluginID: "com.test", Purpose: domainplugin.PurposeExec, ParentSessionID: "sess-1"}
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for close notification")
		case <-time.After(10 * time.Millisecond):
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if got[0].reason != "error" {
		t.Fatalf("reason = %q, want error", got[0].reason)
	}
	if got[0].message != "process killed" {
		t.Fatalf("message = %q, want process killed", got[0].message)
	}
}

func TestChannelExecBackend_CreditZeroSuspendsStdoutReads(t *testing.T) {
	reader := &recordingReader{data: []byte("hello from remote")}
	sess := newFakeExecSession(reader, strings.NewReader(""))
	opener := func(string) (execSession, error) { return sess, nil }
	registry := NewSessionRegistry()
	registry.Put("sess-1", newSessionEntry(sessionInfoFor("sess-1"), context.Background(), func() {}, "conn-1"))

	backend := NewChannelExecBackend("com.test", containerExecTemplates(), true, registry, nil, nil)
	backend.sessionOpener = opener

	hint := hintJSON(t, 1, map[string]string{"containerId": "abc123"})
	if err := backend.Authorize(domainplugin.PurposeExec, "sess-1", hint); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	data := newFakeChannelDataPath()
	data.blockCapacity()
	handle := &domainplugin.ChannelHandle{ChannelID: 1, PluginID: "com.test", Purpose: domainplugin.PurposeExec, ParentSessionID: "sess-1", Data: data}
	if err := backend.Wire(context.Background(), handle); err != nil {
		t.Fatalf("wire: %v", err)
	}
	defer backend.CloseRemote()

	time.Sleep(50 * time.Millisecond)
	if n := reader.readCount(); n != 0 {
		t.Fatalf("stdout Read called %d times while credit is 0, want 0", n)
	}

	data.releaseCapacity()

	deadline := time.After(2 * time.Second)
	for {
		if reader.readCount() > 0 && len(data.sentFrames()) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for stdout read to resume after capacity release")
		case <-time.After(10 * time.Millisecond):
		}
	}
	frames := data.sentFrames()
	if string(frames[0]) != "hello from remote" {
		t.Fatalf("frame = %q, want %q", frames[0], "hello from remote")
	}
}

func TestShellQuoteArgv(t *testing.T) {
	got := shellQuoteArgv([]string{"docker", "exec", "-it", "abc123", "sh"})
	want := `'docker' 'exec' '-it' 'abc123' 'sh'`
	if got != want {
		t.Fatalf("shellQuoteArgv = %q, want %q", got, want)
	}
}
