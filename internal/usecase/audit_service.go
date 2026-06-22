package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/auditlog"
)

// AuditService orchestrates audit log recording, retention, and session secret policy.
type AuditService struct {
	repo         domain.AuditLogRepository
	settingsSvc  *SettingsService
	sessions     SessionInfoProvider
	connRepo     domain.ConnectionRepository
	sanitizers   map[string]*auditlog.Sanitizer
	sanitizersMu sync.Mutex

	sessionLogSecrets bool
	secretsMu         sync.RWMutex
}

// SessionInfoProvider supplies session metadata for audit entries.
type SessionInfoProvider interface {
	GetState(sessionID string) (domain.ConnectionSession, error)
}

// NewAuditService creates an AuditService.
func NewAuditService(
	repo domain.AuditLogRepository,
	settingsSvc *SettingsService,
	sessions SessionInfoProvider,
	connRepo domain.ConnectionRepository,
) *AuditService {
	return &AuditService{
		repo:        repo,
		settingsSvc: settingsSvc,
		sessions:    sessions,
		connRepo:    connRepo,
		sanitizers:  make(map[string]*auditlog.Sanitizer),
	}
}

func (s *AuditService) auditSettings() domain.AuditLogSettings {
	if s.settingsSvc == nil {
		return domain.DefaultAuditLogSettings()
	}
	settings, err := s.settingsSvc.GetSettings()
	if err != nil {
		return domain.DefaultAuditLogSettings()
	}
	return settings.AuditLog
}

// IsEnabled reports whether audit logging is enabled in vault settings.
func (s *AuditService) IsEnabled() bool {
	return s.auditSettings().Enabled
}

// SessionLogSecretsEnabled reports whether secret logging is active this session.
func (s *AuditService) SessionLogSecretsEnabled() bool {
	s.secretsMu.RLock()
	defer s.secretsMu.RUnlock()
	return s.sessionLogSecrets
}

// EnableSessionSecretLogging enables plaintext secret logging until lock/restart.
func (s *AuditService) EnableSessionSecretLogging() {
	s.secretsMu.Lock()
	s.sessionLogSecrets = true
	s.secretsMu.Unlock()
}

// DisableSessionSecretLogging turns off session secret logging.
func (s *AuditService) DisableSessionSecretLogging() {
	s.secretsMu.Lock()
	s.sessionLogSecrets = false
	s.secretsMu.Unlock()
}

// OnVaultLocked resets session-only secret logging.
func (s *AuditService) OnVaultLocked() {
	s.DisableSessionSecretLogging()
}

func (s *AuditService) getSanitizer(sessionID string) *auditlog.Sanitizer {
	s.sanitizersMu.Lock()
	defer s.sanitizersMu.Unlock()
	san, ok := s.sanitizers[sessionID]
	if !ok {
		san = auditlog.NewSanitizer()
		s.sanitizers[sessionID] = san
	}
	return san
}

// FeedOutput updates sanitizer context from terminal output.
func (s *AuditService) FeedOutput(sessionID, output string) {
	s.getSanitizer(sessionID).FeedOutput(output)
}

// RemoveSession cleans up per-session sanitizer state.
func (s *AuditService) RemoveSession(sessionID string) {
	s.sanitizersMu.Lock()
	delete(s.sanitizers, sessionID)
	s.sanitizersMu.Unlock()
}

// RecordCommand persists a submitted command line when audit is enabled.
func (s *AuditService) RecordCommand(ctx context.Context, sessionID, line string) error {
	if s.repo == nil {
		return nil
	}
	line = trimCommandLine(line)
	if line == "" {
		return nil
	}
	if !s.IsEnabled() {
		return nil
	}

	settings := s.auditSettings()
	sanitizer := s.getSanitizer(sessionID)

	input := line
	redacted := false
	if !s.SessionLogSecretsEnabled() {
		input, redacted = sanitizer.SanitizeInput(line)
	}

	info, err := s.sessions.GetState(sessionID)
	if err != nil {
		return err
	}

	conn, _ := s.connRepo.GetByID(ctx, info.ConnectionID)
	username := ""
	connectionName := ""
	host := ""
	if conn != nil {
		if settings.ShowUsername {
			username = conn.EffectiveUsername()
		}
		if settings.ShowConnection {
			connectionName = conn.Name
			h := conn.EffectiveHost()
			p := conn.EffectivePort()
			if h != "" {
				if p > 0 {
					host = fmt.Sprintf("%s:%d", h, p)
				} else {
					host = h
				}
			}
		}
	}

	entry := domain.AuditEntry{
		Timestamp:      time.Now(),
		SessionID:      sessionID,
		ConnectionID:   info.ConnectionID,
		ConnectionName: connectionName,
		Host:           host,
		Username:       username,
		Input:          input,
		Redacted:       redacted,
	}
	if err := s.repo.Append(ctx, entry); err != nil {
		slog.Error("audit append failed", "err", err)
		return domain.ErrAuditLogWrite
	}
	return s.EnforceRetention(ctx)
}

func trimCommandLine(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// EnforceRetention applies configured retention policy.
func (s *AuditService) EnforceRetention(ctx context.Context) error {
	if s.repo == nil || !s.IsEnabled() {
		return nil
	}
	settings := s.auditSettings()
	switch settings.RetentionMode {
	case domain.AuditRetentionByCount:
		if settings.RetentionCount <= 0 {
			return nil
		}
		return s.repo.TrimToCount(ctx, settings.RetentionCount)
	default:
		if settings.RetentionDays <= 0 {
			return nil
		}
		cutoff := time.Now().Add(-time.Duration(settings.RetentionDays) * 24 * time.Hour)
		return s.repo.PurgeOlderThan(ctx, cutoff)
	}
}
