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
	data, err := s.vaultRepo.GetData()
	if err != nil {
		return err
	}
	if data.Settings == nil {
		data.Settings = &domain.AppSettings{}
	}

	normalized := normalizeSettings(settings)
	*data.Settings = normalized

	if err := s.vaultRepo.SaveData(ctx, data); err != nil {
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

	return s
}
