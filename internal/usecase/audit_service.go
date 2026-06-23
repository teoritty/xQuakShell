package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"ssh-client/internal/domain"
)

// AuditService orchestrates audit log recording, retention, and session secret policy.
type AuditService struct {
	repo            domain.AuditLogRepository
	settingsSvc     *SettingsService
	sessions        SessionInfoProvider
	connRepo        domain.ConnectionRepository
	sanitizers      map[string]domain.AuditInputSanitizer
	sanitizersMu    sync.Mutex
	sanitizerFactory domain.AuditInputSanitizerFactory
	trackerFactory  domain.CommandLineTrackerFactory
	lineTrackers    map[string]domain.CommandLineTracker
	lineTrackersMu  sync.Mutex

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
	trackerFactory domain.CommandLineTrackerFactory,
	sanitizerFactory domain.AuditInputSanitizerFactory,
) *AuditService {
	return &AuditService{
		repo:             repo,
		settingsSvc:      settingsSvc,
		sessions:         sessions,
		connRepo:         connRepo,
		sanitizers:       make(map[string]domain.AuditInputSanitizer),
		trackerFactory:   trackerFactory,
		lineTrackers:     make(map[string]domain.CommandLineTracker),
		sanitizerFactory: sanitizerFactory,
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

func (s *AuditService) getSanitizer(sessionID string) domain.AuditInputSanitizer {
	if s.sanitizerFactory == nil {
		return nil
	}
	s.sanitizersMu.Lock()
	defer s.sanitizersMu.Unlock()
	san, ok := s.sanitizers[sessionID]
	if !ok {
		san = s.sanitizerFactory()
		s.sanitizers[sessionID] = san
	}
	return san
}

// FeedOutput updates sanitizer context from terminal output.
func (s *AuditService) FeedOutput(sessionID, output string) {
	if san := s.getSanitizer(sessionID); san != nil {
		san.FeedOutput(output)
	}
}

// RemoveSession cleans up per-session sanitizer state.
func (s *AuditService) RemoveSession(sessionID string) {
	s.sanitizersMu.Lock()
	delete(s.sanitizers, sessionID)
	s.sanitizersMu.Unlock()

	s.lineTrackersMu.Lock()
	delete(s.lineTrackers, sessionID)
	s.lineTrackersMu.Unlock()
}

// ResolveCommandLine returns the submitted command line for audit logging.
func (s *AuditService) ResolveCommandLine(sessionID, data, commandLine string) (string, bool) {
	if commandLine != "" {
		if tracker := s.getLineTracker(sessionID); tracker != nil {
			_, _ = tracker.Feed(data)
		}
		return commandLine, true
	}
	tracker := s.getLineTracker(sessionID)
	submitted, ok := tracker.Feed(data)
	if !ok || submitted == "" {
		return "", false
	}
	return submitted, true
}

func (s *AuditService) getLineTracker(sessionID string) domain.CommandLineTracker {
	if s.trackerFactory == nil {
		return nil
	}
	s.lineTrackersMu.Lock()
	defer s.lineTrackersMu.Unlock()
	tracker, ok := s.lineTrackers[sessionID]
	if !ok {
		tracker = s.trackerFactory()
		s.lineTrackers[sessionID] = tracker
	}
	return tracker
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
	if !s.SessionLogSecretsEnabled() && sanitizer != nil {
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

// Search queries audit log entries.
func (s *AuditService) Search(ctx context.Context, query string, filter domain.AuditSearchFilter) ([]domain.AuditEntry, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("audit log not available")
	}
	return s.repo.Search(ctx, query, filter)
}

// DeleteByID removes a single audit log entry.
func (s *AuditService) DeleteByID(ctx context.Context, id int64) error {
	if s.repo == nil {
		return fmt.Errorf("audit log not available")
	}
	return s.repo.DeleteByID(ctx, id)
}

// ClearAll removes all audit log entries.
func (s *AuditService) ClearAll(ctx context.Context) error {
	if s.repo == nil {
		return fmt.Errorf("audit log not available")
	}
	return s.repo.ClearAll(ctx)
}

// Close releases audit log resources.
func (s *AuditService) Close() {
	if s.repo != nil {
		s.repo.Close()
	}
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
