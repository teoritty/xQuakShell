package usecase

import (
	"context"
	"sync"

	"xquakshell/internal/domain"
)

// PassphraseQuestion is what a connection needs the user to answer: which key to open and what to
// call it on screen.
type PassphraseQuestion struct {
	IdentityID string
	Label      string
	// Retry says the passphrase the user typed for this key a moment ago, for this same
	// connection, did not open it. Without it the second prompt is indistinguishable from the
	// first and reads as the application ignoring what was typed.
	Retry bool
}

// PassphrasePrompt is one question put to the user: open this key so a connection can use it.
//
// It carries no secret and nothing the answer is checked against. RequestID is the only thing the
// answer is bound to, which is what keeps a passphrase typed for one key from being delivered to a
// different connection's prompt.
type PassphrasePrompt struct {
	RequestID string
	PassphraseQuestion
}

// PassphrasePromptUI puts a prompt on screen and takes it off again. Dismiss is called exactly once
// for every prompt Show was called for, however the wait ended, so a dialog never outlives the
// connection it was asking for.
type PassphrasePromptUI interface {
	ShowPassphrasePrompt(prompt PassphrasePrompt)
	DismissPassphrasePrompt(requestID string)
}

type passphraseAnswer struct {
	passphrase string
	cancelled  bool
}

// PassphrasePrompts holds the passphrase questions a connecting session is blocked on until the
// user answers them. The zero value is ready to use.
//
// A connection asks synchronously - the key is needed before the handshake can go on - while the
// answer arrives later through a separate RPC. This type is the meeting point between the two.
// A blocking native dialog cannot stand in for it: the platform message box has no text field,
// which is how the prompt once ended up telling the user it could not ask and failing the connect.
type PassphrasePrompts struct {
	mu      sync.Mutex
	pending map[string]chan passphraseAnswer
}

// Ask shows a prompt for the key and waits for the answer, the user's cancel, or ctx ending -
// whichever comes first. ctx is the session's, so closing the tab or locking the vault releases the
// wait and takes the dialog down with it.
func (p *PassphrasePrompts) Ask(ctx context.Context, question PassphraseQuestion, ui PassphrasePromptUI) (string, error) {
	requestID, answer := p.register()
	// Deferred in this order so they run the other way round: forget first, then dismiss. By the
	// time the dialog is told to close the id is already unanswerable, so a submit racing the
	// dismissal is refused instead of being delivered to a connection that has stopped listening.
	defer ui.DismissPassphrasePrompt(requestID)
	defer p.forget(requestID)

	ui.ShowPassphrasePrompt(PassphrasePrompt{RequestID: requestID, PassphraseQuestion: question})

	select {
	case a := <-answer:
		if a.cancelled {
			return "", domain.ErrPassphrasePromptCancelled
		}
		return a.passphrase, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Resolve delivers the user's passphrase to the prompt it was typed into. An answer is accepted
// once: the prompt stops waiting as soon as it has one, so a second submit for the same id is
// refused rather than silently replacing the first.
func (p *PassphrasePrompts) Resolve(requestID, passphrase string) error {
	return p.deliver(requestID, passphraseAnswer{passphrase: passphrase})
}

// Cancel answers the prompt with the user's refusal; the connection waiting on it fails with
// ErrPassphrasePromptCancelled.
func (p *PassphrasePrompts) Cancel(requestID string) error {
	return p.deliver(requestID, passphraseAnswer{cancelled: true})
}

func (p *PassphrasePrompts) register() (string, <-chan passphraseAnswer) {
	requestID := "pp-" + newRandomID()
	// Buffered so deliver never blocks while holding the lock, even if Ask has already returned
	// through ctx and nobody will read the answer.
	ch := make(chan passphraseAnswer, 1)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending == nil {
		p.pending = make(map[string]chan passphraseAnswer)
	}
	p.pending[requestID] = ch
	return requestID, ch
}

func (p *PassphrasePrompts) forget(requestID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pending, requestID)
}

func (p *PassphrasePrompts) deliver(requestID string, a passphraseAnswer) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch, ok := p.pending[requestID]
	if !ok {
		return domain.ErrNoPendingPassphrasePrompt
	}
	delete(p.pending, requestID)
	ch <- a
	return nil
}
