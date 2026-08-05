package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

// --- fakes -----------------------------------------------------------------

type fakeDialogPresenter struct {
	mu      sync.Mutex
	opened  []domainplugin.Dialog
	closed  []string
	errored []string
}

func (p *fakeDialogPresenter) DialogOpened(d domainplugin.Dialog) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.opened = append(p.opened, d)
}

func (p *fakeDialogPresenter) DialogClosed(dialogID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = append(p.closed, dialogID)
}

func (p *fakeDialogPresenter) DialogError(dialogID, message string, fieldErrors map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errored = append(p.errored, dialogID+":"+message)
}

type fakeDialogOutbound struct {
	mu        sync.Mutex
	submitted []string
	cancelled []string
	values    map[string]map[string]string
}

func newFakeDialogOutbound() *fakeDialogOutbound {
	return &fakeDialogOutbound{values: make(map[string]map[string]string)}
}

func (o *fakeDialogOutbound) Submitted(pluginID, dialogID string, values map[string]string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.submitted = append(o.submitted, dialogID)
	o.values[dialogID] = values
}

func (o *fakeDialogOutbound) Cancelled(pluginID, dialogID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cancelled = append(o.cancelled, dialogID)
}

// --- harness ---------------------------------------------------------------

type dialogHarness struct {
	svc       *DialogService
	presenter *fakeDialogPresenter
	outbound  *fakeDialogOutbound
}

func newDialogHarness(t *testing.T, dialogsGranted bool) *dialogHarness {
	t.Helper()
	h := &dialogHarness{presenter: &fakeDialogPresenter{}, outbound: newFakeDialogOutbound()}
	h.svc = NewDialogService(
		NewDialogRegistry(),
		h.presenter,
		h.outbound,
		func(pluginID string) *domainplugin.UICaps {
			return &domainplugin.UICaps{Dialogs: dialogsGranted}
		},
	)
	return h
}

const oneTextField = `[{"id":"main","label":"Main","fields":[
	{"id":"name","label":"Name","type":"text","secret":false}
]}]`

func (h *dialogHarness) open(t *testing.T, kind string) string {
	t.Helper()
	return h.openWithSections(t, kind, oneTextField)
}

