package wails

import (
	"context"
	"fmt"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"xquakshell/internal/usecase"
)

// --- Key passphrase prompt ---

// ResolvePassphrase answers a passphrase prompt the backend raised for a connecting session.
//
// The prompt is named by its request id and nothing else: which key it opens and which session
// uses it are held by the waiting connection, so the frontend supplies only what the user typed.
func (a *AppAPI) ResolvePassphrase(requestID, passphrase string) error {
	return a.passphrasePrompts.Resolve(requestID, passphrase)
}

// CancelPassphrase answers a passphrase prompt with the user's refusal; the connection waiting on
// it fails instead of hanging in "connecting".
func (a *AppAPI) CancelPassphrase(requestID string) error {
	return a.passphrasePrompts.Cancel(requestID)
}

// onPassphraseRequest is the SSH connector's hook for a key that needs the user's passphrase. It
// blocks the connecting goroutine until the frontend answers through ResolvePassphrase or
// CancelPassphrase, or the session's ctx ends.
func (a *AppAPI) onPassphraseRequest(ctx context.Context, identityID, label string) (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("no wails context for passphrase request")
	}
	return a.passphrasePrompts.Ask(ctx, identityID, label, passphrasePromptEmitter{ctx: a.ctx})
}

// passphrasePromptEmitter turns prompt lifecycle calls into frontend events. The payloads carry no
// secret in either direction; the passphrase itself only ever travels frontend-to-backend, as the
// argument of ResolvePassphrase.
type passphrasePromptEmitter struct {
	ctx context.Context
}

func (e passphrasePromptEmitter) ShowPassphrasePrompt(p usecase.PassphrasePrompt) {
	wailsrt.EventsEmit(e.ctx, EventPassphraseRequired, map[string]string{
		"requestId":  p.RequestID,
		"identityId": p.IdentityID,
		"label":      p.Label,
	})
}

func (e passphrasePromptEmitter) DismissPassphrasePrompt(requestID string) {
	wailsrt.EventsEmit(e.ctx, EventPassphrasePromptClosed, map[string]string{"requestId": requestID})
}
