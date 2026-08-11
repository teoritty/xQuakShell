package usecase

import (
	"context"
	"strings"
	"time"

	"xquakshell/internal/domain"
)

type SettingsService struct {
	vaultRepo domain.VaultRepository
	lockout   domain.LockoutManager
	pingMgr   *PingManager
}

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
		// The settings dialog does not own all of AppSettings, and this is a whole-struct
		// assignment. Plugin trust and the capability grants are established elsewhere — through
		// install-time consent prompts — and never travel in AppSettingsDTO, so without carrying
		// them across, changing the theme silently cleared RequireSignedPlugins, every granted
		// capability, and the disabled-plugin list. Embed is here for the same reason.
		normalized.Plugins = data.Settings.Plugins
		normalized.Embed = data.Settings.Embed
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

// SavePluginSettings persists the plugin section on its own.
//
// It exists because SaveSettings deliberately refuses to write that section: the settings dialog
// does not own it, so SaveSettings carries the stored copy across to stop a theme change from
// clearing every capability grant. The consequence was that the one handler which DOES own the
// section could not write it either — SavePluginSettings assembled its struct and SaveSettings
// discarded it, so "require signed plugins" silently never persisted.
//
// The caller is responsible for merging: this writes what it is given, and what it is given must
// already carry the fields the caller does not own.
func (s *SettingsService) SavePluginSettings(ctx context.Context, plugins domain.PluginSettings) error {
	return s.vaultRepo.UpdateData(ctx, func(data *domain.VaultData) error {
		if data.Settings == nil {
			defaults := defaultAppSettings()
			data.Settings = &defaults
		}
		data.Settings.Plugins = plugins
		return nil
	})
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
		Debug:          domain.DefaultDebugSettings(),
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
	if s.Ping.MaxConcurrent <= 0 {
		s.Ping.MaxConcurrent = 16
	}
	if s.Ping.MaxConcurrent > 64 {
		s.Ping.MaxConcurrent = 64
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
