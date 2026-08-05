package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	domainplugin "xquakshell/internal/domain/plugin"
)

// --- fakes -----------------------------------------------------------------

type fakeSurfacePresenter struct {
	mu      sync.Mutex
	opened  []domainplugin.Surface
	changed []domainplugin.Surface
	closed  []string
	outputs []string
}

func (p *fakeSurfacePresenter) Opened(s domainplugin.Surface) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.opened = append(p.opened, s)
}

func (p *fakeSurfacePresenter) Output(surfaceID, dataBase64, stream string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.outputs = append(p.outputs, surfaceID+":"+stream+":"+dataBase64)
}

// outputSnapshot copies what the pump has delivered so far. Output arrives on the surface's pump
// goroutine now, so every assertion about it reads through here rather than the slice directly.
func (p *fakeSurfacePresenter) outputSnapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.outputs...)
}

func (p *fakeSurfacePresenter) Changed(s domainplugin.Surface) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.changed = append(p.changed, s)
}

func (p *fakeSurfacePresenter) Closed(surfaceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = append(p.closed, surfaceID)
}

type fakeSurfaceOutbound struct {
	mu     sync.Mutex
	closed []string
}

func (o *fakeSurfaceOutbound) Input(pluginID, surfaceID string, data []byte)        {}
func (o *fakeSurfaceOutbound) Resize(pluginID, surfaceID string, cols, rows uint16) {}
func (o *fakeSurfaceOutbound) Closed(pluginID, surfaceID, reason string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.closed = append(o.closed, surfaceID)
}

type fakeSessionConnections map[string]string

func (f fakeSessionConnections) ConnectionForSession(sessionID string) (string, bool) {
	connectionID, ok := f[sessionID]
	return connectionID, ok
}

// --- harness ---------------------------------------------------------------

type surfaceHarness struct {
	svc       *SurfaceService
	presenter *fakeSurfacePresenter
	outbound  *fakeSurfaceOutbound
	audits    []domainplugin.SurfaceAuditEntry
}

func newSurfaceHarness(t *testing.T, caps *domainplugin.UICaps) *surfaceHarness {
	t.Helper()
	h := &surfaceHarness{
		presenter: &fakeSurfacePresenter{},
		outbound:  &fakeSurfaceOutbound{},
	}
	h.svc = NewSurfaceService(
		NewSurfaceRegistry(),
		h.presenter,
		h.outbound,
		fakeSessionConnections{"sess-1": "conn-1"},
		func(pluginID string) *domainplugin.UICaps { return caps },
		func(entry domainplugin.SurfaceAuditEntry) { h.audits = append(h.audits, entry) },
	)
	return h
}

func bothKinds() *domainplugin.UICaps {
	return &domainplugin.UICaps{Surfaces: []string{"terminal", "log"}}
}

// waitForOutputs blocks until the pump has delivered want batches, or fails the test.
//
// Output is batched on a ticker now, so an assertion that read the slice straight after a write
// would be racing the flush. Polling keeps the test honest about what it is waiting for instead of
// sleeping for a guessed interval.
func (h *surfaceHarness) waitForOutputs(t *testing.T, want int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := h.presenter.outputSnapshot()
		if len(got) >= want {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d output batches, got %v", want, got)
		}
		time.Sleep(surfaceOutputBatchInterval / 5)
	}
}

// expectNoOutput gives the pump a fair chance to deliver something and asserts that it did not.
func (h *surfaceHarness) expectNoOutput(t *testing.T, why string) {
	t.Helper()
	time.Sleep(surfaceOutputBatchInterval * 3)
	if got := h.presenter.outputSnapshot(); len(got) != 0 {
		t.Fatalf("%s: outputs = %v", why, got)
	}
}

