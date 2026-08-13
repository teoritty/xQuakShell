package usecase

import (
	"context"
	"fmt"

	"xquakshell/internal/domain"
)

// HostKeySessions is the slice of session management this service needs. It is an interface
// because the pending key is now the security boundary of ResolveHostKey - the caller no longer
// supplies it - and a boundary that cannot be driven from a test is a boundary nobody checks.
type HostKeySessions interface {
	GetHostKeyInfo(sessionID string) (*domain.HostKeyInfo, error)
	RetrySession(ctx context.Context, sessionID string) error
}

type HostKeyService struct {
	repo     domain.KnownHostsRepository
	sessions HostKeySessions
}

func NewHostKeyService(repo domain.KnownHostsRepository, sessions HostKeySessions) *HostKeyService {
	return &HostKeyService{repo: repo, sessions: sessions}
}

func (s *HostKeyService) List() ([]domain.KnownHostEntry, error) {
	return s.repo.List()
}

func (s *HostKeyService) Remove(ctx context.Context, host string) error {
	return s.repo.Remove(ctx, host)
}

func (s *HostKeyService) Verify(host string, remoteKey domain.PublicKey) error {
	return s.repo.Check(host, remoteKey)
}

// ResolveHostKey records the user's decision on the host key this session is waiting on.
// action is "add" or "replace"; after resolving, retries the session connection.
//
// The host and the key come from the session's own pending state, never from the caller. They used
// to be arguments, which meant the frontend told the backend which key to trust: anything holding
// window.go could call ResolveHostKey(anySessionID, "replace", "prod.example.com", attackerKey) and
// silently swap established trust for a host it had never connected to. The user's decision is the
// only part of this the UI is entitled to supply, and it is one of two words.
//
// A session with nothing pending is refused rather than treated as "add", so this cannot be used
// to write trust outside a verification the host itself triggered.
func (s *HostKeyService) ResolveHostKey(ctx context.Context, sessionID, action string) error {
	pending, err := s.sessions.GetHostKeyInfo(sessionID)
	if err != nil {
		return err
	}
	if pending == nil || pending.KeyBase64 == "" {
		return fmt.Errorf("session %s: %w", sessionID, domain.ErrNoPendingHostKey)
	}

	key, err := domain.ParseAuthorizedSSHKey(pending.KeyBase64)
	if err != nil {
		return fmt.Errorf("pending host key: %w", err)
	}

	switch action {
	case "add":
		if err := s.repo.Add(ctx, pending.Host, key); err != nil {
			return fmt.Errorf("add host key: %w", err)
		}
	case "replace":
		if err := s.repo.Replace(ctx, pending.Host, key); err != nil {
			return fmt.Errorf("replace host key: %w", err)
		}
	default:
		return fmt.Errorf("unknown host key action %q", action)
	}

	return s.sessions.RetrySession(ctx, sessionID)
}
