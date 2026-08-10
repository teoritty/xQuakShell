package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"xquakshell/internal/domain"
)

// UpdateService answers one question: is the release this build came from still the latest one?
//
// Only the latest release is supported (ADR-017), which is only a fair policy if a user can find
// out that a newer one exists — a portable archive sitting on a disk says nothing. It downloads
// and installs nothing; the whole feature is a version comparison and a link.
type UpdateService struct {
	feed        domain.ReleaseFeed
	settingsSvc *SettingsService
	audit       domain.AuditLogRepository
	current     string

	mu   sync.RWMutex
	last domain.UpdateStatus
}

func NewUpdateService(feed domain.ReleaseFeed, settingsSvc *SettingsService, audit domain.AuditLogRepository, currentVersion string) *UpdateService {
	return &UpdateService{feed: feed, settingsSvc: settingsSvc, audit: audit, current: currentVersion}
}

// Check contacts the release feed and records the result. It returns the status rather than an
// error for the ordinary "could not reach GitHub" case: being offline is the normal state of a lot
// of installations of an SSH client, and it is not something to report as a failure.
func (s *UpdateService) Check(ctx context.Context) domain.UpdateStatus {
	if !s.startupCheckEnabled() {
		return domain.UpdateStatus{CurrentVersion: s.current}
	}

	release, err := s.feed.LatestStableRelease(ctx)
	if err != nil {
		// ErrNoStableRelease means there is nothing to upgrade to, which for the user is the same
		// as being current; anything else is a network or API problem they cannot act on either.
		if !errors.Is(err, domain.ErrNoStableRelease) {
			slog.Debug("update check failed", "err", err)
		}
		s.record(domain.UpdateStatus{CurrentVersion: s.current, Checked: true})
		return s.Last()
	}

	status := domain.UpdateStatus{
		CurrentVersion:  s.current,
		LatestVersion:   release.Version,
		ReleaseURL:      release.URL,
		UpdateAvailable: domain.IsNewerRelease(release.Version, s.current),
		Checked:         true,
	}
	s.record(status)
	s.auditCheck(ctx, status)
	return status
}

// Last returns the most recent result without contacting anything, so a window that reopens or a
// component that mounts late can render the banner without triggering a second request.
func (s *UpdateService) Last() domain.UpdateStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.last
}

func (s *UpdateService) record(status domain.UpdateStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = status
}

func (s *UpdateService) startupCheckEnabled() bool {
	if s.settingsSvc == nil {
		return false
	}
	settings, err := s.settingsSvc.GetSettings()
	if err != nil {
		// The settings live in the vault, so this is the locked state. Staying silent is the only
		// safe reading: the user may have turned the check off, and we cannot yet know.
		return false
	}
	return settings.Updates.StartupCheckEnabled()
}

// auditCheck records that the application reached out to GitHub. The request is invisible
// otherwise, and an outbound connection a security tool makes on its own behalf is exactly the kind
// of thing its audit log exists to account for. It carries versions and nothing else.
func (s *UpdateService) auditCheck(ctx context.Context, status domain.UpdateStatus) {
	if s.audit == nil || s.settingsSvc == nil {
		return
	}
	settings, err := s.settingsSvc.GetSettings()
	if err != nil || !settings.AuditLog.Enabled {
		return
	}
	entry := domain.AuditEntry{
		Timestamp: time.Now(),
		Category:  domain.AuditCategorySystem,
		Input:     fmt.Sprintf("update check: running %s, latest release %s", status.CurrentVersion, status.LatestVersion),
	}
	if err := s.audit.Append(ctx, entry); err != nil {
		slog.Warn("audit update check", "err", err)
	}
}
