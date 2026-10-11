package usecase

import (
	"context"
	"log/slog"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/agentforward"
)

type approvalRequest struct {
	key      forwardableKey
	login    agentforward.UserAuthRequest
	dest     agentforward.Binding
	verified bool
	cfg      *agentforward.Config
}

// approve returns nil only when the user said yes now, or said "for this session" earlier for the
// same key and the same verified destination.
func (p *agentSessionPolicy) approve(ctx context.Context, r approvalRequest) error {
	grant := agentforward.Grant{SessionID: p.info.SessionID, IdentityID: r.key.identity.ID, Destination: r.dest.HostKeyFingerprint}
	if r.verified && p.svc.window.Allows(grant) {
		return nil
	}
	release, err := p.acquireConfirmSlot(ctx)
	if err != nil {
		return err
	}
	defer release()
	// A dialog answered while this request waited for the slot may have opened the window it needs.
	if r.verified && p.svc.window.Allows(grant) {
		return nil
	}
	if p.svc.cfg.Confirm == nil {
		return domain.ErrAgentSignDenied
	}
	question := p.question(r)
	decision, err := p.svc.cfg.Confirm(ctx, question)
	if err != nil {
		return err
	}
	switch decision {
	case AgentAllowOnce:
		return nil
	case AgentAllowWindow:
		if question.WindowMinutes == 0 {
			return domain.ErrAgentSignDenied
		}
		if err := p.svc.window.Allow(grant, r.cfg.Window()); err != nil {
			// The signature was approved; only the window failed to open, so the next request asks.
			slog.Warn("agent approval window not opened", "session", p.info.SessionID, "err", err)
		}
		return nil
	default:
		return domain.ErrAgentSignDenied
	}
}

func (p *agentSessionPolicy) acquireConfirmSlot(ctx context.Context) (func(), error) {
	select {
	case p.confirmSlot <- struct{}{}:
		return func() { <-p.confirmSlot }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.ctx.Done():
		return nil, domain.ErrAgentSessionEnded
	}
}

// question offers a window only for a verified destination on a connection that allows one. An
// unverified destination is "whichever host asks next", and a window for that is a blanket yes.
func (p *agentSessionPolicy) question(r approvalRequest) AgentConfirmQuestion {
	q := AgentConfirmQuestion{
		SessionID:      p.info.SessionID,
		ConnectionName: p.info.ConnectionName,
		Host:           p.info.Host,
		KeyLabel:       r.key.identity.Comment,
		KeyFingerprint: r.key.identity.Fingerprint,
		RemoteUser:     r.login.User,
		Destination: AgentDestinationView{
			Verified:        r.verified,
			Fingerprint:     r.dest.HostKeyFingerprint,
			ForwardsFurther: r.dest.Forwarding,
		},
	}
	if q.KeyLabel == "" {
		q.KeyLabel = r.key.identity.ID
	}
	if r.verified {
		q.Destination.KnownAs = p.knownNames(r.dest.HostKeyFingerprint)
		q.WindowMinutes = int(r.cfg.Window().Minutes())
	}
	return q
}

func (p *agentSessionPolicy) knownNames(fingerprint string) []string {
	if p.svc.cfg.KnownHosts == nil {
		return nil
	}
	entries, err := p.svc.cfg.KnownHosts.List()
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.Fingerprint == fingerprint {
			names = append(names, e.Host)
		}
	}
	return names
}
