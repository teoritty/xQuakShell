package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/domain"
)

// scriptedPromptUI records what reached the screen and hands each shown prompt to answer, which
// plays the part of the user acting on the dialog.
type scriptedPromptUI struct {
	mu        sync.Mutex
	shown     []PassphrasePrompt
	dismissed []string
	answer    func(PassphrasePrompt)
}

func (u *scriptedPromptUI) ShowPassphrasePrompt(p PassphrasePrompt) {
	u.mu.Lock()
	u.shown = append(u.shown, p)
	u.mu.Unlock()
	if u.answer != nil {
		u.answer(p)
	}
}

func (u *scriptedPromptUI) DismissPassphrasePrompt(requestID string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.dismissed = append(u.dismissed, requestID)
}

func (u *scriptedPromptUI) snapshot() ([]PassphrasePrompt, []string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]PassphrasePrompt(nil), u.shown...), append([]string(nil), u.dismissed...)
}

func TestPassphrasePromptsDeliversTheTypedPassphrase(t *testing.T) {
	var prompts PassphrasePrompts
	ui := &scriptedPromptUI{}
	ui.answer = func(p PassphrasePrompt) {
		if err := prompts.Resolve(p.RequestID, "hunter2"); err != nil {
			t.Errorf("Resolve = %v, want nil for a prompt that is on screen", err)
		}
	}

	got, err := prompts.Ask(context.Background(), PassphraseQuestion{IdentityID: "id-1", Label: "prod key"}, ui)
	if err != nil || got != "hunter2" {
		t.Fatalf("Ask = (%q, %v), want (\"hunter2\", nil)", got, err)
	}
	shown, dismissed := ui.snapshot()
	if len(shown) != 1 || shown[0].IdentityID != "id-1" || shown[0].Label != "prod key" {
		t.Errorf("shown = %+v, want one prompt naming id-1 and its label", shown)
	}
	if len(dismissed) != 1 || dismissed[0] != shown[0].RequestID {
		t.Errorf("dismissed = %v, want exactly the shown request; an answered dialog must close", dismissed)
	}
}

func TestPassphrasePromptsCancelFailsTheConnection(t *testing.T) {
	var prompts PassphrasePrompts
	ui := &scriptedPromptUI{}
	ui.answer = func(p PassphrasePrompt) { _ = prompts.Cancel(p.RequestID) }

	_, err := prompts.Ask(context.Background(), PassphraseQuestion{IdentityID: "id-1", Label: "k"}, ui)
	if !errors.Is(err, domain.ErrPassphrasePromptCancelled) {
		t.Fatalf("Ask after cancel = %v, want ErrPassphrasePromptCancelled", err)
	}
	if _, dismissed := ui.snapshot(); len(dismissed) != 1 {
		t.Errorf("dismissed %d times, want 1", len(dismissed))
	}
}

// Closing the session while the dialog is open must release the waiting connection and take the
// dialog down. Before this, nothing waited at all; a naive wait without ctx would leak a goroutine
// per abandoned prompt.
func TestPassphrasePromptsSessionEndReleasesTheWait(t *testing.T) {
	var prompts PassphrasePrompts
	ctx, cancel := context.WithCancel(context.Background())
	ui := &scriptedPromptUI{answer: func(PassphrasePrompt) { cancel() }}

	_, err := prompts.Ask(ctx, PassphraseQuestion{IdentityID: "id-1", Label: "k"}, ui)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Ask with a cancelled session = %v, want context.Canceled", err)
	}
	shown, dismissed := ui.snapshot()
	if len(dismissed) != 1 {
		t.Fatalf("dismissed %d times, want 1; the dialog would outlive its session", len(dismissed))
	}
	if err := prompts.Resolve(shown[0].RequestID, "late"); !errors.Is(err, domain.ErrNoPendingPassphrasePrompt) {
		t.Errorf("Resolve after the session ended = %v, want ErrNoPendingPassphrasePrompt", err)
	}
}

// An answer is accepted once. A second submit - a double click, or a replay through window.go -
// must be refused rather than quietly overwrite what the connection already used.
func TestPassphrasePromptsAcceptsOneAnswerPerRequest(t *testing.T) {
	var prompts PassphrasePrompts
	var second error
	ui := &scriptedPromptUI{}
	ui.answer = func(p PassphrasePrompt) {
		_ = prompts.Resolve(p.RequestID, "first")
		second = prompts.Resolve(p.RequestID, "second")
	}

	got, err := prompts.Ask(context.Background(), PassphraseQuestion{IdentityID: "id-1", Label: "k"}, ui)
	if err != nil || got != "first" {
		t.Fatalf("Ask = (%q, %v), want the first answer", got, err)
	}
	if !errors.Is(second, domain.ErrNoPendingPassphrasePrompt) {
		t.Errorf("second Resolve = %v, want ErrNoPendingPassphrasePrompt", second)
	}
}

func TestPassphrasePromptsRefusesAnUnknownRequest(t *testing.T) {
	var prompts PassphrasePrompts
	if err := prompts.Resolve("pp-forged", "x"); !errors.Is(err, domain.ErrNoPendingPassphrasePrompt) {
		t.Errorf("Resolve of a never-raised id = %v, want ErrNoPendingPassphrasePrompt", err)
	}
	if err := prompts.Cancel("pp-forged"); !errors.Is(err, domain.ErrNoPendingPassphrasePrompt) {
		t.Errorf("Cancel of a never-raised id = %v, want ErrNoPendingPassphrasePrompt", err)
	}
}

// Two sessions can wait on two keys at once. Each answer must reach its own prompt: the request id
// is the binding, and crossing them would hand one key's passphrase to another key's connection.
func TestPassphrasePromptsKeepsConcurrentRequestsApart(t *testing.T) {
	var prompts PassphrasePrompts
	shownCh := make(chan PassphrasePrompt, 2)
	ui := &scriptedPromptUI{answer: func(p PassphrasePrompt) { shownCh <- p }}

	type result struct {
		identity, got string
		err           error
	}
	results := make(chan result, 2)
	for _, id := range []string{"id-a", "id-b"} {
		go func() {
			got, err := prompts.Ask(context.Background(), PassphraseQuestion{IdentityID: id, Label: id}, ui)
			results <- result{identity: id, got: got, err: err}
		}()
	}

	for range 2 {
		select {
		case p := <-shownCh:
			if err := prompts.Resolve(p.RequestID, "pass-for-"+p.IdentityID); err != nil {
				t.Fatalf("Resolve(%s) = %v", p.IdentityID, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("prompt was never shown")
		}
	}
	for range 2 {
		r := <-results
		if r.err != nil || r.got != "pass-for-"+r.identity {
			t.Errorf("%s got (%q, %v), want its own passphrase", r.identity, r.got, r.err)
		}
	}
}