func (h *dialogHarness) openWithSections(t *testing.T, kind, sections string) string {
	t.Helper()
	params := json.RawMessage(`{"kind":"` + kind + `","title":"T","sections":` + sections + `}`)
	raw, err := h.svc.Handle(context.Background(), "plugin-a", "dialog.open", params)
	if err != nil {
		t.Fatalf("dialog.open: %v", err)
	}
	var res struct {
		DialogID string `json:"dialogId"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.DialogID == "" {
		t.Fatal("dialog.open returned an empty dialogId")
	}
	return res.DialogID
}

// --- open ------------------------------------------------------------------

func TestDialogOpenRequiresTheGrant(t *testing.T) {
	h := newDialogHarness(t, false)
	params := json.RawMessage(`{"kind":"form","title":"T","sections":` + oneTextField + `}`)
	_, err := h.svc.Handle(context.Background(), "plugin-a", "dialog.open", params)
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("got %v, want ErrCapabilityDenied", err)
	}
}

func TestDialogOpenRejectsUnknownKind(t *testing.T) {
	h := newDialogHarness(t, true)
	params := json.RawMessage(`{"kind":"wizard","title":"T","sections":` + oneTextField + `}`)
	if _, err := h.svc.Handle(context.Background(), "plugin-a", "dialog.open", params); err == nil {
		t.Fatal("expected an unknown dialog kind to be refused")
	}
}

// A second modal from the same plugin would stack over the first, and the user would answer a
// question without seeing which one they were answering.
func TestSecondDialogOpenIsRefused(t *testing.T) {
	h := newDialogHarness(t, true)
	first := h.open(t, "form")
	params := json.RawMessage(`{"kind":"form","title":"Second","sections":` + oneTextField + `}`)
	_, err := h.svc.Handle(context.Background(), "plugin-a", "dialog.open", params)
	if !errors.Is(err, domainplugin.ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited", err)
	}
	if len(h.presenter.opened) != 1 || h.presenter.opened[0].ID != first {
		t.Fatal("the refused open must not have disturbed the first dialog")
	}
}

func TestAnotherPluginMayStillOpenADialog(t *testing.T) {
	h := newDialogHarness(t, true)
	h.open(t, "form")
	params := json.RawMessage(`{"kind":"form","title":"Other","sections":` + oneTextField + `}`)
	if _, err := h.svc.Handle(context.Background(), "plugin-b", "dialog.open", params); err != nil {
		t.Fatalf("the limit is per plugin, not global: %v", err)
	}
}

func TestDialogOpenRejectsSecretField(t *testing.T) {
	h := newDialogHarness(t, true)
	sections := `[{"id":"g","label":"G","fields":[{"id":"p","label":"P","type":"password","secret":true}]}]`
	params := json.RawMessage(`{"kind":"form","title":"T","sections":` + sections + `}`)
	if _, err := h.svc.Handle(context.Background(), "plugin-a", "dialog.open", params); err == nil {
		t.Fatal("expected a secret field to be refused: a dialog has no vault to put one in")
	}
}

func TestDialogOpenRejectsTooManyFields(t *testing.T) {
	h := newDialogHarness(t, true)
	var b strings.Builder
	b.WriteString(`[{"id":"g","label":"G","fields":[`)
	for i := 0; i <= domainplugin.MaxDialogFields; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"id":"f`)
		b.WriteString(itoaTest(i))
		b.WriteString(`","label":"F","type":"text","secret":false}`)
	}
	b.WriteString(`]}]`)
	params := json.RawMessage(`{"kind":"form","title":"T","sections":` + b.String() + `}`)
	if _, err := h.svc.Handle(context.Background(), "plugin-a", "dialog.open", params); err == nil {
		t.Fatalf("expected more than %d fields to be refused", domainplugin.MaxDialogFields)
	}
}

func TestDialogOpenSanitizesTitle(t *testing.T) {
	h := newDialogHarness(t, true)
	params, err := json.Marshal(map[string]any{
		"kind":     "form",
		"title":    "Create‮volume",
		"sections": json.RawMessage(oneTextField),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := h.svc.Handle(context.Background(), "plugin-a", "dialog.open", params); err != nil {
		t.Fatalf("dialog.open: %v", err)
	}
	title := h.presenter.opened[0].Title
	if strings.ContainsRune(title, '‮') || strings.ContainsRune(title, '\a') {
		t.Fatalf("title was not sanitized: %q", title)
	}
}

// --- answers ---------------------------------------------------------------

func TestDialogDeliversExactlyOneAnswer(t *testing.T) {
	h := newDialogHarness(t, true)
	id := h.open(t, "form")
	if err := h.svc.Submit(id, map[string]string{"name": "vol"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := h.svc.Cancel(id); err == nil {
		t.Fatal("a dialog that was already answered must refuse a second answer")
	}
	if len(h.outbound.submitted) != 1 || len(h.outbound.cancelled) != 0 {
		t.Fatalf("submitted=%v cancelled=%v", h.outbound.submitted, h.outbound.cancelled)
	}
}

func TestDialogSubmitValidatesAgainstDeclaredFields(t *testing.T) {
	h := newDialogHarness(t, true)
	id := h.openWithSections(t, "form", `[{"id":"g","label":"G","fields":[
		{"id":"name","label":"Name","type":"text","secret":false,"validation":{"maxLength":4}}
	]}]`)

	if err := h.svc.Submit(id, map[string]string{"name": "far too long"}); err == nil {
		t.Fatal("expected a value violating its declared validation to be refused")
	}
	if len(h.outbound.submitted) != 0 {
		t.Fatal("a refused submit must not reach the plugin")
	}
}

// A pattern arrives as a string and has to be compiled before a submit can be checked against it.
// Without that step every value of such a field was refused with "field pattern not compiled" —
// the form validated in the browser, the host rejected it, and the user had nothing to fix.
func TestDialogSubmitAcceptsAValueMatchingItsDeclaredPattern(t *testing.T) {
	h := newDialogHarness(t, true)
	id := h.openWithSections(t, "form", `[{"id":"g","label":"G","fields":[
		{"id":"name","label":"Name","type":"text","secret":false,"validation":{"pattern":"^[a-z]+$"}}
	]}]`)

	if err := h.svc.Submit(id, map[string]string{"name": "webserver"}); err != nil {
		t.Fatalf("a value matching the declared pattern was refused: %v", err)
	}
	if h.outbound.values[id]["name"] != "webserver" {
		t.Fatalf("the accepted value did not reach the plugin: %v", h.outbound.values[id])
	}
}

func TestDialogSubmitRefusesAValueBreakingItsDeclaredPattern(t *testing.T) {
	h := newDialogHarness(t, true)
	id := h.openWithSections(t, "form", `[{"id":"g","label":"G","fields":[
		{"id":"name","label":"Name","type":"text","secret":false,"validation":{"pattern":"^[a-z]+$"}}
	]}]`)

	if err := h.svc.Submit(id, map[string]string{"name": "NOT lowercase"}); err == nil {
		t.Fatal("expected a value breaking the declared pattern to be refused")
	}
	if len(h.outbound.submitted) != 0 {
		t.Fatal("a refused submit must not reach the plugin")
	}
}

// A dialog whose pattern the host cannot vouch for is refused at open, not at submit: the modal
// must not appear at all if answering it could never succeed.
func TestDialogOpenRejectsAnUnsafePattern(t *testing.T) {
	h := newDialogHarness(t, true)
	sections := `[{"id":"g","label":"G","fields":[
		{"id":"name","label":"Name","type":"text","secret":false,"validation":{"pattern":"((a+)+)+"}}
	]}]`
	params := json.RawMessage(`{"kind":"form","title":"T","sections":` + sections + `}`)
	if _, err := h.svc.Handle(context.Background(), "plugin-a", "dialog.open", params); err == nil {
		t.Fatal("expected an unsafe pattern to be refused at open")
	}
	if len(h.presenter.opened) != 0 {
		t.Fatal("a refused open must not put a modal on screen")
	}
}

// An undeclared key is dropped rather than refused: the frontend sends what it rendered, and a
// stale extra key is a UI race, not an attack — but the plugin must never receive a value for a
// field it did not declare.
func TestDialogSubmitDropsUndeclaredKeys(t *testing.T) {
	h := newDialogHarness(t, true)
	id := h.open(t, "form")
	if err := h.svc.Submit(id, map[string]string{"name": "ok", "sneaky": "x"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	values := h.outbound.values[id]
	if _, present := values["sneaky"]; present {
		t.Fatalf("undeclared key reached the plugin: %v", values)
	}
	if values["name"] != "ok" {
		t.Fatalf("declared value lost: %v", values)
	}
}

const dependentRequiredFields = `[{"id":"g","label":"G","fields":[
	{"id":"mode","label":"Mode","type":"text","secret":false},
	{"id":"path","label":"Path","type":"text","secret":false,"required":true,"dependsOn":"mode"}
]}]`

// A required field whose dependency is off is not on screen, so demanding it would refuse a form
// the user cannot complete. The renderer and SavePluginFields both already skip such a field; this
// is the third place that has to agree.
func TestDialogSubmitIgnoresARequiredFieldItsDependencyHides(t *testing.T) {
	h := newDialogHarness(t, true)
	id := h.openWithSections(t, "form", dependentRequiredFields)

	if err := h.svc.Submit(id, map[string]string{"mode": ""}); err != nil {
		t.Fatalf("a hidden required field blocked the submit: %v", err)
	}
	if _, present := h.outbound.values[id]["path"]; present {
		t.Fatalf("a hidden field must not reach the plugin: %v", h.outbound.values[id])
	}
}

func TestDialogSubmitEnforcesARequiredFieldItsDependencyShows(t *testing.T) {
	h := newDialogHarness(t, true)
	id := h.openWithSections(t, "form", dependentRequiredFields)

	if err := h.svc.Submit(id, map[string]string{"mode": "local", "path": ""}); err == nil {
		t.Fatal("expected a visible required field left empty to be refused")
	}
	if len(h.outbound.submitted) != 0 {
		t.Fatal("a refused submit must not reach the plugin")
	}

	if err := h.svc.Submit(id, map[string]string{"mode": "local", "path": "/srv"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if h.outbound.values[id]["path"] != "/srv" {
		t.Fatalf("the visible dependent value did not reach the plugin: %v", h.outbound.values[id])
	}
}

// A stale value for a field the user then hid is dropped rather than validated: the answer is what
// the form shows now, not what it showed three keystrokes ago.
func TestDialogSubmitDropsTheValueOfAHiddenField(t *testing.T) {
	h := newDialogHarness(t, true)
	id := h.openWithSections(t, "form", `[{"id":"g","label":"G","fields":[
		{"id":"mode","label":"Mode","type":"text","secret":false},
		{"id":"port","label":"Port","type":"number","secret":false,"dependsOn":"mode","validation":{"min":1,"max":10}}
	]}]`)

	if err := h.svc.Submit(id, map[string]string{"mode": "", "port": "99999"}); err != nil {
		t.Fatalf("a hidden field's stale value was validated: %v", err)
	}
	if _, present := h.outbound.values[id]["port"]; present {
		t.Fatalf("a hidden field's value reached the plugin: %v", h.outbound.values[id])
	}
}

// A detail dialog has only a close button. Submitting one would hand a plugin an answer to a
// question it never asked.
func TestDetailDialogNeverSubmits(t *testing.T) {
	h := newDialogHarness(t, true)
	id := h.open(t, "detail")
	if err := h.svc.Submit(id, map[string]string{"name": "x"}); err == nil {
		t.Fatal("expected submit on a detail dialog to be refused")
	}
	if err := h.svc.Cancel(id); err != nil {
		t.Fatalf("a detail dialog must still be closable: %v", err)
	}
	if len(h.outbound.cancelled) != 1 {
		t.Fatalf("cancelled = %v", h.outbound.cancelled)
	}
}

func TestDialogCancelledWhenPluginStops(t *testing.T) {
	h := newDialogHarness(t, true)
	id := h.open(t, "form")
	h.svc.CancelForPlugin("plugin-a")
	if len(h.presenter.closed) != 1 || h.presenter.closed[0] != id {
		t.Fatalf("presenter closed = %v", h.presenter.closed)
	}
	// The plugin's process is gone; telling it its dialog was cancelled has nowhere to go.
	if len(h.outbound.cancelled) != 0 {
		t.Fatal("a plugin that has stopped must not be sent a cancellation")
	}
	h.svc.CancelForPlugin("plugin-a")
	if len(h.presenter.closed) != 1 {
		t.Fatal("cancelling twice must not re-announce anything")
	}
}

func TestDialogSetErrorRequiresOwnership(t *testing.T) {
	h := newDialogHarness(t, true)
	id := h.open(t, "form")
	params := json.RawMessage(`{"dialogId":"` + id + `","message":"docker refused"}`)
	if _, err := h.svc.Handle(context.Background(), "plugin-b", "dialog.setError", params); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("got %v, want ErrCapabilityDenied", err)
	}
	if _, err := h.svc.Handle(context.Background(), "plugin-a", "dialog.setError", params); err != nil {
		t.Fatalf("the owner must be able to report an error: %v", err)
	}
	if len(h.presenter.errored) != 1 {
		t.Fatalf("errored = %v", h.presenter.errored)
	}
}

func TestDialogCloseByPluginIsIdempotent(t *testing.T) {
	h := newDialogHarness(t, true)
	id := h.open(t, "form")
	params := json.RawMessage(`{"dialogId":"` + id + `"}`)
	for i := 0; i < 2; i++ {
		if _, err := h.svc.Handle(context.Background(), "plugin-a", "dialog.close", params); err != nil {
			t.Fatalf("dialog.close #%d: %v", i, err)
		}
	}
	if len(h.presenter.closed) != 1 {
		t.Fatalf("presenter closed %d times, want 1", len(h.presenter.closed))
	}
}

func TestDialogUnknownMethodIsRefused(t *testing.T) {
	h := newDialogHarness(t, true)
	if _, err := h.svc.Handle(context.Background(), "plugin-a", "dialog.levitate", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected an unknown dialog method to be refused")
	}
}

func itoaTest(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
