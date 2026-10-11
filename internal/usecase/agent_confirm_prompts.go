package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/agentforward"
)

// AgentDecision is the user's answer to a forwarded-agent signature request.
type AgentDecision string

const (
	// AgentAllowOnce signs this one request.
	AgentAllowOnce AgentDecision = "once"
	// AgentAllowWindow signs this request and the same key for the same verified destination in
	// this session until the connection's window ends.
	AgentAllowWindow AgentDecision = "window"
	// AgentDeny refuses the request.
	AgentDeny AgentDecision = "deny"
)

// AgentDestinationView is what the dialog says about where a signature will be used.
type AgentDestinationView struct {
	// Verified is true when the remote client proved the destination with session-bind.
	Verified bool
	// Fingerprint is the destination host key's SHA256 fingerprint; empty when unverified.
	Fingerprint string
	// KnownAs lists the known_hosts names carrying that key, so the user reads a host name
	// rather than a hash. Empty for a key this vault has never seen.
	KnownAs []string
	// ForwardsFurther says the remote client will forward the agent on to the destination too.
	ForwardsFurther bool
}

// AgentConfirmQuestion is one signature request put to the user. Every field is either the
// user's own data or validated remote input (RemoteUser passed ParseUserAuthRequest).
type AgentConfirmQuestion struct {
	SessionID      string
	ConnectionName string
	Host           string
	KeyLabel       string
	KeyFingerprint string
	RemoteUser     string
	Destination    AgentDestinationView
	// WindowMinutes is the window the dialog may offer; zero means it offers none.
	WindowMinutes int
}

// AgentConfirmPrompt is a question bound to the request id its answer must carry.
type AgentConfirmPrompt struct {
	RequestID string
	AgentConfirmQuestion
}

// AgentConfirmUI shows and withdraws confirmation dialogs. Dismiss is called exactly once for
// every prompt shown, however the wait ended.
type AgentConfirmUI interface {
	ShowAgentConfirm(prompt AgentConfirmPrompt)
	DismissAgentConfirm(requestID string)
}

// AgentConfirmFunc asks the user about one request and blocks until an answer, a timeout or ctx.
type AgentConfirmFunc func(ctx context.Context, question AgentConfirmQuestion) (AgentDecision, error)

type pendingAgentConfirm struct {
	sessionID   string
	allowWindow bool
	answer      chan AgentDecision
}

// AgentConfirmPrompts is where a blocked signature request meets the user's answer, which arrives
// later through a separate RPC. It follows PassphrasePrompts: an unguessable request id, exactly one
// answer accepted, and the prompt forgotten before its dialog is told to close.
//
// Two rules are its own. The answer must name the session the request belongs to, so a stray
// answer cannot approve another session's signature. And a window may be granted only where the
// question offered one: the dialog is the only party that decides what can be chosen, and the
// backend does not take the frontend's word for it.
type AgentConfirmPrompts struct {
	// Timeout overrides agentforward.ConfirmTimeout; tests shorten it.
	Timeout time.Duration

	mu      sync.Mutex
	pending map[string]pendingAgentConfirm
}

// Ask shows the question and waits. A timeout is a denial, not an error: the remote command gets a
// refusal it can report, and the dialog cannot be approved later by whoever finds it open.
func (p *AgentConfirmPrompts) Ask(ctx context.Context, q AgentConfirmQuestion, ui AgentConfirmUI) (AgentDecision, error) {
	requestID, answer := p.register(q)
	defer ui.DismissAgentConfirm(requestID)
	defer p.forget(requestID)

	ui.ShowAgentConfirm(AgentConfirmPrompt{RequestID: requestID, AgentConfirmQuestion: q})

	timer := time.NewTimer(p.timeout())
	defer timer.Stop()
	select {
	case d := <-answer:
		return d, nil
	case <-timer.C:
		return AgentDeny, nil
	case <-ctx.Done():
		return AgentDeny, ctx.Err()
	}
}

// Resolve delivers the user's decision for one request of one session.
func (p *AgentConfirmPrompts) Resolve(sessionID, requestID string, d AgentDecision) error {
	if d != AgentAllowOnce && d != AgentAllowWindow && d != AgentDeny {
		return fmt.Errorf("%w: unknown decision %q", domain.ErrNoPendingAgentConfirm, d)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	pending, ok := p.pending[requestID]
	if !ok || pending.sessionID != sessionID {
		return domain.ErrNoPendingAgentConfirm
	}
	if d == AgentAllowWindow && !pending.allowWindow {
		return fmt.Errorf("%w: this request offers no window", domain.ErrNoPendingAgentConfirm)
	}
	delete(p.pending, requestID)
	pending.answer <- d
	return nil
}

func (p *AgentConfirmPrompts) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return agentforward.ConfirmTimeout
}

func (p *AgentConfirmPrompts) register(q AgentConfirmQuestion) (string, <-chan AgentDecision) {
	requestID := "ag-" + newRandomID()
	// Buffered so Resolve never blocks under the lock after Ask has already given up.
	ch := make(chan AgentDecision, 1)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending == nil {
		p.pending = make(map[string]pendingAgentConfirm)
	}
	p.pending[requestID] = pendingAgentConfirm{sessionID: q.SessionID, allowWindow: q.WindowMinutes > 0, answer: ch}
	return requestID, ch
}

func (p *AgentConfirmPrompts) forget(requestID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pending, requestID)
}
