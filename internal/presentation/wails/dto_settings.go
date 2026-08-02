package wails

import (
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

type LocalNodeDTO struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Owner   string `json:"owner,omitempty"`
}

type AppSettingsDTO struct {
	LockoutEnabled              bool   `json:"lockoutEnabled"`
	LockoutIdleMinutes          int    `json:"lockoutIdleMinutes"`
	LockOnMinimize              bool   `json:"lockOnMinimize"`
	TerminalFontFamily          string `json:"terminalFontFamily"`
	TerminalFontSize            int    `json:"terminalFontSize"`
	TerminalFontColor           string `json:"terminalFontColor"`
	Theme                       string `json:"theme"`
	UIScalePercent              int    `json:"uiScalePercent"`
	PingEnabled                 bool   `json:"pingEnabled"`
	PingMode                    string `json:"pingMode"`
	PingIntervalSeconds         int    `json:"pingIntervalSeconds"`
	PingIntervalMin             int    `json:"pingIntervalMin"`
	MaxConcurrentPings          int    `json:"maxConcurrentPings"`
	ExternalEditorPath          string `json:"externalEditorPath"`
	TransferSpeedLimitKbps      int    `json:"transferSpeedLimitKbps"`
	ConnectionTimeoutSeconds    int    `json:"connectionTimeoutSeconds"`
	MaxConcurrentTransfers      int    `json:"maxConcurrentTransfers"`
	DefaultUploadExistsAction   string `json:"defaultUploadExistsAction"`
	DefaultDownloadExistsAction string `json:"defaultDownloadExistsAction"`
	SessionHotkeyCreate         string `json:"sessionHotkeyCreate"`
	SessionHotkeyNext           string `json:"sessionHotkeyNext"`
	SessionHotkeyPrev           string `json:"sessionHotkeyPrev"`
	SessionHotkeyClose          string `json:"sessionHotkeyClose"`
	AuditLogEnabled             bool   `json:"auditLogEnabled"`
	AuditRetentionMode          string `json:"auditRetentionMode"`
	AuditRetentionDays          int    `json:"auditRetentionDays"`
	AuditRetentionCount         int    `json:"auditRetentionCount"`
	AuditShowUsername           bool   `json:"auditShowUsername"`
	AuditShowConnection         bool   `json:"auditShowConnection"`
	DebugLogWindowEnabled       bool   `json:"debugLogWindowEnabled"`
	DebugLogLevel               string `json:"debugLogLevel"`
}

type PuTTYSessionDTO struct {
	Name     string `json:"name"`
	HostName string `json:"hostName"`
	Port     int    `json:"port"`
	UserName string `json:"userName"`
}

func puttySessionsToDTO(sessions []usecase.PuTTYSessionPreview) []PuTTYSessionDTO {
	result := make([]PuTTYSessionDTO, len(sessions))
	for i, s := range sessions {
		result[i] = PuTTYSessionDTO{
			Name:     s.Name,
			HostName: s.HostName,
			Port:     s.Port,
			UserName: s.UserName,
		}
	}
	return result
}

func LocalNodeToDTO(name, fullPath string, isDir bool, size int64, modTime, mode, owner string) LocalNodeDTO {
	return LocalNodeDTO{Name: name, Path: fullPath, IsDir: isDir, Size: size, ModTime: modTime, Mode: mode, Owner: owner}
}

