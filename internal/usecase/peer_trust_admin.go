package usecase

import (
	"context"
	"fmt"

	"xquakshell/internal/domain"
)

// Managing recorded trust, as opposed to establishing it.
//
// A file of its own because it changes for its own reason: Verify and Resolve answer a plugin and
// a dialog, these two answer a management screen. Without a way to reach them, a wrongly confirmed identity
// would be permanent - the SSH path has had Known Hosts since the beginning, and a mechanism that
// can only ever add trust is not a security mechanism.

// ListTrustedPeers returns every recorded entry.
//
// Ordering is left to the caller: the repository stores newest-last, and a screen that wants
// another order should say so itself rather than have one imposed here.
func (s *PeerTrustService) ListTrustedPeers() ([]domain.PeerTrustEntry, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("peer trust: no repository")
	}
	return s.repo.List()
}

// Revoke removes one recorded identity.
//
// Audited as its own action, not as a decision: a revocation is the moment a peer that used to be
// trusted stops being trusted, and an investigation that cannot see it has to guess why the next
// connection asked again.
//
// Idempotent, because the repository's Remove is: removing something already gone is the same
// outcome the caller wanted, and reporting it as an error only invites callers to ignore errors.
func (s *PeerTrustService) Revoke(ctx context.Context, scope, subject string) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("peer trust: no repository")
	}
	if scope == "" || subject == "" {
		return fmt.Errorf("peer trust: revoke needs both a scope and a subject")
	}
	revocation := PeerTrustAuditEntry{Action: peerTrustAuditRevoke, Scope: scope, Subject: subject}
	if err := s.repo.Remove(ctx, scope, subject); err != nil {
		s.recordAudit(revocation.asDenied("", err))
		return fmt.Errorf("revoke peer trust: %w", err)
	}
	s.recordAudit(revocation.asAllowed(""))
	return nil
}
