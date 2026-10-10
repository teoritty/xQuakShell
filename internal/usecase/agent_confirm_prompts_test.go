package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain"
)

type recordingAgentUI struct {
	mu        sync.Mutex
	shown     []AgentConfirmPrompt
	dismissed []string
	shownCh   chan AgentConfirmPrompt
}

func newRecordingAgentUI() *recordingAgentUI {
	return &recordingAgentUI{shownCh: make(chan AgentConfirmPrompt, 4)}
}

func (u *recordingAgentUI) ShowAgentConfirm(p AgentConfirmPrompt) {
	u.mu.Lock()
	u.shown = append(u.shown, p)
	u.mu.Unlock()
	u.shownCh <- p
}

func (u *recordingAgentUI) DismissAgentConfirm(id string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.dismissed = append(u.dismissed, id)
}

func askAsync(p *AgentConfirmPrompts, ctx context.Context, q AgentConfirmQuestion, ui AgentConfirmUI) <-chan struct {
	d   AgentDecision
	err error
} {
	out := make(chan struct {
		d   AgentDecision
		err error
	}, 1)
	go func() {
		d, err := p.Ask(ctx, q, ui)
		out <- struct {
			d   AgentDecision
			err error
		}{d, err}
	}()
	return out
}

func TestAgentConfirmDeliversTheDecision(t *testing.T) {
	var p AgentConfirmPrompts
	ui := newRecordingAgentUI()
	res := askAsync(&p, context.Background(), AgentConfirmQuestion{SessionID: "s1", WindowMinutes: 15}, ui)
	prompt := <-ui.shownCh
	if err := p.Resolve("s1", prompt.RequestID, AgentAllowWindow); err != nil {
		t.Fatal(err)
	}
	if r := <-res; r.d != AgentAllowWindow || r.err != nil {
		t.Fatalf("Ask = %v, %v; want the user's window decision", r.d, r.err)
	}
	if len(ui.dismissed) != 1 || ui.dismissed[0] != prompt.RequestID {
		t.Error("the dialog must be dismissed exactly once after it was answered")
	}
}

func TestAgentConfirmRefusesAnAnswerFromAnotherSession(t *testing.T) {
	var p AgentConfirmPrompts
	ui := newRecordingAgentUI()
	res := askAsync(&p, context.Background(), AgentConfirmQuestion{SessionID: "s1"}, ui)
	prompt := <-ui.shownCh
	if err := p.Resolve("s2", prompt.RequestID, AgentAllowOnce); !errors.Is(err, domain.ErrNoPendingAgentConfirm) {
		t.Fatalf("err = %v; an answer naming another session must not approve this one", err)
	}
	if err := p.Resolve("s1", prompt.RequestID, AgentDeny); err != nil {
		t.Fatalf("the rightful answer must still be accepted after a refused one: %v", err)
	}
	if r := <-res; r.d != AgentDeny {
		t.Fatalf("Ask = %v, want deny", r.d)
	}
}

func TestAgentConfirmRefusesAWindowTheQuestionDidNotOffer(t *testing.T) {
	var p AgentConfirmPrompts
	ui := newRecordingAgentUI()
	res := askAsync(&p, context.Background(), AgentConfirmQuestion{SessionID: "s1", WindowMinutes: 0}, ui)
	prompt := <-ui.shownCh
	if err := p.Resolve("s1", prompt.RequestID, AgentAllowWindow); !errors.Is(err, domain.ErrNoPendingAgentConfirm) {
		t.Fatalf("err = %v; the frontend must not be able to escalate a once-only question to a window", err)
	}
	if err := p.Resolve("s1", prompt.RequestID, "always"); !errors.Is(err, domain.ErrNoPendingAgentConfirm) {
		t.Fatalf("err = %v; an unknown decision must be refused", err)
	}
	_ = p.Resolve("s1", prompt.RequestID, AgentDeny)
	<-res
}

func TestAgentConfirmAcceptsOneAnswer(t *testing.T) {
	var p AgentConfirmPrompts
	ui := newRecordingAgentUI()
	res := askAsync(&p, context.Background(), AgentConfirmQuestion{SessionID: "s1"}, ui)
	prompt := <-ui.shownCh
	if err := p.Resolve("s1", prompt.RequestID, AgentDeny); err != nil {
		t.Fatal(err)
	}
	if err := p.Resolve("s1", prompt.RequestID, AgentAllowOnce); !errors.Is(err, domain.ErrNoPendingAgentConfirm) {
		t.Fatalf("err = %v; a second answer must not replace the first", err)
	}
	if r := <-res; r.d != AgentDeny {
		t.Fatalf("Ask = %v; the first answer stands", r.d)
	}
}

func TestAgentConfirmTimesOutAsADenial(t *testing.T) {
	p := AgentConfirmPrompts{Timeout: 20 * time.Millisecond}
	ui := newRecordingAgentUI()
	res := askAsync(&p, context.Background(), AgentConfirmQuestion{SessionID: "s1"}, ui)
	prompt := <-ui.shownCh
	r := <-res
	if r.d != AgentDeny || r.err != nil {
		t.Fatalf("Ask = %v, %v; an unanswered dialog must deny, not error", r.d, r.err)
	}
	if err := p.Resolve("s1", prompt.RequestID, AgentAllowOnce); !errors.Is(err, domain.ErrNoPendingAgentConfirm) {
		t.Fatalf("err = %v; a timed-out dialog must not be approvable afterwards", err)
	}
}

func TestAgentConfirmEndsWithTheSession(t *testing.T) {
	var p AgentConfirmPrompts
	ui := newRecordingAgentUI()
	ctx, cancel := context.WithCancel(context.Background())
	res := askAsync(&p, ctx, AgentConfirmQuestion{SessionID: "s1"}, ui)
	<-ui.shownCh
	cancel()
	if r := <-res; r.d != AgentDeny || !errors.Is(r.err, context.Canceled) {
		t.Fatalf("Ask = %v, %v; closing the session must release the wait as a denial", r.d, r.err)
	}
}

func TestAgentConfirmDefaultTimeoutIsTheADRValue(t *testing.T) {
	var p AgentConfirmPrompts
	if p.timeout() != 60*time.Second {
		t.Fatalf("default timeout = %s, want 60s (ADR-022)", p.timeout())
	}
}