func AppSettingsToDTO(s domain.AppSettings) AppSettingsDTO {
	return AppSettingsDTO{
		LockoutEnabled:              s.Lockout.Enabled,
		LockoutIdleMinutes:          int(s.Lockout.IdleTimeout.Minutes()),
		LockOnMinimize:              s.Lockout.LockOnMinimize,
		TerminalFontFamily:          s.Terminal.FontFamily,
		TerminalFontSize:            s.Terminal.FontSize,
		TerminalFontColor:           s.Terminal.FontColor,
		Theme:                       s.Theme,
		UIScalePercent:              s.UIScalePercent,
		PingEnabled:                 s.Ping.Enabled,
		PingMode:                    s.Ping.Mode,
		PingIntervalSeconds:         s.Ping.EffectiveIntervalSeconds(),
		MaxConcurrentPings:          s.Ping.EffectiveMaxConcurrent(),
		ExternalEditorPath:          s.ExternalEditorPath,
		TransferSpeedLimitKbps:      s.Transfer.SpeedLimitKbps,
		ConnectionTimeoutSeconds:    s.Transfer.ConnectionTimeoutSec,
		MaxConcurrentTransfers:      s.Transfer.MaxConcurrent,
		DefaultUploadExistsAction:   s.Transfer.DefaultUploadExistsAction,
		DefaultDownloadExistsAction: s.Transfer.DefaultDownloadExistsAction,
		SessionHotkeyCreate:         s.SessionHotkeys.Create,
		SessionHotkeyNext:           s.SessionHotkeys.Next,
		SessionHotkeyPrev:           s.SessionHotkeys.Prev,
		SessionHotkeyClose:          s.SessionHotkeys.Close,
		AuditLogEnabled:             s.AuditLog.Enabled,
		AuditRetentionMode:          string(s.AuditLog.RetentionMode),
		AuditRetentionDays:          s.AuditLog.RetentionDays,
		AuditRetentionCount:         s.AuditLog.RetentionCount,
		AuditShowUsername:           s.AuditLog.ShowUsername,
		AuditShowConnection:         s.AuditLog.ShowConnection,
		DebugLogWindowEnabled:       s.Debug.LogWindowEnabled,
		DebugLogLevel:               s.Debug.LogLevel,
	}
}

func DTOToAppSettings(dto AppSettingsDTO) domain.AppSettings {
	return domain.AppSettings{
		Lockout: domain.LockoutSettings{
			Enabled:        dto.LockoutEnabled,
			IdleTimeout:    time.Duration(dto.LockoutIdleMinutes) * time.Minute,
			LockOnMinimize: dto.LockOnMinimize,
		},
		Terminal: domain.TerminalSettings{
			FontFamily: dto.TerminalFontFamily,
			FontSize:   dto.TerminalFontSize,
			FontColor:  dto.TerminalFontColor,
		},
		Theme:          dto.Theme,
		UIScalePercent: dto.UIScalePercent,
		Ping: domain.PingSettings{
			Enabled:         dto.PingEnabled,
			Mode:            dto.PingMode,
			IntervalSeconds: dto.PingIntervalSeconds,
			MaxConcurrent:   dto.MaxConcurrentPings,
		},
		ExternalEditorPath: dto.ExternalEditorPath,
		Transfer: domain.TransferSettings{
			SpeedLimitKbps:              dto.TransferSpeedLimitKbps,
			ConnectionTimeoutSec:        dto.ConnectionTimeoutSeconds,
			MaxConcurrent:               dto.MaxConcurrentTransfers,
			DefaultUploadExistsAction:   dto.DefaultUploadExistsAction,
			DefaultDownloadExistsAction: dto.DefaultDownloadExistsAction,
		},
		SessionHotkeys: domain.SessionHotkeysSettings{
			Create: dto.SessionHotkeyCreate,
			Next:   dto.SessionHotkeyNext,
			Prev:   dto.SessionHotkeyPrev,
			Close:  dto.SessionHotkeyClose,
		},
		AuditLog: domain.AuditLogSettings{
			Enabled:        dto.AuditLogEnabled,
			RetentionMode:  domain.AuditRetentionMode(dto.AuditRetentionMode),
			RetentionDays:  dto.AuditRetentionDays,
			RetentionCount: dto.AuditRetentionCount,
			ShowUsername:   dto.AuditShowUsername,
			ShowConnection: dto.AuditShowConnection,
		},
		Debug: domain.DebugSettings{
			LogWindowEnabled: dto.DebugLogWindowEnabled,
			LogLevel:         dto.DebugLogLevel,
		},
	}
}
