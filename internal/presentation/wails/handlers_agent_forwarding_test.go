package wails

import (
	"context"
	"errors"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/agentforward"
	"xquakshell/internal/usecase"
)

// agentConnRepo serves one connection of a protocol nothing implements, so OpenSession registers
// a session that never dials anything: what these tests need is a live session id, not a shell.
type agentConnRepo struct {
	domain.ConnectionRepository
}

func (agentConnRepo) GetByID(_ context.Context, id string) (*domain.Connection, error) {
	return &domain.Connection{ID: id, Name: "n", Host: "h", Port: 22, Protocol: "none"}, nil
}

type capturingAgentUI struct {
	shown chan usecase.AgentConfirmPrompt
}

func (u capturingAgentUI) ShowAgentConfirm(p usecase.AgentConfirmPrompt) { u.shown <- p }
func (capturingAgentUI) DismissAgentConfirm(string)                      {}

type agentAPIHarness struct {
	t   *testing.T
	api *AppAPI
}

func newAgentAPIHarness(t *testing.T) *agentAPIHarness {
	t.Helper()
	sm := usecase.NewSessionManager(usecase.SessionManagerConfig{ConnRepo: agentConnRepo{}})
	t.Cleanup(sm.CloseAll)
	return &agentAPIHarness{t: t, api: &AppAPI{sessions: sm}}
}

func (h *agentAPIHarness) openSession() string {
	h.t.Helper()
	id, err := h.api.sessions.OpenSession(context.Background(), "c1")
	if err != nil {
		h.t.Fatal(err)
	}
	return id
}

// ask raises a confirmation for sessionID and returns its request id and the eventual decision.
func (h *agentAPIHarness) ask(sessionID string, windowMinutes int) (string, <-chan usecase.AgentDecision) {
	h.t.Helper()
	ui := capturingAgentUI{shown: make(chan usecase.AgentConfirmPrompt, 1)}
	out := make(chan usecase.AgentDecision, 1)
	go func() {
		d, _ := h.api.agentPrompts.Ask(context.Background(), usecase.AgentConfirmQuestion{SessionID: sessionID, WindowMinutes: windowMinutes}, ui)
		out <- d
	}()
	select {
	case p := <-ui.shown:
		return p.RequestID, out
	case <-time.After(5 * time.Second):
		h.t.Fatal("the confirmation was never shown")
		return "", nil
	}
}

func TestResolveAgentConfirmDeliversTheUsersAnswer(t *testing.T) {
	h := newAgentAPIHarness(t)
	sid := h.openSession()
	reqID, decision := h.ask(sid, 15)
	if err := h.api.ResolveAgentConfirm(sid, reqID, "window"); err != nil {
		t.Fatal(err)
	}
	if d := <-decision; d != usecase.AgentAllowWindow {
		t.Fatalf("decision = %q, want window", d)
	}
}

func TestResolveAgentConfirmRefusesInvalidInput(t *testing.T) {
	h := newAgentAPIHarness(t)
	sid := h.openSession()
	reqID, decision := h.ask(sid, 0)
	for name, args := range map[string][3]string{
		"empty session":      {"", reqID, "once"},
		"empty request":      {sid, "", "once"},
		"unknown decision":   {sid, reqID, "always"},
		"window not offered": {sid, reqID, "window"},
	} {
		if err := h.api.ResolveAgentConfirm(args[0], args[1], args[2]); !errors.Is(err, errAgentConfirmGone) {
			t.Errorf("%s: err = %v, want the generic refusal", name, err)
		}
	}
	if err := h.api.ResolveAgentConfirm(sid, reqID, "deny"); err != nil {
		t.Fatalf("the request must still be answerable after refused attempts: %v", err)
	}
	if d := <-decision; d != usecase.AgentDeny {
		t.Fatalf("decision = %q, want deny", d)
	}
}

func TestResolveAgentConfirmRefusesAnotherSessionsRequest(t *testing.T) {
	h := newAgentAPIHarness(t)
	victim, other := h.openSession(), h.openSession()
	reqID, decision := h.ask(victim, 0)
	if err := h.api.ResolveAgentConfirm(other, reqID, "once"); !errors.Is(err, errAgentConfirmGone) {
		t.Fatalf("err = %v; a live session must not approve another session's signature", err)
	}
	_ = h.api.ResolveAgentConfirm(victim, reqID, "deny")
	if d := <-decision; d != usecase.AgentDeny {
		t.Fatalf("decision = %q; the refused cross-session answer must not have been delivered", d)
	}
}

func TestResolveAgentConfirmRefusesAClosedSession(t *testing.T) {
	h := newAgentAPIHarness(t)
	sid := h.openSession()
	reqID, _ := h.ask(sid, 0)
	if err := h.api.sessions.CloseSession(sid); err != nil {
		t.Fatal(err)
	}
	if err := h.api.ResolveAgentConfirm(sid, reqID, "once"); !errors.Is(err, errAgentConfirmGone) {
		t.Fatalf("err = %v; an answer for a closed session must be refused", err)
	}
	if err := h.api.ResolveAgentConfirm("no-such-session", reqID, "once"); !errors.Is(err, errAgentConfirmGone) {
		t.Fatalf("err = %v; an unknown session must be refused", err)
	}
}

func TestAgentForwardingDTORoundTrips(t *testing.T) {
	in := &agentforward.Config{Enabled: true, IdentityIDs: []string{"k"}, Confirm: agentforward.ConfirmSessionWindow, WindowMinutes: 5}
	out := agentForwardingFromDTO(agentForwardingToDTO(in))
	if out.Enabled != in.Enabled || out.Confirm != in.Confirm || out.WindowMinutes != 5 || len(out.IdentityIDs) != 1 {
		t.Fatalf("round trip changed the settings: %+v", out)
	}
	if agentForwardingToDTO(nil) != nil || agentForwardingFromDTO(nil) != nil {
		t.Fatal("no settings must stay no settings in both directions")
	}
	if dto := agentForwardingToDTO(&agentforward.Config{}); dto.Confirm != string(agentforward.ConfirmEach) || dto.IdentityIDs == nil {
		t.Fatalf("an unset policy must show as confirm-each with an empty key list: %+v", dto)
	}
	if c := agentForwardingFromDTO(&AgentForwardingDTO{Confirm: "always"}); c.Validate() == nil {
		t.Fatal("an unknown policy from the UI must reach the domain unchanged, where Validate refuses it")
	}
}
