package usecase

import (
	"context"
	"strings"
	"time"

	"ssh-client/internal/domain"
)

// SettingsService orchestrates reading and persisting application settings.
// It applies defaults and normalization so presentation handlers stay thin.
type SettingsService struct {
	vaultRepo domain.VaultRepository
	lockout   domain.LockoutManager
	pingMgr   *PingManager
}

// NewSettingsService creates a SettingsService with the provided dependencies.
func NewSettingsService(vault domain.VaultRepository, lockout domain.LockoutManager, ping *PingManager) *SettingsService {
	return &SettingsService{vaultRepo: vault, lockout: lockout, pingMgr: ping}
}

// GetSettings returns the effective application settings with defaults applied.
func (s *SettingsService) GetSettings() (domain.AppSettings, error) {
	data, err := s.vaultRepo.GetData()
	if err != nil {
		return domain.AppSettings{}, err
	}
	if data.Settings == nil {
		return defaultAppSettings(), nil
	}
	return normalizeSettings(*data.Settings), nil
}

// SaveSettings validates, normalizes, and persists settings, then applies
// lockout and ping manager updates. Ping restart (with event callback) must be
// triggered by the caller after this method returns.
func (s *SettingsService) SaveSettings(ctx context.Context, settings domain.AppSettings) error {
	normalized := normalizeSettings(settings)
	if err := s.vaultRepo.UpdateData(ctx, func(data *domain.VaultData) error {
		if data.Settings == nil {
			data.Settings = &domain.AppSettings{}
		}
		*data.Settings = normalized
		return nil
	}); err != nil {
		return err
	}

	if s.lockout != nil {
		s.lockout.UpdateSettings(normalized.Lockout)
	}
	if s.pingMgr != nil {
		s.pingMgr.UpdateSettings(normalized.Ping)
	}
	return nil
}

// defaultAppSettings returns factory defaults for a fresh vault.
func defaultAppSettings() domain.AppSettings {
	lockout := domain.DefaultLockoutSettings()
	terminal := domain.DefaultTerminalSettings()
	ping := domain.DefaultPingSettings()
	transfer := domain.DefaultTransferSettings()
	hotkeys := domain.DefaultSessionHotkeysSettings()
	return domain.AppSettings{
		Lockout:  lockout,
		Terminal: terminal,
		Theme:    "dark",
		Ping:     ping,
		Transfer: domain.TransferSettings{
			SpeedLimitKbps:       transfer.SpeedLimitKbps,
			ConnectionTimeoutSec: transfer.ConnectionTimeoutSec,
			MaxConcurrent:        transfer.MaxConcurrent,
		},
		SessionHotkeys: hotkeys,
		AuditLog:       domain.DefaultAuditLogSettings(),
		UIScalePercent: 100,
	}
}

// normalizeSettings fills in missing/invalid values with sensible defaults.
func normalizeSettings(s domain.AppSettings) domain.AppSettings {
	if s.Transfer.ConnectionTimeoutSec <= 0 {
		s.Transfer.ConnectionTimeoutSec = 15
	}
	if s.Transfer.MaxConcurrent <= 0 {
		s.Transfer.MaxConcurrent = 4
	}

	defHotkeys := domain.DefaultSessionHotkeysSettings()
	if strings.TrimSpace(s.SessionHotkeys.Create) == "" {
		s.SessionHotkeys.Create = defHotkeys.Create
	}
	if strings.TrimSpace(s.SessionHotkeys.Next) == "" {
		s.SessionHotkeys.Next = defHotkeys.Next
	}
	if strings.TrimSpace(s.SessionHotkeys.Prev) == "" {
		s.SessionHotkeys.Prev = defHotkeys.Prev
	}
	if strings.TrimSpace(s.SessionHotkeys.Close) == "" {
		s.SessionHotkeys.Close = defHotkeys.Close
	}

	if s.Ping.Mode != domain.PingModeInterval && s.Ping.Mode != domain.PingModeOnChange {
		s.Ping.Mode = domain.PingModeInterval
	}
	if s.Ping.IntervalSeconds < 1 {
		s.Ping.IntervalSeconds = 5
	}

	if s.Lockout.IdleTimeout < time.Minute {
		s.Lockout.IdleTimeout = domain.DefaultLockoutSettings().IdleTimeout
	}

	defAudit := domain.DefaultAuditLogSettings()
	if s.AuditLog.RetentionMode != domain.AuditRetentionByDays && s.AuditLog.RetentionMode != domain.AuditRetentionByCount {
		s.AuditLog.RetentionMode = defAudit.RetentionMode
	}
	if s.AuditLog.RetentionDays <= 0 {
		s.AuditLog.RetentionDays = defAudit.RetentionDays
	}
	if s.AuditLog.RetentionDays > 365 {
		s.AuditLog.RetentionDays = 365
	}
	if s.AuditLog.RetentionCount <= 0 {
		s.AuditLog.RetentionCount = defAudit.RetentionCount
	}
	if s.AuditLog.RetentionCount > 10000 {
		s.AuditLog.RetentionCount = 10000
	}

	if s.UIScalePercent <= 0 {
		s.UIScalePercent = 100
	}
	if s.UIScalePercent < 75 {
		s.UIScalePercent = 75
	}
	if s.UIScalePercent > 200 {
		s.UIScalePercent = 200
	}

	return s
}
