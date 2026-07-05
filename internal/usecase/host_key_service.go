package usecase

import (
	"context"
	"fmt"

	"ssh-client/internal/domain"
)

// HostKeyService orchestrates known-host mutations and host-key verification prompts.
type HostKeyService struct {
	repo     domain.KnownHostsRepository
	sessions *SessionManager
}

// NewHostKeyService creates a HostKeyService.
func NewHostKeyService(repo domain.KnownHostsRepository, sessions *SessionManager) *HostKeyService {
	return &HostKeyService{repo: repo, sessions: sessions}
}

// List returns all known host entries.
func (s *HostKeyService) List() ([]domain.KnownHostEntry, error) {
	return s.repo.List()
}

// Add parses authorizedKey and stores a new known host entry.
func (s *HostKeyService) Add(ctx context.Context, host, authorizedKey string) error {
	key, err := domain.ParseAuthorizedSSHKey(authorizedKey)
	if err != nil {
		return err
	}
	return s.repo.Add(ctx, host, key)
}

// Remove deletes a known host entry by host pattern.
func (s *HostKeyService) Remove(ctx context.Context, host string) error {
	return s.repo.Remove(ctx, host)
}

// Replace parses authorizedKey and replaces the existing host key entry.
func (s *HostKeyService) Replace(ctx context.Context, host, authorizedKey string) error {
	key, err := domain.ParseAuthorizedSSHKey(authorizedKey)
	if err != nil {
		return err
	}
	return s.repo.Replace(ctx, host, key)
}

// Verify checks a remote host key against stored known hosts.
func (s *HostKeyService) Verify(host string, remoteKey domain.PublicKey) error {
	return s.repo.Check(host, remoteKey)
}

// ResolveHostKey handles the user's decision on a pending host key verification.
// action is "add" or "replace"; after resolving, retries the session connection.
func (s *HostKeyService) ResolveHostKey(ctx context.Context, sessionID, action, host, authorizedKey string) error {
	key, err := domain.ParseAuthorizedSSHKey(authorizedKey)
	if err != nil {
		return err
	}

	switch action {
	case "add":
		if err := s.repo.Add(ctx, host, key); err != nil {
			return fmt.Errorf("add host key: %w", err)
		}
	case "replace":
		if err := s.repo.Replace(ctx, host, key); err != nil {
			return fmt.Errorf("replace host key: %w", err)
		}
	default:
		return fmt.Errorf("unknown host key action %q", action)
	}

	return s.sessions.RetrySession(ctx, sessionID)
}
