package wails

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// --- Forwarded agent confirmation ---

// errAgentConfirmGone is what the user sees when an answer arrives for a request that is no longer
// waiting: answered already, timed out, or its session closed. One message for all three, because
// which one it was is no help to the user and is detail about another session's state.
var errAgentConfirmGone = errors.New("this signature request is no longer waiting for an answer")

// ResolveAgentConfirm answers a forwarded-agent signature request.
//
// The answer names the session as well as the request. The session must be live and the request
// must belong to it: an answer cannot approve a signature in a session other than the one whose
// dialog the user was looking at.
func (a *AppAPI) ResolveAgentConfirm(sessionID, requestID, decision string) error {
	if sessionID == "" || requestID == "" {
		return errAgentConfirmGone
	}
	if _, err := a.sessions.GetState(sessionID); err != nil {
		return errAgentConfirmGone
	}
	if err := a.agentPrompts.Resolve(sessionID, requestID, usecase.AgentDecision(decision)); err != nil {
		if !errors.Is(err, domain.ErrNoPendingAgentConfirm) {
			slog.Warn("agent confirm resolve failed", "session", sessionID, "err", err)
		}
		return errAgentConfirmGone
	}
	return nil
}

// onAgentConfirm is the agent policy's hook for a request that needs the user. It blocks the
// agent channel until the frontend answers, the dialog times out, or the session ends.
func (a *AppAPI) onAgentConfirm(ctx context.Context, q usecase.AgentConfirmQuestion) (usecase.AgentDecision, error) {
	if a.ctx == nil {
		return usecase.AgentDeny, fmt.Errorf("no wails context for agent confirmation")
	}
	return a.agentPrompts.Ask(ctx, q, agentConfirmEmitter{ctx: a.ctx})
}

type agentConfirmEmitter struct {
	ctx context.Context
}

func (e agentConfirmEmitter) ShowAgentConfirm(p usecase.AgentConfirmPrompt) {
	wailsrt.EventsEmit(e.ctx, EventAgentConfirmRequired, agentConfirmToDTO(p))
}

func (e agentConfirmEmitter) DismissAgentConfirm(requestID string) {
	wailsrt.EventsEmit(e.ctx, EventAgentConfirmClosed, map[string]string{"requestId": requestID})
}