func (h *surfaceHarness) open(t *testing.T, kind, title string) string {
	t.Helper()
	params := json.RawMessage(`{"parentSessionId":"sess-1","kind":"` + kind + `","title":"` + title + `"}`)
	raw, err := h.svc.Handle(context.Background(), "plugin-a", "surface.open", params)
	if err != nil {
		t.Fatalf("surface.open: %v", err)
	}
	var res struct {
		SurfaceID string `json:"surfaceId"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("decode surface.open result: %v", err)
	}
	if res.SurfaceID == "" {
		t.Fatal("surface.open returned an empty surfaceId")
	}
	return res.SurfaceID
}

func (h *surfaceHarness) call(pluginID, method string, params string) error {
	_, err := h.svc.Handle(context.Background(), pluginID, method, json.RawMessage(params))
	return err
}

// --- open ------------------------------------------------------------------

func TestSurfaceOpenRejectsUndeclaredKind(t *testing.T) {
	h := newSurfaceHarness(t, &domainplugin.UICaps{Surfaces: []string{"log"}})
	params := json.RawMessage(`{"parentSessionId":"sess-1","kind":"terminal","title":"t"}`)
	_, err := h.svc.Handle(context.Background(), "plugin-a", "surface.open", params)
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("got %v, want ErrCapabilityDenied", err)
	}
	if len(h.presenter.opened) != 0 {
		t.Fatal("a refused open must not reach the presenter")
	}
}

func TestSurfaceOpenRejectsUnknownKind(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	params := json.RawMessage(`{"parentSessionId":"sess-1","kind":"hologram","title":"t"}`)
	if _, err := h.svc.Handle(context.Background(), "plugin-a", "surface.open", params); err == nil {
		t.Fatal("expected an unknown kind to be refused")
	}
}

func TestSurfaceOpenRejectsSessionWithNoConnection(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	params := json.RawMessage(`{"parentSessionId":"sess-gone","kind":"log","title":"t"}`)
	if _, err := h.svc.Handle(context.Background(), "plugin-a", "surface.open", params); err == nil {
		t.Fatal("expected a session the host does not know to be refused")
	}
	if len(h.presenter.opened) != 0 {
		t.Fatal("a refused open must not reach the presenter")
	}
}

// A title is drawn next to tabs the user trusts, so control characters and bidirectional
// overrides are stripped before it is stored — the ADR-014 rule, applied to the same kind of
// string in the same window.
func TestSurfaceOpenSanitizesTitle(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	// U+202E RIGHT-TO-LEFT OVERRIDE would reverse everything after it in the tab bar; U+0007 is a
	// C0 control. Both go through json.Marshal so the test sends the bytes a plugin would send,
	// rather than a literal the JSON decoder rejects before the sanitizer is ever reached.
	params, marshalErr := json.Marshal(map[string]string{
		"parentSessionId": "sess-1",
		"kind":            "log",
		"title":           "log‮gnol",
	})
	if marshalErr != nil {
		t.Fatalf("marshal params: %v", marshalErr)
	}
	if _, err := h.svc.Handle(context.Background(), "plugin-a", "surface.open", params); err != nil {
		t.Fatalf("surface.open: %v", err)
	}
	got := h.presenter.opened[0].Title
	if strings.ContainsRune(got, '‮') || strings.ContainsRune(got, '') {
		t.Fatalf("title was not sanitized: %q", got)
	}
	if got != "loggnol" {
		t.Fatalf("title = %q, want %q", got, "loggnol")
	}
}

// An over-long title is truncated rather than refused: it is cosmetic, and losing the tab the
// user asked for over a long name would be a worse answer than a shortened name.
func TestSurfaceOpenTruncatesLongTitle(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	long := strings.Repeat("щ", domainplugin.MaxSurfaceTitleLen+50)
	params, _ := json.Marshal(map[string]string{"parentSessionId": "sess-1", "kind": "log", "title": long})
	if _, err := h.svc.Handle(context.Background(), "plugin-a", "surface.open", params); err != nil {
		t.Fatalf("surface.open: %v", err)
	}
	got := h.presenter.opened[0].Title
	if n := utf8.RuneCountInString(got); n != domainplugin.MaxSurfaceTitleLen {
		t.Fatalf("title length = %d runes, want %d", n, domainplugin.MaxSurfaceTitleLen)
	}
}

func TestSurfaceOpenResolvesConnectionAndAudits(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	opened := h.presenter.opened[0]
	if opened.ConnectionID != "conn-1" {
		t.Fatalf("ConnectionID = %q, want conn-1", opened.ConnectionID)
	}
	if opened.State != domainplugin.SurfaceStateConnecting {
		t.Fatalf("a new surface must start connecting, got %q", opened.State)
	}
	if len(h.audits) != 1 {
		t.Fatalf("audits = %d, want 1", len(h.audits))
	}
	if h.audits[0].SurfaceID != id || h.audits[0].ParentSessionID != "sess-1" || !h.audits[0].Success {
		t.Fatalf("audit entry = %+v", h.audits[0])
	}
}

func TestSurfaceOpenEnforcesPerPluginCap(t *testing.T) {
	caps := &domainplugin.UICaps{Surfaces: []string{"log"}, MaxSurfaces: 1}
	h := newSurfaceHarness(t, caps)
	h.open(t, "log", "one")
	params := json.RawMessage(`{"parentSessionId":"sess-1","kind":"log","title":"two"}`)
	_, err := h.svc.Handle(context.Background(), "plugin-a", "surface.open", params)
	if !errors.Is(err, domainplugin.ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited", err)
	}
}

// --- io --------------------------------------------------------------------

func TestSurfaceWriteReachesThePresenter(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	payload := base64.StdEncoding.EncodeToString([]byte("hello"))
	if err := h.call("plugin-a", "surface.write", `{"surfaceId":"`+id+`","dataBase64":"`+payload+`","stream":"stderr"}`); err != nil {
		t.Fatalf("surface.write: %v", err)
	}
	outputs := h.waitForOutputs(t, 1)
	if !strings.Contains(outputs[0], ":stderr:") {
		t.Fatalf("outputs = %v", outputs)
	}
}

func TestSurfaceWriteDefaultsToStdout(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	payload := base64.StdEncoding.EncodeToString([]byte("hi"))
	if err := h.call("plugin-a", "surface.write", `{"surfaceId":"`+id+`","dataBase64":"`+payload+`"}`); err != nil {
		t.Fatalf("surface.write: %v", err)
	}
	outputs := h.waitForOutputs(t, 1)
	if !strings.Contains(outputs[0], ":stdout:") {
		t.Fatalf("outputs = %v", outputs)
	}
}

func TestSurfaceWriteRejectsUnknownStream(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	payload := base64.StdEncoding.EncodeToString([]byte("hi"))
	if err := h.call("plugin-a", "surface.write", `{"surfaceId":"`+id+`","dataBase64":"`+payload+`","stream":"syslog"}`); err == nil {
		t.Fatal("expected an unknown stream name to be refused")
	}
}

func TestSurfaceWriteFromForeignPluginIsDenied(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	payload := base64.StdEncoding.EncodeToString([]byte("hi"))
	err := h.call("plugin-b", "surface.write", `{"surfaceId":"`+id+`","dataBase64":"`+payload+`"}`)
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("got %v, want ErrCapabilityDenied", err)
	}
	h.expectNoOutput(t, "a denied write must not reach the presenter")
}

// A surface the user already closed is not an error for the plugin: the tab is gone, the plugin
// has been told, and the write it had already queued is simply dropped. Erroring here would make
// an ordinary race look like a fault in the plugin.
func TestSurfaceWriteAfterCloseIsANoOp(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	if err := h.call("plugin-a", "surface.close", `{"surfaceId":"`+id+`"}`); err != nil {
		t.Fatalf("surface.close: %v", err)
	}
	payload := base64.StdEncoding.EncodeToString([]byte("late"))
	if err := h.call("plugin-a", "surface.write", `{"surfaceId":"`+id+`","dataBase64":"`+payload+`"}`); err != nil {
		t.Fatalf("a write to a closed surface must be a no-op, got %v", err)
	}
	h.expectNoOutput(t, "a write to a closed surface must not reach the presenter")
}

// Whether the consumer is keeping up is the queue's question, not the presenter's, so the
// backpressure verdict is covered where it is decided: surface_output_test.go.

// A payload that is not base64 is refused rather than forwarded. The frontend decoder falls back
// to treating an undecodable string as raw bytes, so passing it through put the literal base64
// text on screen and called it output.
func TestSurfaceWriteRejectsInvalidBase64(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	if err := h.call("plugin-a", "surface.write", `{"surfaceId":"`+id+`","dataBase64":"not base64!!"}`); err == nil {
		t.Fatal("expected an undecodable payload to be refused")
	}
	h.expectNoOutput(t, "an undecodable payload must not reach the presenter")
}

// Several writes inside one flush interval arrive as one event per stream. That is what keeps a
// chatty producer from becoming one repaint per chunk in the UI.
func TestSurfaceWritesAreBatchedPerStream(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	for _, part := range []string{"one\n", "two\n", "three\n"} {
		payload := base64.StdEncoding.EncodeToString([]byte(part))
		if err := h.call("plugin-a", "surface.write", `{"surfaceId":"`+id+`","dataBase64":"`+payload+`"}`); err != nil {
			t.Fatalf("surface.write: %v", err)
		}
	}
	outputs := h.waitForOutputs(t, 1)
	joined := strings.Join(outputs, "|")
	decoded := decodeAllBatches(t, outputs)
	if decoded != "one\ntwo\nthree\n" {
		t.Fatalf("batched output lost or reordered bytes: %q (raw %s)", decoded, joined)
	}
}

// stdout and stderr are never merged: the log viewer colours them apart, and a batch that spliced
// them would either lose that or interleave two half-lines.
func TestSurfaceBatchesKeepStreamsApart(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	out := base64.StdEncoding.EncodeToString([]byte("normal\n"))
	errPayload := base64.StdEncoding.EncodeToString([]byte("broken\n"))
	if err := h.call("plugin-a", "surface.write", `{"surfaceId":"`+id+`","dataBase64":"`+out+`","stream":"stdout"}`); err != nil {
		t.Fatalf("surface.write: %v", err)
	}
	if err := h.call("plugin-a", "surface.write", `{"surfaceId":"`+id+`","dataBase64":"`+errPayload+`","stream":"stderr"}`); err != nil {
		t.Fatalf("surface.write: %v", err)
	}

	outputs := h.waitForOutputs(t, 2)
	var sawStdout, sawStderr bool
	for _, entry := range outputs {
		if strings.Contains(entry, ":stdout:") {
			sawStdout = true
		}
		if strings.Contains(entry, ":stderr:") {
			sawStderr = true
		}
	}
	if !sawStdout || !sawStderr {
		t.Fatalf("each stream must arrive as its own batch: %v", outputs)
	}
}

// decodeAllBatches concatenates the payloads of every delivered batch.
func decodeAllBatches(t *testing.T, outputs []string) string {
	t.Helper()
	var b strings.Builder
	for _, entry := range outputs {
		parts := strings.SplitN(entry, ":", 3)
		if len(parts) != 3 {
			t.Fatalf("malformed recorded output %q", entry)
		}
		data, err := base64.StdEncoding.DecodeString(parts[2])
		if err != nil {
			t.Fatalf("presenter received something that is not base64: %v", err)
		}
		b.Write(data)
	}
	return b.String()
}

func TestSurfaceUpdateStateRejectsUnknownState(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	if err := h.call("plugin-a", "surface.updateState", `{"surfaceId":"`+id+`","state":"haunted"}`); err == nil {
		t.Fatal("expected an unknown state to be refused")
	}
}

func TestSurfaceUpdateStateAndTitleReachThePresenter(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	if err := h.call("plugin-a", "surface.updateState", `{"surfaceId":"`+id+`","state":"ready"}`); err != nil {
		t.Fatalf("surface.updateState: %v", err)
	}
	if err := h.call("plugin-a", "surface.setTitle", `{"surfaceId":"`+id+`","title":"renamed"}`); err != nil {
		t.Fatalf("surface.setTitle: %v", err)
	}
	if len(h.presenter.changed) != 2 {
		t.Fatalf("changed = %d, want 2", len(h.presenter.changed))
	}
	last := h.presenter.changed[1]
	if last.Title != "renamed" || last.State != domainplugin.SurfaceStateReady {
		t.Fatalf("last change = %+v", last)
	}
}

// --- lifetime --------------------------------------------------------------

func TestSurfaceCloseByPluginIsIdempotentAndDoesNotEchoBack(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	for i := 0; i < 2; i++ {
		if err := h.call("plugin-a", "surface.close", `{"surfaceId":"`+id+`"}`); err != nil {
			t.Fatalf("surface.close #%d: %v", i, err)
		}
	}
	if len(h.presenter.closed) != 1 {
		t.Fatalf("presenter closed %d times, want 1", len(h.presenter.closed))
	}
	if len(h.outbound.closed) != 0 {
		t.Fatal("a plugin that closed its own surface must not be notified about it")
	}
}

func TestClosingSessionClosesItsSurfacesAndNotifiesThePlugin(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	h.svc.CloseSurfacesForSession("sess-1")
	if len(h.presenter.closed) != 1 || h.presenter.closed[0] != id {
		t.Fatalf("presenter closed = %v", h.presenter.closed)
	}
	if len(h.outbound.closed) != 1 || h.outbound.closed[0] != id {
		t.Fatalf("plugin was not told: %v", h.outbound.closed)
	}
	h.svc.CloseSurfacesForSession("sess-1")
	if len(h.presenter.closed) != 1 {
		t.Fatal("closing a session twice must not re-announce anything")
	}
}

// A plugin whose process is gone cannot be told anything, and the tab it left behind shows a
// stream nobody is writing to.
func TestClosingPluginClosesItsSurfacesWithoutNotifyingIt(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	h.svc.CloseSurfacesForPlugin("plugin-a")
	if len(h.presenter.closed) != 1 || h.presenter.closed[0] != id {
		t.Fatalf("presenter closed = %v", h.presenter.closed)
	}
	if len(h.outbound.closed) != 0 {
		t.Fatal("a plugin that has exited must not be sent a notification")
	}
}

func TestSurfaceHandleRejectsUnknownMethod(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	if err := h.call("plugin-a", "surface.levitate", `{}`); err == nil {
		t.Fatal("expected an unknown surface method to be refused")
	}
}
