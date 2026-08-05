package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	domainplugin "xquakshell/internal/domain/plugin"
)

// --- fakes -----------------------------------------------------------------

type fakeSurfacePresenter struct {
	mu        sync.Mutex
	opened    []domainplugin.Surface
	changed   []domainplugin.Surface
	closed    []string
	outputs   []string
	outputErr error
}

func (p *fakeSurfacePresenter) Opened(s domainplugin.Surface) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.opened = append(p.opened, s)
}

func (p *fakeSurfacePresenter) Output(surfaceID, dataBase64, stream string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.outputErr != nil {
		return p.outputErr
	}
	p.outputs = append(p.outputs, surfaceID+":"+stream+":"+dataBase64)
	return nil
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
	if len(h.presenter.outputs) != 1 || !strings.Contains(h.presenter.outputs[0], ":stderr:") {
		t.Fatalf("outputs = %v", h.presenter.outputs)
	}
}

func TestSurfaceWriteDefaultsToStdout(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	payload := base64.StdEncoding.EncodeToString([]byte("hi"))
	if err := h.call("plugin-a", "surface.write", `{"surfaceId":"`+id+`","dataBase64":"`+payload+`"}`); err != nil {
		t.Fatalf("surface.write: %v", err)
	}
	if !strings.Contains(h.presenter.outputs[0], ":stdout:") {
		t.Fatalf("outputs = %v", h.presenter.outputs)
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
	if len(h.presenter.outputs) != 0 {
		t.Fatal("a denied write must not reach the presenter")
	}
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
	if len(h.presenter.outputs) != 0 {
		t.Fatal("a write to a closed surface must not reach the presenter")
	}
}

func TestSurfaceWriteReportsBackpressureAsRateLimited(t *testing.T) {
	h := newSurfaceHarness(t, bothKinds())
	id := h.open(t, "log", "logs")
	h.presenter.outputErr = errors.New("consumer is behind")
	payload := base64.StdEncoding.EncodeToString([]byte("x"))
	err := h.call("plugin-a", "surface.write", `{"surfaceId":"`+id+`","dataBase64":"`+payload+`"}`)
	if !errors.Is(err, domainplugin.ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited", err)
	}
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
